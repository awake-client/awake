import { Component, DestroyRef, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import * as EnvironmentsService from '@bindings/github.com/clementherve/awake/src/features/environments/service';
import { Summary } from '@bindings/github.com/clementherve/awake/src/features/environments/models';
import * as CollectionsService from '@bindings/github.com/clementherve/awake/src/features/collections/service';
import { requestEntryCreation } from '@core/collection-events';
import { WorkspaceStore } from '@core/workspace-store';
import { Icon } from '@shared/icon';
import { ENVIRONMENTS_CHANGED_EVENT, EnvironmentEditor } from './environment-editor';

/**
 * Lists the environments of a scope, global or of a collection, and edits
 * the selected one.
 */
@Component({
  selector: 'app-environments-panel',
  imports: [FormsModule, Icon, EnvironmentEditor],
  templateUrl: './environments-panel.html',
  styleUrl: './environments-panel.css',
})
export class EnvironmentsPanel {
  private readonly workspaceStore = inject(WorkspaceStore);

  protected readonly environments = signal<Summary[]>([]);
  /** Empty for the global environments, else the collection being edited. */
  protected readonly scope = signal('');
  protected readonly scopes = signal<{ path: string; name: string }[]>([]);
  /** Global environment names, to show which collection ones override them. */
  protected readonly globalNames = signal<Set<string>>(new Set());
  protected readonly selectedFileName = signal('');
  protected readonly message = signal<string | null>(null);
  protected readonly errorMessage = signal<string | null>(null);

  constructor() {
    // Opened right after a creation: show the new environment.
    const query = inject(ActivatedRoute).snapshot.queryParamMap;
    const requested = query.get('environment');
    this.scope.set(query.get('scope') ?? '');
    void this.loadEnvironments().then(() => {
      if (requested) {
        this.selectEnvironment(requested);
      }
    });
    void this.loadScopes();

    // Reload after any change, and select an environment just created from
    // the new entry dialog.
    const environmentsChanged = (event: Event) => {
      const detail = (event as CustomEvent<{ created?: string; scope?: string } | null>).detail;
      const created = detail?.created;
      if (created) {
        this.scope.set(detail?.scope ?? '');
      }
      void this.loadEnvironments().then(() => {
        if (created) {
          this.selectedFileName.set(created);
        }
      });
    };
    window.addEventListener(ENVIRONMENTS_CHANGED_EVENT, environmentsChanged);
    inject(DestroyRef).onDestroy(() =>
      window.removeEventListener(ENVIRONMENTS_CHANGED_EVENT, environmentsChanged),
    );
  }

  protected selectEnvironment(fileName: string): void {
    if (fileName !== '') {
      this.message.set(null);
      this.selectedFileName.set(fileName);
    }
  }

  protected showClone(fileName: string): void {
    this.selectedFileName.set(fileName);
    this.message.set('Environment cloned.');
  }

  protected forgetSelection(): void {
    this.selectedFileName.set('');
    this.message.set('Environment deleted.');
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
      this.errorMessage.set(error instanceof Error ? error.message : String(error));
    }
  }
}
