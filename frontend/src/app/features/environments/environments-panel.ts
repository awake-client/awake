import { Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import * as EnvironmentsService from '@bindings/github.com/clementherve/awake/src/features/environments/service';
import {
  Environment,
  Summary,
} from '@bindings/github.com/clementherve/awake/src/features/environments/models';
import * as SecretsService from '@bindings/github.com/clementherve/awake/src/features/secrets/service';
import { SecretState } from '@bindings/github.com/clementherve/awake/src/features/secrets/models';
import { WorkspaceStore } from '@core/workspace-store';

const FILE_NAME_PATTERN = /^[a-z0-9][a-z0-9_-]*$/;

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
  imports: [FormsModule],
  templateUrl: './environments-panel.html',
  styleUrl: './environments-panel.css',
})
export class EnvironmentsPanel {
  private readonly workspaceStore = inject(WorkspaceStore);

  protected readonly environments = signal<Summary[]>([]);
  protected readonly selectedFileName = signal('');
  protected readonly name = signal('');
  protected readonly variables = signal<VariableRow[]>([]);
  protected readonly secretNames = signal<string[]>([]);
  protected readonly secretStates = signal<SecretState[]>([]);
  protected readonly secretValues = signal<Record<string, string>>({});
  protected readonly newSecretName = signal('');
  protected readonly newFileName = signal('');
  protected readonly newEnvironmentName = signal('');
  protected readonly isLoading = signal(false);
  protected readonly isSaving = signal(false);
  protected readonly message = signal<string | null>(null);
  protected readonly errorMessage = signal<string | null>(null);

  constructor() {
    void this.loadEnvironments();
  }

  protected async selectEnvironment(fileName: string): Promise<void> {
    if (fileName === '' || fileName === this.selectedFileName()) {
      return;
    }

    this.selectedFileName.set(fileName);
    await this.loadEnvironment(fileName);
  }

  protected async createEnvironment(): Promise<void> {
    const fileName = this.newFileName().trim();
    const name = this.newEnvironmentName().trim() || fileName;
    if (!FILE_NAME_PATTERN.test(fileName)) {
      this.errorMessage.set(
        'The file name may only contain lowercase letters, digits, dashes and underscores.',
      );
      return;
    }
    if (name === '') {
      this.errorMessage.set('An environment needs a name.');
      return;
    }

    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      return;
    }

    this.isSaving.set(true);
    this.errorMessage.set(null);
    this.message.set(null);

    try {
      await EnvironmentsService.Save(rootPath, fileName, {
        Name: name,
        Variables: {},
        SecretVariables: [],
      });
      this.newFileName.set('');
      this.newEnvironmentName.set('');
      await this.loadEnvironments();
      this.selectedFileName.set(fileName);
      await this.loadEnvironment(fileName);
      this.notifyChange();
      this.message.set('Environment created.');
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isSaving.set(false);
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
    return this.secretStates().some(
      (state) => state.Name === name && state.IsSet,
    );
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
        this.errorMessage.set(
          `${secretName} cannot be both a plain variable and a secret.`,
        );
        return;
      }
    }

    const environment: Environment = {
      Name: name,
      Variables: variables,
      SecretVariables: this.secretNames(),
    };

    this.isSaving.set(true);
    this.errorMessage.set(null);
    this.message.set(null);

    try {
      await EnvironmentsService.Save(rootPath, fileName, environment);
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
      await SecretsService.SetSecret(rootPath, fileName, name, value);
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
      await SecretsService.DeleteSecret(rootPath, fileName, name);
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
      this.environments.set((await EnvironmentsService.List(rootPath)) ?? []);
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
      const resolved = await EnvironmentsService.Load(rootPath, fileName);
      this.name.set(resolved.Name);
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
        (await SecretsService.SecretStates(rootPath, fileName)) ?? [],
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
