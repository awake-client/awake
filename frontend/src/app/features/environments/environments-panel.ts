import { Component, DestroyRef, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import * as EnvironmentsService from '@bindings/github.com/clementherve/awake/src/features/environments/service';
import {
  Environment,
  Summary,
} from '@bindings/github.com/clementherve/awake/src/features/environments/models';
import * as CollectionsService from '@bindings/github.com/clementherve/awake/src/features/collections/service';
import * as SecretsService from '@bindings/github.com/clementherve/awake/src/features/secrets/service';
import { SecretState } from '@bindings/github.com/clementherve/awake/src/features/secrets/models';
import { requestEntryCreation } from '@core/collection-events';
import { Confirmation } from '@core/confirmation';
import { EnvironmentStore } from '@core/environment-store';
import { WorkspaceStore } from '@core/workspace-store';
import { ColorPicker } from '@shared/color-picker';
import { Icon } from '@shared/icon';

interface VariableRow {
  name: string;
  value: string;
}

/**
 * Edits the shared environments of a workspace and stores declared secret
 * values in the operating system keychain.
 */
@Component({
  selector: 'app-environments-panel',
  imports: [FormsModule, Icon, ColorPicker],
  templateUrl: './environments-panel.html',
  styleUrl: './environments-panel.css',
})
export class EnvironmentsPanel {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly environmentStore = inject(EnvironmentStore);
  private readonly confirmation = inject(Confirmation);

  protected readonly environments = signal<Summary[]>([]);
  /** Empty for the global environments, else the collection being edited. */
  protected readonly scope = signal('');
  protected readonly scopes = signal<{ path: string; name: string }[]>([]);
  /** Global environment names, to show which collection ones override them. */
  protected readonly globalNames = signal<Set<string>>(new Set());
  protected readonly selectedFileName = signal('');
  protected readonly name = signal('');
  protected readonly color = signal('');
  protected readonly variables = signal<VariableRow[]>([]);
  protected readonly secretNames = signal<string[]>([]);
  protected readonly secretStates = signal<SecretState[]>([]);
  protected readonly secretValues = signal<Record<string, string>>({});
  protected readonly newSecretName = signal('');
  protected readonly isLoading = signal(false);
  protected readonly isSaving = signal(false);
  protected readonly message = signal<string | null>(null);
  protected readonly errorMessage = signal<string | null>(null);

  constructor() {
    // Opened right after a creation: show the new environment.
    const query = inject(ActivatedRoute).snapshot.queryParamMap;
    const requested = query.get('environment');
    this.scope.set(query.get('scope') ?? '');
    void this.loadEnvironments().then(() => {
      if (requested) {
        void this.selectEnvironment(requested);
      }
    });
    void this.loadScopes();

    // Select the environment just created from the new entry dialog.
    const environmentsChanged = (event: Event) => {
      const detail = (event as CustomEvent<{ created?: string; scope?: string } | null>).detail;
      const created = detail?.created;
      if (created) {
        this.scope.set(detail?.scope ?? '');
        void this.loadEnvironments().then(() => {
          this.selectedFileName.set('');
          return this.selectEnvironment(created);
        });
      }
    };
    window.addEventListener('awake:environments-changed', environmentsChanged);
    inject(DestroyRef).onDestroy(() =>
      window.removeEventListener('awake:environments-changed', environmentsChanged),
    );
  }

  protected async selectEnvironment(fileName: string): Promise<void> {
    if (fileName === '' || fileName === this.selectedFileName()) {
      return;
    }

    this.selectedFileName.set(fileName);
    await this.loadEnvironment(fileName);
  }

  protected newEnvironment(): void {
    requestEntryCreation({ kind: 'environment', scope: this.scope() });
  }

  protected async selectScope(scope: string): Promise<void> {
    this.scope.set(scope);
    this.selectedFileName.set('');
    this.message.set(null);
    this.errorMessage.set(null);
    await this.loadEnvironments();
  }

  private async loadScopes(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      return;
    }
    const [shared, privateCollections] = await Promise.all([
      CollectionsService.List(rootPath, ''),
      CollectionsService.List(rootPath, 'local'),
    ]);
    this.scopes.set(
      [...(shared ?? []), ...(privateCollections ?? [])].map((collection) => ({
        path: collection.Path,
        name: collection.Path.startsWith('local/')
          ? `${collection.Name} (private)`
          : collection.Name,
      })),
    );
  }

  /**
   * Deletes the environment after confirmation, together with its local
   * overrides and the secret values stored in the keychain.
   */
  protected async deleteEnvironment(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const fileName = this.selectedFileName();
    if (!rootPath || !fileName) {
      return;
    }

    const confirmed = await this.confirmation.ask({
      title: `Delete the environment “${this.name() || fileName}”?`,
      message: `${this.scope() ? `${this.scope()}/` : ''}environments/${fileName}.yaml, its local overrides and its stored secret values will be removed.`,
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
      this.selectedFileName.set('');
      await this.loadEnvironments();
      this.notifyChange();
      this.message.set('Environment deleted.');
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  protected addVariable(): void {
    this.variables.update((variables) => [...variables, { name: '', value: '' }]);
  }

  protected updateVariable(index: number, field: 'name' | 'value', value: string): void {
    this.variables.update((variables) =>
      variables.map((variable, position) =>
        position === index ? { ...variable, [field]: value } : variable,
      ),
    );
  }

  protected removeVariable(index: number): void {
    this.variables.update((variables) =>
      variables.filter((_variable, position) => position !== index),
    );
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

  protected async saveEnvironment(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const fileName = this.selectedFileName();
    if (!rootPath || !fileName || this.isSaving()) {
      return;
    }

    const name = this.name().trim();
    if (name === '') {
      this.errorMessage.set('An environment needs a name.');
      return;
    }

    const variables: Record<string, string> = {};
    for (const variable of this.variables()) {
      const variableName = variable.name.trim();
      if (variableName === '') {
        continue;
      }
      if (Object.hasOwn(variables, variableName)) {
        this.errorMessage.set(`The variable ${variableName} is declared twice.`);
        return;
      }
      variables[variableName] = variable.value;
    }

    for (const secretName of this.secretNames()) {
      if (Object.hasOwn(variables, secretName)) {
        this.errorMessage.set(`${secretName} cannot be both a plain variable and a secret.`);
        return;
      }
    }

    const environment: Environment = {
      Name: name,
      Variables: variables,
      SecretVariables: this.secretNames(),
      Color: this.color(),
    };

    this.isSaving.set(true);
    this.errorMessage.set(null);
    this.message.set(null);

    try {
      await EnvironmentsService.Save(rootPath, this.scope(), fileName, environment);
      await this.loadEnvironments();
      await this.loadEnvironment(fileName);
      this.notifyChange();
      this.message.set('Environment saved.');
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isSaving.set(false);
    }
  }

  protected async storeSecret(name: string): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const fileName = this.selectedFileName();
    const value = this.secretValues()[name] ?? '';
    if (!rootPath || !fileName || value === '') {
      return;
    }

    this.errorMessage.set(null);
    this.message.set(null);

    try {
      await SecretsService.SetSecret(rootPath, this.scope(), fileName, name, value);
      this.updateSecretValue(name, '');
      await this.loadSecretStates(fileName);
      this.message.set(`Secret ${name} stored.`);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  protected async clearSecret(name: string): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const fileName = this.selectedFileName();
    if (!rootPath || !fileName) {
      return;
    }

    this.errorMessage.set(null);
    this.message.set(null);

    try {
      await SecretsService.DeleteSecret(rootPath, this.scope(), fileName, name);
      await this.loadSecretStates(fileName);
      this.message.set(`Secret ${name} cleared.`);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  private async loadEnvironments(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      return;
    }

    try {
      this.environments.set((await EnvironmentsService.List(rootPath, this.scope())) ?? []);
      const global = this.scope() ? ((await EnvironmentsService.List(rootPath, '')) ?? []) : [];
      this.globalNames.set(new Set(global.map((environment) => environment.FileName)));
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  private async loadEnvironment(fileName: string): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath || fileName === '') {
      return;
    }

    this.isLoading.set(true);
    this.errorMessage.set(null);

    try {
      const resolved = await EnvironmentsService.Load(rootPath, this.scope(), fileName);
      this.name.set(resolved.Name);
      this.color.set(resolved.Shared.Color ?? '');
      this.variables.set(
        Object.entries(resolved.Shared.Variables ?? {})
          .map(([name, value]) => ({ name, value: value ?? '' }))
          .sort((first, second) => first.name.localeCompare(second.name)),
      );
      this.secretNames.set(resolved.Shared.SecretVariables ?? []);
      this.secretValues.set({});
      await this.loadSecretStates(fileName);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  private async loadSecretStates(fileName: string): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      return;
    }

    try {
      this.secretStates.set(
        (await SecretsService.SecretStates(rootPath, this.scope(), fileName)) ?? [],
      );
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  private notifyChange(): void {
    window.dispatchEvent(new Event('awake:environments-changed'));
  }
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
