import { DestroyRef, Injectable, computed, effect, inject, signal } from '@angular/core';
import * as EnvironmentsService from '@bindings/github.com/clementherve/awake/src/features/environments/service';
import * as HTTPClientService from '@bindings/github.com/clementherve/awake/src/features/httpclient/service';
import { EnvironmentStore } from './environment-store';
import { WorkspaceStore } from './workspace-store';

/** Where a variable value comes from, by increasing precedence. */
export type VariableSource = 'workspace' | 'environment' | 'local' | 'secret' | 'dynamic';

/** A variable that can be referenced as `{{name}}`. */
export interface VariableInfo {
  name: string;
  /** Displayed value; secrets and dynamic variables only show a hint. */
  value: string;
  source: VariableSource;
}

/** One `{{…}}` reference found in a text. */
export interface VariableReference {
  name: string;
  start: number;
  end: number;
  variable: VariableInfo | undefined;
}

const REFERENCE_PATTERN = /\{\{\s*(\$?[A-Za-z_][A-Za-z0-9_]*)\s*\}\}/g;

/**
 * Exposes the variables available to requests with the active environment,
 * merged the same way the backend interpolates them: workspace variables,
 * then the shared environment, its local overrides and its secrets. Dynamic
 * variables such as `{{$uuid}}` are listed too.
 */
@Injectable({ providedIn: 'root' })
export class VariableStore {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly environmentStore = inject(EnvironmentStore);

  private readonly environmentVariables = signal<VariableInfo[]>([]);
  private readonly dynamicVariables = signal<VariableInfo[]>([]);

  /** Every variable by name, the highest precedence layer winning. */
  readonly variables = computed(() => {
    const merged = new Map<string, VariableInfo>();
    const workspaceVariables = this.workspaceStore.workspace()?.configuration.Variables ?? {};
    for (const [name, value] of Object.entries(workspaceVariables)) {
      merged.set(name, { name, value: value ?? '', source: 'workspace' });
    }
    for (const variable of this.environmentVariables()) {
      merged.set(variable.name, variable);
    }
    for (const variable of this.dynamicVariables()) {
      merged.set(variable.name, variable);
    }
    return merged;
  });

  /** Variables sorted for autocompletion: user variables before dynamic ones. */
  readonly suggestions = computed(() =>
    [...this.variables().values()].sort((first, second) => {
      const firstDynamic = first.source === 'dynamic';
      const secondDynamic = second.source === 'dynamic';
      if (firstDynamic !== secondDynamic) {
        return firstDynamic ? 1 : -1;
      }
      return first.name.localeCompare(second.name);
    }),
  );

  constructor() {
    void HTTPClientService.DynamicVariables()
      .then((variables) =>
        this.dynamicVariables.set(
          (variables ?? []).map((variable) => ({
            name: variable.Name,
            value: variable.Description,
            source: 'dynamic' as const,
          })),
        ),
      )
      .catch(() => this.dynamicVariables.set([]));

    effect(() => {
      const rootPath = this.workspaceStore.rootPath();
      const environment = this.environmentStore.selected();
      void this.loadEnvironment(rootPath, environment);
    });

    const reload = () =>
      void this.loadEnvironment(this.workspaceStore.rootPath(), this.environmentStore.selected());
    window.addEventListener('awake:environments-changed', reload);
    inject(DestroyRef).onDestroy(() =>
      window.removeEventListener('awake:environments-changed', reload),
    );
  }

  /** Finds every `{{…}}` reference of a text with its resolved variable. */
  references(text: string): VariableReference[] {
    const variables = this.variables();
    const references: VariableReference[] = [];
    for (const match of text.matchAll(REFERENCE_PATTERN)) {
      const start = match.index ?? 0;
      references.push({
        name: match[1],
        start,
        end: start + match[0].length,
        variable: variables.get(match[1]),
      });
    }
    return references;
  }

  /**
   * Replaces references with their values for a preview. Secrets stay masked
   * and dynamic variables keep their placeholder, since they change per send.
   */
  preview(text: string): string {
    const variables = this.variables();
    return text.replace(REFERENCE_PATTERN, (placeholder, name: string) => {
      const variable = variables.get(name);
      if (!variable || variable.source === 'dynamic') {
        return placeholder;
      }
      return variable.source === 'secret' ? '•••••' : variable.value;
    });
  }

  private async loadEnvironment(rootPath: string | null, fileName: string): Promise<void> {
    if (!rootPath || !fileName) {
      this.environmentVariables.set([]);
      return;
    }

    try {
      const resolved = await EnvironmentsService.Load(rootPath, fileName);
      const variables: VariableInfo[] = [];
      for (const [name, value] of Object.entries(resolved.Shared.Variables ?? {})) {
        variables.push({ name, value: value ?? '', source: 'environment' });
      }
      for (const [name, value] of Object.entries(resolved.Local?.Variables ?? {})) {
        variables.push({ name, value: value ?? '', source: 'local' });
      }
      const secrets = [
        ...(resolved.Shared.SecretVariables ?? []),
        ...(resolved.Local?.SecretVariables ?? []),
      ];
      for (const name of secrets) {
        variables.push({ name, value: 'secret', source: 'secret' });
      }
      this.environmentVariables.set(variables);
    } catch {
      this.environmentVariables.set([]);
    }
  }
}
