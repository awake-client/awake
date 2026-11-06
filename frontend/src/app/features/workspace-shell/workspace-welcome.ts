import { Component, DestroyRef, computed, effect, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import * as CollectionsService from '@bindings/github.com/clementherve/awake/src/features/collections/service';
import { Entry } from '@bindings/github.com/clementherve/awake/src/features/collections/models';
import * as EnvironmentsService from '@bindings/github.com/clementherve/awake/src/features/environments/service';
import { Summary } from '@bindings/github.com/clementherve/awake/src/features/environments/models';
import { WorkspaceStore } from '@core/workspace-store';
import { requestEntryCreation } from '@core/collection-events';
import { Icon } from '@shared/icon';

/** Workspace overview shown in the editor area until a request is selected. */
@Component({
  selector: 'app-workspace-welcome',
  imports: [RouterLink, Icon],
  templateUrl: './workspace-welcome.html',
  styleUrl: './workspace-welcome.css',
})
export class WorkspaceWelcome {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly workspaceName = computed(
    () => this.workspaceStore.workspace()?.configuration.Name ?? 'Workspace',
  );
  protected readonly rootPath = computed(() => this.workspaceStore.rootPath() ?? '');

  protected readonly collections = signal<Entry[]>([]);
  protected readonly environments = signal<Summary[]>([]);
  protected readonly selectedCollection = signal<Entry | null>(null);
  protected readonly selectedDocumentation = signal('');
  protected readonly isLoading = signal(false);
  protected readonly errorMessage = signal<string | null>(null);


  constructor() {
    effect(() => {
      const rootPath = this.rootPath();
      if (rootPath) {
        void this.load(rootPath);
      }
    });

    const reload = () => {
      const rootPath = this.rootPath();
      if (rootPath) {
        void this.load(rootPath);
      }
    };

    window.addEventListener('awake:environments-changed', reload);
    window.addEventListener('awake:collections-changed', reload);
    this.destroyRef.onDestroy(() => {
      window.removeEventListener('awake:environments-changed', reload);
      window.removeEventListener('awake:collections-changed', reload);
    });
  }

  protected async load(rootPath: string): Promise<void> {
    this.isLoading.set(true);
    this.errorMessage.set(null);

    try {
      const [collections, environments] = await Promise.all([
        CollectionsService.List(rootPath, ''),
        EnvironmentsService.List(rootPath),
      ]);
      this.collections.set(collections ?? []);
      this.environments.set(environments ?? []);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  protected async openCollection(entry: Entry): Promise<void> {
    this.selectedCollection.set(entry);
    this.selectedDocumentation.set('');

    const rootPath = this.rootPath();
    if (!rootPath) {
      return;
    }

    try {
      const collection = await CollectionsService.LoadCollection(rootPath, entry.Path);
      this.selectedDocumentation.set(collection.Documentation ?? '');
    } catch {
      this.selectedDocumentation.set('');
    }

    window.dispatchEvent(new CustomEvent('awake:open-collection', { detail: entry.Path }));
  }

  protected newCollection(): void {
    requestEntryCreation({ kind: 'collection' });
  }

  /** Opens the new request dialog in the selected collection. */
  protected newRequest(): void {
    requestEntryCreation({
      kind: 'request',
      folder: this.selectedCollection()?.Path,
    });
  }

  /** Stable hue per collection so each card keeps its color. */
  protected collectionHue(entry: Entry): number {
    let hash = 0;
    for (const character of entry.Path) {
      hash = (hash * 31 + character.charCodeAt(0)) >>> 0;
    }
    return hash % 360;
  }
}


function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
