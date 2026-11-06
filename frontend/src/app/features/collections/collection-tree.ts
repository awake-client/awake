import { ScrollingModule } from '@angular/cdk/scrolling';
import { Component, DestroyRef, effect, inject, input, output, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router } from '@angular/router';
import { filter } from 'rxjs';
import * as CollectionsService from '@bindings/github.com/clementherve/awake/src/features/collections/service';
import {
  Entry,
  Kind,
} from '@bindings/github.com/clementherve/awake/src/features/collections/models';
import {
  COLLECTIONS_CHANGED_EVENT,
  CollectionsChange,
  CreationKind,
  requestEntryCreation,
} from '@core/collection-events';
import { Icon } from '@shared/icon';

const ROW_HEIGHT = 28;

/** Short labels keep the method column narrow. */
const METHOD_LABELS: Record<string, string> = {
  DELETE: 'DEL',
  OPTIONS: 'OPT',
};

/** One visible line of the flattened collection tree. */
interface TreeRow {
  entry: Entry;
  depth: number;
  expanded: boolean;
}

/**
 * Lists the collection tree and lazily loads directories as they expand.
 * Only expanded rows are kept in memory and rendered through a virtual
 * viewport, so a workspace with thousands of requests stays responsive.
 * Creation itself happens in the new entry dialog.
 */
@Component({
  selector: 'app-collection-tree',
  imports: [ScrollingModule, Icon],
  templateUrl: './collection-tree.html',
  styleUrl: './collection-tree.css',
})
export class CollectionTree {
  private readonly childrenCache = new Map<string, Entry[]>();
  private readonly expandedPaths = new Set<string>();
  private readonly destroyRef = inject(DestroyRef);
  private readonly router = inject(Router);

  readonly workspaceRoot = input('');
  readonly requestSelected = output<string>();

  protected readonly rowHeight = ROW_HEIGHT;
  protected readonly rows = signal<TreeRow[]>([]);
  protected readonly isLoading = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly selectedPath = signal<string | null>(null);
  protected readonly selectedFolder = signal<string | null>(null);
  protected readonly isMenuOpen = signal(false);

  constructor() {
    effect(() => {
      const workspaceRoot = this.workspaceRoot();
      if (workspaceRoot) {
        void this.reload(workspaceRoot);
      }
    });

    const collectionsChanged = (event: Event) => {
      const change = (event as CustomEvent<CollectionsChange | undefined>).detail;
      void this.applyChange(change ?? {});
    };
    const openCollection = (event: Event) => {
      const path = (event as CustomEvent<string>).detail;
      if (!path) {
        return;
      }
      this.selectedFolder.set(path);
      this.selectedPath.set(null);
      void this.revealDirectory(path);
    };
    const closeMenu = () => this.isMenuOpen.set(false);

    // Keep the highlighted request in sync with the page on screen, whether it
    // was opened from the tree, the search palette or the history.
    this.router.events
      .pipe(
        filter((event) => event instanceof NavigationEnd),
        takeUntilDestroyed(),
      )
      .subscribe(() => this.syncSelectionWithRoute());

    window.addEventListener(COLLECTIONS_CHANGED_EVENT, collectionsChanged);
    window.addEventListener('awake:open-collection', openCollection);
    document.addEventListener('click', closeMenu);
    this.destroyRef.onDestroy(() => {
      window.removeEventListener(COLLECTIONS_CHANGED_EVENT, collectionsChanged);
      window.removeEventListener('awake:open-collection', openCollection);
      document.removeEventListener('click', closeMenu);
    });
  }

  protected methodLabel(method: string | undefined): string {
    const normalized = (method || 'GET').toUpperCase();
    return METHOD_LABELS[normalized] ?? normalized;
  }

  /** Depths at which a vertical guide is drawn to the left of a row. */
  protected indentGuides(row: TreeRow): number[] {
    return Array.from({ length: row.depth }, (_, index) => index);
  }

  protected isRequest(row: TreeRow): boolean {
    return row.entry.Kind === Kind.KindRequest;
  }

  protected async activate(row: TreeRow): Promise<void> {
    if (this.isRequest(row)) {
      this.selectedPath.set(row.entry.Path);
      this.selectedFolder.set(null);
      this.requestSelected.emit(row.entry.Path);
      return;
    }

    this.selectedFolder.set(row.entry.Path);
    await this.toggle(row);
  }

  /**
   * Reloads the tree from disk and expands again the folders that were open,
   * so a refresh never collapses what the user was looking at.
   */
  protected async reload(workspaceRoot: string): Promise<void> {
    this.isLoading.set(true);
    this.errorMessage.set(null);

    const previouslyExpanded = [...this.expandedPaths].sort(
      (first, second) => first.split('/').length - second.split('/').length,
    );

    try {
      const entries = (await CollectionsService.List(workspaceRoot, '')) ?? [];

      this.childrenCache.clear();
      this.expandedPaths.clear();
      this.childrenCache.set('', entries);
      this.rows.set(this.toRows(entries, 0));

      for (const path of previouslyExpanded) {
        const row = this.rows().find((candidate) => candidate.entry.Path === path);
        if (row && !row.expanded) {
          await this.toggle(row);
        }
      }
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  protected refresh(): void {
    const workspaceRoot = this.workspaceRoot();
    if (workspaceRoot) {
      void this.reload(workspaceRoot);
    }
  }

  protected toggleMenu(event: Event): void {
    event.stopPropagation();
    this.isMenuOpen.update((open) => !open);
  }

  /** Opens the creation dialog with the current selection as location. */
  protected create(kind: CreationKind): void {
    this.isMenuOpen.set(false);
    requestEntryCreation({ kind, folder: this.currentFolder() });
  }

  /** Opens the creation dialog for a folder row without toggling the row. */
  protected createIn(event: Event, kind: CreationKind, row: TreeRow): void {
    event.stopPropagation();
    this.selectedFolder.set(row.entry.Path);
    this.selectedPath.set(null);
    requestEntryCreation({ kind, folder: row.entry.Path });
  }

  protected trackByPath(_index: number, row: TreeRow): string {
    return row.entry.Path;
  }

  private syncSelectionWithRoute(): void {
    const prefix = '/workspace/request/';
    const url = this.router.url.split(/[?#]/)[0];
    if (url.startsWith(prefix)) {
      this.selectedPath.set(decodeURIComponent(url.slice(prefix.length)));
      this.selectedFolder.set(null);
    } else {
      this.selectedPath.set(null);
    }
  }

  private currentFolder(): string | undefined {
    const selectedFolder = this.selectedFolder();
    if (selectedFolder) {
      return selectedFolder;
    }
    const selectedPath = this.selectedPath();
    if (selectedPath && selectedPath.includes('/')) {
      return selectedPath.slice(0, selectedPath.lastIndexOf('/'));
    }
    return undefined;
  }

  /** Reloads after a change and reveals the entry that was just created. */
  private async applyChange(change: CollectionsChange): Promise<void> {
    const workspaceRoot = this.workspaceRoot();
    if (!workspaceRoot) {
      return;
    }
    await this.reload(workspaceRoot);

    const created = change.created;
    if (!created) {
      return;
    }
    if (change.kind === 'request') {
      await this.revealDirectory(created.slice(0, created.lastIndexOf('/')));
      this.selectedPath.set(created);
      this.selectedFolder.set(null);
    } else {
      await this.revealDirectory(created.slice(0, created.lastIndexOf('/')));
      this.selectedFolder.set(created);
      this.selectedPath.set(null);
    }
  }

  private async toggle(row: TreeRow): Promise<void> {
    const rows = this.rows();
    const index = rows.findIndex((candidate) => candidate.entry.Path === row.entry.Path);
    if (index < 0) {
      return;
    }

    if (rows[index].expanded) {
      this.collapse(index, rows);
      return;
    }

    const workspaceRoot = this.workspaceRoot();
    if (!workspaceRoot) {
      return;
    }

    let children = this.childrenCache.get(row.entry.Path);
    if (!children) {
      try {
        children = (await CollectionsService.List(workspaceRoot, row.entry.Path)) ?? [];
      } catch (error) {
        this.errorMessage.set(describeError(error));
        return;
      }
      this.childrenCache.set(row.entry.Path, children);
    }

    const currentRows = this.rows();
    const currentIndex = currentRows.findIndex(
      (candidate) => candidate.entry.Path === row.entry.Path,
    );
    if (currentIndex < 0 || currentRows[currentIndex].expanded) {
      return;
    }

    const updatedRows = [...currentRows];
    updatedRows[currentIndex] = { ...currentRows[currentIndex], expanded: true };
    updatedRows.splice(currentIndex + 1, 0, ...this.toRows(children, row.depth + 1));

    this.expandedPaths.add(row.entry.Path);
    this.rows.set(updatedRows);
  }

  private collapse(index: number, rows: TreeRow[]): void {
    const row = rows[index];
    let endIndex = index + 1;
    while (endIndex < rows.length && rows[endIndex].depth > row.depth) {
      this.expandedPaths.delete(rows[endIndex].entry.Path);
      endIndex += 1;
    }

    this.expandedPaths.delete(row.entry.Path);
    const updatedRows = [...rows];
    updatedRows[index] = { ...row, expanded: false };
    updatedRows.splice(index + 1, endIndex - index - 1);
    this.rows.set(updatedRows);
  }

  /** Expands a directory and each of its ancestors. */
  private async revealDirectory(directoryPath: string): Promise<void> {
    const segments = directoryPath.split('/');
    let prefix = segments[0];

    for (let index = 1; index < segments.length; index += 1) {
      prefix = `${prefix}/${segments[index]}`;
      const row = this.rows().find((candidate) => candidate.entry.Path === prefix);
      if (row && !row.expanded) {
        await this.toggle(row);
      }
    }
  }

  /** Rows start collapsed; expansion inserts their children explicitly. */
  private toRows(entries: Entry[], depth: number): TreeRow[] {
    return entries.map((entry) => ({ entry, depth, expanded: false }));
  }
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
