import { DestroyRef, Injectable, computed, effect, inject, signal } from '@angular/core';
import * as HTTPClientService from '@bindings/github.com/awake-client/awake/src/features/httpclient/service';
import { COLLECTIONS_CHANGED_EVENT } from './collection-events';
import { EnvironmentStore } from './environment-store';
import { TabsStore } from './tabs-store';
import { WorkspaceStore } from './workspace-store';

/** Where a variable value comes from, by increasing precedence. */
/**
 * Where a variable value comes from: workspace, collection, environment,
 * collection environment (each possibly "(local)"), secret or dynamic.
 */
export type VariableSource = string;

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
 * Exposes the variables visible to the request on screen with the active
 * environment, as resolved by the backend: workspace, collection, global
 * environment, collection environment, local overrides and secrets. Dynamic
 * variables such as `{{$uuid}}` are listed too.
 */
@Injectable({ providedIn: 'root' })
export class VariableStore {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly environmentStore = inject(EnvironmentStore);
  private readonly tabsStore = inject(TabsStore);

  private readonly environmentVariables = signal<VariableInfo[]>([]);
  /** Collection whose settings are on screen, when no request is. */
  private readonly collectionContext = signal<string | null>(null);

  /**
   * Path whose collection gives the variables: the request on screen, or a
   * path inside the collection being configured.
   */
  private readonly contextPath = computed(() => {
    const collection = this.collectionContext();
    if (collection) {
      return `${collection}/collection.yaml`;
    }
    return this.tabsStore.active() ?? '';
  });
  private readonly dynamicVariables = signal<VariableInfo[]>([]);

  /** Every variable by name, the highest precedence layer winning. */
  readonly variables = computed(() => {
    const merged = new Map<string, VariableInfo>();
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

    // The variables depend on what is on screen: a request, or the settings
    // of a collection. Both may refine the selected environment.
    effect(() => {
      const rootPath = this.workspaceStore.rootPath();
      const environment = this.environmentStore.selected();
      void this.loadEnvironment(rootPath, this.contextPath(), environment);
    });

    // Collection variables and environments change from other screens.
    const reload = () =>
      void this.loadEnvironment(
        this.workspaceStore.rootPath(),
        this.contextPath(),
        this.environmentStore.selected(),
      );
    window.addEventListener('awake:environments-changed', reload);
    window.addEventListener(COLLECTIONS_CHANGED_EVENT, reload);
    inject(DestroyRef).onDestroy(() => {
      window.removeEventListener('awake:environments-changed', reload);
      window.removeEventListener(COLLECTIONS_CHANGED_EVENT, reload);
    });
  }

  /** Resolves variables for a collection while its settings are on screen. */
  useCollection(collectionPath: string | null): void {
    this.collectionContext.set(collectionPath);
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

  private async loadEnvironment(
    rootPath: string | null,
    requestPath: string,
    fileName: string,
  ): Promise<void> {
    if (!rootPath) {
      this.environmentVariables.set([]);
      return;
    }

    try {
      const resolved =
        (await HTTPClientService.ResolveVariables(rootPath, requestPath, fileName)) ?? [];
      this.environmentVariables.set(
        resolved.map((variable) => ({
          name: variable.Name,
          value: variable.Secret ? 'secret' : variable.Value,
          source: variable.Secret ? 'secret' : variable.Source,
        })),
      );
    } catch {
      this.environmentVariables.set([]);
    }
  }
}
