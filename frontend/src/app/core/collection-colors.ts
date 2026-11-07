import { Injectable, effect, inject, signal } from '@angular/core';
import * as CollectionsService from '@bindings/github.com/clementherve/awake/src/features/collections/service';
import { COLLECTIONS_CHANGED_EVENT } from './collection-events';
import { WorkspaceStore } from './workspace-store';

/** Roots holding collections: shared through git, or private. */
const COLLECTION_ROOTS = ['', 'local'];

/**
 * Colors chosen for the collections of the open workspace, by collection
 * path, so a request can be tinted with the color of its collection. They
 * reload with the workspace and after any change to the collections.
 */
@Injectable({ providedIn: 'root' })
export class CollectionColors {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly colors = signal<Record<string, string>>({});

  constructor() {
    effect(() => void this.load(this.workspaceStore.rootPath()));
    window.addEventListener(COLLECTIONS_CHANGED_EVENT, () =>
      this.load(this.workspaceStore.rootPath()),
    );
  }

  /** The color of the collection holding a request, or an empty string. */
  colorOf(requestPath: string): string {
    const scope = requestPath.split('/').slice(0, 2).join('/');
    return this.colors()[scope] ?? '';
  }

  private async load(rootPath: string | null): Promise<void> {
    if (!rootPath) {
      this.colors.set({});
      return;
    }
    try {
      const lists = await Promise.all(
        COLLECTION_ROOTS.map((root) => CollectionsService.List(rootPath, root)),
      );
      const colors: Record<string, string> = {};
      for (const entry of lists.flatMap((entries) => entries ?? [])) {
        if (entry.Color) {
          colors[entry.Path] = entry.Color;
        }
      }
      this.colors.set(colors);
    } catch {
      // Tabs stay neutral when the collections cannot be listed; the tree
      // reports the failure itself.
      this.colors.set({});
    }
  }
}
