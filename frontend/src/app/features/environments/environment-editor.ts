import {
  Component,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import * as EnvironmentsService from '@bindings/github.com/awake-client/awake/src/features/environments/service';
import { Environment } from '@bindings/github.com/awake-client/awake/src/features/environments/models';
import * as SecretsService from '@bindings/github.com/awake-client/awake/src/features/secrets/service';
import { SecretState } from '@bindings/github.com/awake-client/awake/src/features/secrets/models';
import { Confirmation } from '@core/confirmation';
import { EnvironmentStore } from '@core/environment-store';
import { Prompt } from '@core/prompt';
import { WorkspaceStore } from '@core/workspace-store';
import { ColorPicker } from '@shared/color-picker';
import {
  RESERVED_VARIABLES,
  ReservedVariable,
  parseReservedValue,
  reservedVariable,
} from '@shared/environment-authentication';
import { Icon } from '@shared/icon';
import { Select, SelectOption } from '@shared/select';
import { SecretInput } from '@shared/secret-input';
import { slugify } from '@shared/slug';

interface VariableRow {
  name: string;
  value: string;
}

/** Shared variables live in the tracked file, local ones on this machine. */
type VariableList = 'shared' | 'local';

/** Window event sent after an environment was saved, cloned or deleted. */
export const ENVIRONMENTS_CHANGED_EVENT = 'awake:environments-changed';

/**
 * Edits one environment: its name, color, variables and secrets, and offers
 * to clone or delete it. Used by the environments page and the edit dialog.
 */
@Component({
  selector: 'app-environment-editor',
  imports: [FormsModule, Icon, ColorPicker, SecretInput, Select],
  templateUrl: './environment-editor.html',
  styleUrl: './environment-editor.css',
})
export class EnvironmentEditor {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly environmentStore = inject(EnvironmentStore);
  private readonly confirmation = inject(Confirmation);
  private readonly prompt = inject(Prompt);

  /** Empty for a global environment, else the collection defining it. */
  readonly scope = input('');
  readonly fileName = input.required<string>();
  /** Emitted with the file name of a new clone. */
  readonly cloned = output<string>();
  readonly deleted = output<void>();
  readonly saved = output<void>();
  /** Emitted with the new file name when a new display name moved the file. */
  readonly renamed = output<string>();

  protected readonly name = signal('');
  protected readonly color = signal('');
  protected readonly variables = signal<VariableRow[]>([]);
  /** Local overrides: git ignored, they win over the shared variables. */
  protected readonly localVariables = signal<VariableRow[]>([]);
  protected readonly localNames = computed(
    () => new Set(this.localVariables().map((variable) => variable.name.trim())),
  );
  protected readonly secretNames = signal<string[]>([]);
  protected readonly secretStates = signal<SecretState[]>([]);
  protected readonly secretValues = signal<Record<string, string>>({});
  protected readonly newSecretName = signal('');
  protected readonly isLoading = signal(false);
  protected readonly isSaving = signal(false);
  /** Whether the environment lives in a git ignored local file, not shared. */
  protected readonly isPrivate = signal(false);
  protected readonly message = signal<string | null>(null);
  protected readonly errorMessage = signal<string | null>(null);
  /** The file name always follows the display name. */
  protected readonly targetFileName = computed(() => slugify(this.name()));
  /** Shown once the editor reloads under a new file name. */
  private carriedMessage: string | null = null;

  /** Path of the environment file on disk, local when it is private. */
  protected readonly filePath = computed(() => this.environmentFilePath(this.fileName()));
  /** Path of the file a rename moves to, local when the environment is private. */
  protected readonly targetFilePath = computed(() =>
    this.environmentFilePath(this.targetFileName()),
  );

  private environmentFilePath(fileName: string): string {
    const directory = this.scope() ? `${this.scope()}/` : '';
    const base = this.isPrivate() ? `${fileName}.local` : fileName;
    return `${directory}environments/${base}.yaml`;
  }

  constructor() {
    effect(() => {
      const scope = this.scope();
      const fileName = this.fileName();
      untracked(() => {
        this.message.set(this.carriedMessage);
        this.carriedMessage = null;
        void this.loadEnvironment(scope, fileName);
      });
    });
  }

  /** Reserved variables not declared yet, offered as shortcuts. */
  protected readonly missingReserved = computed(() => {
    const declared = new Set(
      [...this.variables(), ...this.localVariables()].map((variable) => variable.name.trim()),
    );
    return Object.values(RESERVED_VARIABLES).filter((definition) => !declared.has(definition.name));
  });

  protected reserved(name: string): ReservedVariable | null {
    return reservedVariable(name);
  }

  protected selectOptions(definition: ReservedVariable): SelectOption[] {
    return definition.options.map((option) => ({ ...option, hint: option.value }));
  }

  /** The option matching a value written in another accepted spelling. */
  protected reservedValue(variable: VariableRow): string {
    return parseReservedValue(variable.name, variable.value) ?? variable.value.trim();
  }

  protected addReserved(name: string): void {
    const definition = RESERVED_VARIABLES[name];
    this.variables.update((variables) => [
      ...variables,
      { name, value: definition?.options[0]?.value ?? '' },
    ]);
  }

  protected addVariable(list: VariableList = 'shared'): void {
    this.rows(list).update((variables) => [...variables, { name: '', value: '' }]);
  }

  protected updateVariable(
    index: number,
    field: 'name' | 'value',
    value: string,
    list: VariableList = 'shared',
  ): void {
    this.rows(list).update((variables) =>
      variables.map((variable, position) =>
        position === index ? { ...variable, [field]: value } : variable,
      ),
    );
  }

  protected removeVariable(index: number, list: VariableList = 'shared'): void {
    this.rows(list).update((variables) =>
      variables.filter((_variable, position) => position !== index),
    );
  }

  private rows(list: VariableList) {
    return list === 'local' ? this.localVariables : this.variables;
  }

  protected addSecretName(): void {
    const name = this.newSecretName().trim();
    if (name === '') {
      return;
    }
    if (this.secretNames().includes(name)) {
      this.errorMessage.set(`The secret ${name} is already declared.`);
      return;
    }

    this.secretNames.update((names) => [...names, name]);
    this.newSecretName.set('');
  }

  protected removeSecretName(name: string): void {
    this.secretNames.update((names) => names.filter((entry) => entry !== name));
  }

  protected updateSecretValue(name: string, value: string): void {
    this.secretValues.update((values) => ({ ...values, [name]: value }));
  }

  protected secretIsSet(name: string): boolean {
    return this.secretStates().some((state) => state.Name === name && state.IsSet);
  }

  /** Saves the environment, as the Save button does; used by Ctrl+S / Cmd+S. */
  save(): Promise<void> {
    return this.saveEnvironment();
  }

  protected async saveEnvironment(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const fileName = this.fileName();
    if (!rootPath || this.isSaving()) {
      return;
    }

    const environment = this.collectEnvironment();
    const localVariables = this.collectVariables(this.localVariables(), 'local override');
    if (!environment || !localVariables) {
      return;
    }
    for (const secretName of this.secretNames()) {
      if (Object.hasOwn(localVariables, secretName)) {
        this.errorMessage.set(`${secretName} is a secret: store its value as a secret instead.`);
        return;
      }
    }

    const targetFileName = this.targetFileName();
    if (targetFileName === '') {
      this.errorMessage.set('Choose a name with at least one letter or digit.');
      return;
    }

    this.isSaving.set(true);
    this.errorMessage.set(null);
    this.message.set(null);

    try {
      // Rename first, so a name already taken stops the save before anything
      // is written.
      if (targetFileName !== fileName) {
        await this.renameFile(rootPath, fileName, targetFileName);
      }
      if (this.isPrivate()) {
        await EnvironmentsService.SavePrivate(rootPath, this.scope(), targetFileName, environment);
      } else {
        await EnvironmentsService.Save(rootPath, this.scope(), targetFileName, environment);
        await EnvironmentsService.SaveLocal(rootPath, this.scope(), targetFileName, localVariables);
      }
      notifyEnvironmentsChanged();
      this.saved.emit();
      if (targetFileName !== fileName) {
        this.carriedMessage = `Environment saved as ${targetFileName}.yaml.`;
        this.renamed.emit(targetFileName);
      } else {
        await this.loadEnvironment(this.scope(), fileName);
        this.message.set('Environment saved.');
      }
    } catch (error) {
      this.errorMessage.set(
        describeError(error).includes('already exists')
          ? `Another environment already uses the file ${targetFileName}.yaml. Choose another name.`
          : describeError(error),
      );
    } finally {
      this.isSaving.set(false);
    }
  }

  /** Moves the files, the secret values and the selection to a new file name. */
  private async renameFile(
    rootPath: string,
    fileName: string,
    targetFileName: string,
  ): Promise<void> {
    await EnvironmentsService.Rename(rootPath, this.scope(), fileName, targetFileName);
    await SecretsService.MoveSecrets(rootPath, this.scope(), fileName, targetFileName);
    if (this.environmentStore.selected() === fileName) {
      this.environmentStore.select(targetFileName);
    }
  }

  /**
   * Copies the environment as saved, its local overrides and its stored
   * secret values, under a name the user chooses.
   */
  protected async cloneEnvironment(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const sourceFileName = this.fileName();
    if (!rootPath) {
      return;
    }

    const name = await this.prompt.ask({
      title: `Clone “${this.name() || sourceFileName}”`,
      message:
        'The copy gets the saved variables, local overrides and stored secrets. Unsaved changes are not copied.',
      label: 'Name of the copy',
      icon: 'copy',
      value: `${this.name() || sourceFileName} copy`,
      confirmLabel: 'Clone environment',
    });
    if (name === null) {
      return;
    }

    const targetFileName = slugify(name);
    if (targetFileName === '') {
      this.errorMessage.set('Choose a name with at least one letter or digit.');
      return;
    }

    this.errorMessage.set(null);
    this.message.set(null);

    try {
      await EnvironmentsService.Clone(rootPath, this.scope(), sourceFileName, targetFileName, name);
    } catch (error) {
      this.errorMessage.set(
        describeError(error).includes('already exists')
          ? `An environment named ${targetFileName} already exists.`
          : describeError(error),
      );
      return;
    }

    notifyEnvironmentsChanged();
    try {
      await SecretsService.CopySecrets(rootPath, this.scope(), sourceFileName, targetFileName);
    } catch (error) {
      this.errorMessage.set(
        `Cloned, but the secret values were not copied: ${describeError(error)}`,
      );
    }
    this.cloned.emit(targetFileName);
  }

  /**
   * Deletes the environment after confirmation, together with its local
   * overrides and the secret values stored in the keychain.
   */
  protected async deleteEnvironment(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const fileName = this.fileName();
    if (!rootPath) {
      return;
    }

    const targetFile = this.isPrivate() ? `${fileName}.local.yaml` : `${fileName}.yaml`;
    const overrides = this.isPrivate() ? '' : ', its local overrides';
    const message = `${this.scope() ? `${this.scope()}/` : ''}environments/${targetFile}${overrides} and its stored secret values will be removed.`;

    const confirmed = await this.confirmation.ask({
      title: `Delete the environment “${this.name() || fileName}”?`,
      message,
      confirmLabel: 'Delete environment',
      danger: true,
    });
    if (!confirmed) {
      return;
    }

    this.errorMessage.set(null);
    this.message.set(null);

    try {
      for (const name of this.secretNames()) {
        if (this.secretIsSet(name)) {
          await SecretsService.DeleteSecret(rootPath, this.scope(), fileName, name);
        }
      }
      await EnvironmentsService.Delete(rootPath, this.scope(), fileName);
      if (this.environmentStore.selected() === fileName) {
        this.environmentStore.select('');
      }
      notifyEnvironmentsChanged();
      this.deleted.emit();
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  protected async storeSecret(name: string): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const value = this.secretValues()[name] ?? '';
    if (!rootPath || value === '') {
      return;
    }

    this.errorMessage.set(null);
    this.message.set(null);

    try {
      await SecretsService.SetSecret(rootPath, this.scope(), this.fileName(), name, value);
      this.updateSecretValue(name, '');
      await this.loadSecretStates();
      this.message.set(`Secret ${name} stored.`);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  protected async clearSecret(name: string): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      return;
    }

    this.errorMessage.set(null);
    this.message.set(null);

    try {
      await SecretsService.DeleteSecret(rootPath, this.scope(), this.fileName(), name);
      await this.loadSecretStates();
      this.message.set(`Secret ${name} cleared.`);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  private collectEnvironment(): Environment | null {
    const name = this.name().trim();
    if (name === '') {
      this.errorMessage.set('An environment needs a name.');
      return null;
    }

    const variables = this.collectVariables(this.variables(), 'variable');
    if (!variables) {
      return null;
    }

    for (const secretName of this.secretNames()) {
      if (Object.hasOwn(variables, secretName)) {
        this.errorMessage.set(`${secretName} cannot be both a plain variable and a secret.`);
        return null;
      }
    }

    return {
      Name: name,
      Variables: variables,
      SecretVariables: this.secretNames(),
      Color: this.color(),
    };
  }

  private collectVariables(rows: VariableRow[], label: string): Record<string, string> | null {
    const variables: Record<string, string> = {};
    for (const variable of rows) {
      const variableName = variable.name.trim();
      if (variableName === '') {
        continue;
      }
      if (Object.hasOwn(variables, variableName)) {
        this.errorMessage.set(`The ${label} ${variableName} is declared twice.`);
        return null;
      }
      variables[variableName] = variable.value;
    }
    return variables;
  }

  private async loadEnvironment(scope: string, fileName: string): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath || fileName === '') {
      return;
    }

    this.isLoading.set(true);
    this.errorMessage.set(null);

    try {
      const resolved = await EnvironmentsService.Load(rootPath, scope, fileName);
      this.name.set(resolved.Name);
      this.isPrivate.set(resolved.Private);
      this.color.set(
        resolved.Private ? (resolved.Local?.Color ?? '') : (resolved.Shared.Color ?? ''),
      );
      if (resolved.Private) {
        this.variables.set(toRows(resolved.Local?.Variables));
        this.localVariables.set([]);
      } else {
        this.variables.set(toRows(resolved.Shared.Variables));
        this.localVariables.set(toRows(resolved.Local?.Variables));
      }
      this.secretNames.set(
        resolved.Private
          ? (resolved.Local?.SecretVariables ?? [])
          : (resolved.Shared.SecretVariables ?? []),
      );
      this.secretValues.set({});
      this.newSecretName.set('');
      await this.loadSecretStates();
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  private async loadSecretStates(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      return;
    }

    try {
      this.secretStates.set(
        (await SecretsService.SecretStates(rootPath, this.scope(), this.fileName())) ?? [],
      );
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }
}

function toRows(values: Record<string, string | undefined> | null | undefined): VariableRow[] {
  return Object.entries(values ?? {})
    .map(([name, value]) => ({ name, value: value ?? '' }))
    .sort((first, second) => first.name.localeCompare(second.name));
}

/** Tells the picker and the environments page to reload. */
export function notifyEnvironmentsChanged(): void {
  window.dispatchEvent(new Event(ENVIRONMENTS_CHANGED_EVENT));
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
