import { ScrollingModule } from '@angular/cdk/scrolling';
import { Component, DestroyRef, effect, inject, input, output, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router } from '@angular/router';
import { Clipboard } from '@wailsio/runtime';
import { filter } from 'rxjs';
import * as HistoryService from '@bindings/github.com/awake-client/awake/src/features/history/service';
import * as CollectionsService from '@bindings/github.com/awake-client/awake/src/features/collections/service';
import {
  Entry,
  Kind,
} from '@bindings/github.com/awake-client/awake/src/features/collections/models';
import {
  COLLECTIONS_CHANGED_EVENT,
  CollectionsChange,
  CreationKind,
  notifyCollectionsChanged,
  requestEntryCreation,
} from '@core/collection-events';
import { Confirmation } from '@core/confirmation';
import { HiddenCollectionsStore } from '@core/hidden-collections';
import { Prompt } from '@core/prompt';
import { Shortcuts } from '@core/shortcuts';
import { TabsStore } from '@core/tabs-store';
import { Icon } from '@shared/icon';

const ROW_HEIGHT = 28;

/** Short labels keep the method column narrow. */
const METHOD_LABELS: Record<string, string> = {
  DELETE: 'DEL',
  OPTIONS: 'OPT',
};

/** The contextual menu of a row, positioned in viewport coordinates. */
interface RowMenu {
  row: TreeRow;
  top: number;
  left: number;
}

/** Root of the collections that stay on this computer, ignored by git. */
const PRIVATE_ROOT = 'local';
const PRIVATE_SECTION_PATH = '#private';

/** One visible line of the flattened collection tree. */
interface TreeRow {
  entry: Entry;
  depth: number;
  expanded: boolean;
  /** Set on the header row that introduces the private collections. */
  section?: boolean;
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
  private readonly confirmation = inject(Confirmation);
  private readonly hiddenCollections = inject(HiddenCollectionsStore);
  private readonly prompt = inject(Prompt);
  private readonly tabsStore = inject(TabsStore);
  protected readonly shortcuts = inject(Shortcuts);

  readonly workspaceRoot = input('');
  readonly requestSelected = output<string>();

  protected readonly rowHeight = ROW_HEIGHT;
  protected readonly rows = signal<TreeRow[]>([]);
  protected readonly isLoading = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  /**
   * The single highlighted row: the open request, or the folder last clicked.
   * One selection at a time keeps the tree from showing two active rows.
   */
  protected readonly selected = signal<string | null>(null);
  protected readonly isMenuOpen = signal(false);
  protected readonly rowMenu = signal<RowMenu | null>(null);
  /** Number of collections the user hid, shown dimmed on demand. */
  protected readonly hiddenCount = signal(0);
  protected readonly showHidden = this.hiddenCollections.showHidden;
  /** Folder that would receive the row being dragged. */
  protected readonly dropTarget = signal<string | null>(null);
  protected readonly dragged = signal<TreeRow | null>(null);
  private expandTimer: number | undefined;

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
      this.selected.set(path);
      void this.revealDirectory(path);
    };
    const closeMenu = () => {
      this.isMenuOpen.set(false);
      this.rowMenu.set(null);
    };

    // Keep the highlighted request in sync with the page on screen, whether it
    // was opened from the tree, the search palette or the history.
    this.router.events
      .pipe(
        filter((event) => event instanceof NavigationEnd),
        takeUntilDestroyed(),
      )
      .subscribe(() => void this.syncSelectionWithRoute());

    // Agents and editors change the files from other processes, such as
    // the MCP server: reload when the window comes back to the front.
    const reloadOnFocus = () => this.refresh();

    window.addEventListener(COLLECTIONS_CHANGED_EVENT, collectionsChanged);
    window.addEventListener('awake:open-collection', openCollection);
    window.addEventListener('focus', reloadOnFocus);
    document.addEventListener('click', closeMenu);
    this.destroyRef.onDestroy(() => {
      window.removeEventListener('focus', reloadOnFocus);
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

  protected isPrivate(row: TreeRow): boolean {
    return row.entry.Path.startsWith(`${PRIVATE_ROOT}/`);
  }

  protected isRequest(row: TreeRow): boolean {
    return row.entry.Kind === Kind.KindRequest;
  }

  /** Reports whether a root entry is a collection the user hid. */
  protected isHidden(entry: Entry): boolean {
    return entry.Kind === Kind.KindCollection && this.hiddenCollections.isHidden(entry.Path);
  }

  protected async activate(row: TreeRow, event?: MouseEvent): Promise<void> {
    if (row.section) {
      return;
    }
    // The second click of a double click on a collection or folder is left
    // to the double click, which opens the settings, so the row does not
    // fold back.
    if (event && event.detail > 1 && !this.isRequest(row)) {
      return;
    }
    if (this.isRequest(row)) {
      this.selected.set(row.entry.Path);
      this.requestSelected.emit(row.entry.Path);
      return;
    }

    // A collection opens its settings and unfolds; clicked again while its
    // settings are on screen, it folds. A folder just toggles.
    if (row.depth === 0) {
      const settingsShown = this.openSettingsPath() === row.entry.Path;
      this.selected.set(row.entry.Path);
      if (row.expanded && settingsShown) {
        await this.toggle(row);
        return;
      }
      void this.router.navigate(['/workspace/collection', ...row.entry.Path.split('/')]);
      if (!row.expanded) {
        await this.toggle(row);
      }
      return;
    }
    this.selected.set(row.entry.Path);
    await this.toggle(row);
  }

  /** The chevron only folds or unfolds, without opening anything. */
  protected async toggleRow(event: Event, row: TreeRow): Promise<void> {
    event.stopPropagation();
    await this.toggle(row);
  }

  /**
   * Reloads the tree from disk and expands again the folders that were open,
   * so a refresh never collapses what the user was looking at.
   */
  protected async reload(workspaceRoot: string): Promise<void> {
    this.isLoading.set(true);
    this.errorMessage.set(null);
    this.hiddenCollections.useWorkspace(workspaceRoot);

    const previouslyExpanded = [...this.expandedPaths].sort(
      (first, second) => first.split('/').length - second.split('/').length,
    );

    try {
      const [entries, privateEntries] = await Promise.all([
        CollectionsService.List(workspaceRoot, ''),
        CollectionsService.List(workspaceRoot, PRIVATE_ROOT),
      ]);

      this.childrenCache.clear();
      this.expandedPaths.clear();
      const rows: TreeRow[] = [];
      const showHidden = this.showHidden();
      let hiddenCount = 0;
      for (const entry of entries ?? []) {
        if (this.isHidden(entry)) {
          hiddenCount++;
          if (!showHidden) {
            continue;
          }
        }
        rows.push(...this.toRows([entry], 0));
      }

      const visiblePrivateEntries: Entry[] = [];
      for (const entry of privateEntries ?? []) {
        if (this.isHidden(entry)) {
          hiddenCount++;
          if (!showHidden) {
            continue;
          }
        }
        visiblePrivateEntries.push(entry);
      }
      if (visiblePrivateEntries.length > 0) {
        rows.push(privateSectionRow(), ...this.toRows(visiblePrivateEntries, 0));
      }
      this.rows.set(rows);
      this.hiddenCount.set(hiddenCount);

      for (const path of previouslyExpanded) {
        const row = this.rows().find((candidate) => candidate.entry.Path === path);
        if (row && !row.expanded) {
          await this.toggle(row);
        }
      }

      // The page may already show a request, opened before the tree finished
      // loading, such as the one restored when the workspace opens.
      await this.syncSelectionWithRoute();
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

  /** Shows the hidden collections dimmed in the tree, or hides them again. */
  protected toggleShowHidden(): void {
    this.hiddenCollections.toggleShowHidden();
    if (!this.showHidden()) {
      const openRequest = this.openRequestPath();
      if (openRequest && this.hiddenCollections.isInHidden(openRequest)) {
        void this.router.navigate(['/workspace']);
      }
    }
    this.refresh();
  }

  /** Reports whether a row belongs to a hidden collection shown dimmed. */
  protected isDimmed(row: TreeRow): boolean {
    return !row.section && this.hiddenCollections.isInHidden(row.entry.Path);
  }

  /** Makes a hidden collection a regular one again. */
  protected async unhideCollection(row: TreeRow): Promise<void> {
    this.hiddenCollections.unhide(row.entry.Path);
    const workspaceRoot = this.workspaceRoot();
    if (workspaceRoot) {
      await this.reload(workspaceRoot);
    }
  }

  /** Hides a collection from the tree, keeping its files on disk. */
  protected async hideCollection(row: TreeRow): Promise<void> {
    const workspaceRoot = this.workspaceRoot();
    if (!workspaceRoot || row.section) {
      return;
    }

    this.hiddenCollections.hide(row.entry.Path);
    if (this.showHidden()) {
      // The collection stays on screen, dimmed.
      await this.reload(workspaceRoot);
      return;
    }
    const contains = (path: string | null) =>
      !!path && (path === row.entry.Path || path.startsWith(`${row.entry.Path}/`));
    if (contains(this.openRequestPath())) {
      await this.router.navigate(['/workspace']);
    }
    if (contains(this.selected())) {
      this.selected.set(null);
    }
    await this.reload(workspaceRoot);
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
    this.selected.set(row.entry.Path);
    requestEntryCreation({ kind, folder: row.entry.Path });
  }

  /** Opens the row menu under the ⋯ button or at the pointer on right click. */
  protected openRowMenu(event: MouseEvent, row: TreeRow): void {
    if (row.section) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    this.isMenuOpen.set(false);

    const target = event.currentTarget as HTMLElement | null;
    if (event.type === 'click' && target) {
      const rect = target.getBoundingClientRect();
      this.rowMenu.set({ row, top: rect.bottom + 4, left: rect.right - 190 });
    } else {
      this.rowMenu.set({ row, top: event.clientY, left: event.clientX });
    }
  }

  protected createFromRowMenu(kind: CreationKind): void {
    const menu = this.rowMenu();
    this.rowMenu.set(null);
    if (menu) {
      this.selected.set(menu.row.entry.Path);
      requestEntryCreation({ kind, folder: menu.row.entry.Path });
    }
  }

  /** A collection environment, scoped to the collection of the row. */
  protected createEnvironmentFromRowMenu(): void {
    const menu = this.rowMenu();
    this.rowMenu.set(null);
    if (menu) {
      requestEntryCreation({ kind: 'environment', scope: menu.row.entry.Path });
    }
  }

  /** Opens the settings of the collection or folder of the row. */
  protected openSettings(): void {
    const menu = this.rowMenu();
    this.rowMenu.set(null);
    if (menu) {
      void this.router.navigate(['/workspace/collection', ...menu.row.entry.Path.split('/')]);
    }
  }

  protected renameFromRowMenu(): void {
    const menu = this.rowMenu();
    this.rowMenu.set(null);
    if (menu) {
      void this.renameCollection(menu.row);
    }
  }

  /** Hides a collection from the tree, keeping its files on disk. */
  protected hideFromRowMenu(): void {
    const menu = this.rowMenu();
    this.rowMenu.set(null);
    if (menu) {
      void this.hideCollection(menu.row);
    }
  }

  protected unhideFromRowMenu(): void {
    const menu = this.rowMenu();
    this.rowMenu.set(null);
    if (menu) {
      void this.unhideCollection(menu.row);
    }
  }

  /**
   * Renames a collection: its display name, shown by the tree and its
   * settings. Its directory keeps its path, so requests, environments and
   * their stored secrets stay where they are.
   */
  protected async renameCollection(row: TreeRow): Promise<void> {
    const workspaceRoot = this.workspaceRoot();
    if (!workspaceRoot || row.section || row.depth !== 0) {
      return;
    }

    const name = await this.prompt.ask({
      title: `Rename the collection “${row.entry.Name}”`,
      message: 'The name shown in the tree and the collection settings.',
      label: 'Collection name',
      icon: 'box',
      value: row.entry.Name,
      confirmLabel: 'Rename',
    });
    if (!name || name === row.entry.Name) {
      return;
    }

    try {
      const collection = await CollectionsService.LoadCollection(workspaceRoot, row.entry.Path);
      await CollectionsService.UpdateCollection(workspaceRoot, row.entry.Path, {
        ...collection,
        Name: name,
      });
      this.selected.set(row.entry.Path);
      notifyCollectionsChanged();
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  protected duplicateFromRowMenu(): void {
    const menu = this.rowMenu();
    this.rowMenu.set(null);
    if (menu) {
      void this.duplicateRequest(menu.row);
    }
  }

  /** Copies the workspace-relative path of a request, for awake.runRequest. */
  protected copyPathFromRowMenu(): void {
    const menu = this.rowMenu();
    this.rowMenu.set(null);
    if (!menu) {
      return;
    }

    void this.copyPath(menu.row.entry.Path);
  }

  private async copyPath(path: string): Promise<void> {
    try {
      await Clipboard.SetText(path);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  /**
   * Copies a request next to the original as "<name> copy", picking a free
   * file name, then opens the copy.
   */
  protected async duplicateRequest(row: TreeRow): Promise<void> {
    const workspaceRoot = this.workspaceRoot();
    if (!workspaceRoot || !this.isRequest(row)) {
      return;
    }

    const path = row.entry.Path;
    const directory = path.slice(0, path.lastIndexOf('/'));
    const baseName = path.slice(path.lastIndexOf('/') + 1).replace(/\.yaml$/, '');

    try {
      const request = await CollectionsService.LoadRequest(workspaceRoot, path);
      const siblings = (await CollectionsService.List(workspaceRoot, directory)) ?? [];
      const taken = new Set(siblings.map((entry) => entry.Path));
      let copyPath = `${directory}/${baseName}-copy.yaml`;
      for (let index = 2; taken.has(copyPath); index += 1) {
        copyPath = `${directory}/${baseName}-copy-${index}.yaml`;
      }

      await CollectionsService.CreateRequest(workspaceRoot, copyPath, {
        ...request,
        Name: `${request.Name} copy`,
      });
      notifyCollectionsChanged({ created: copyPath, kind: 'request' });
      this.requestSelected.emit(copyPath);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  protected deleteFromRowMenu(): void {
    const menu = this.rowMenu();
    this.rowMenu.set(null);
    if (menu) {
      void this.deleteEntry(menu.row);
    }
  }

  /** Asks for confirmation, deletes the entry and leaves its page if open. */
  protected async deleteEntry(row: TreeRow): Promise<void> {
    const workspaceRoot = this.workspaceRoot();
    if (!workspaceRoot || row.section) {
      return;
    }

    const kind = this.isRequest(row) ? 'request' : row.depth === 0 ? 'collection' : 'folder';
    const confirmed = await this.confirmation.ask({
      title: `Delete the ${kind} “${row.entry.Name}”?`,
      message:
        kind === 'request'
          ? `${row.entry.Path} will be removed from disk.`
          : `${row.entry.Path} and everything inside it will be removed from disk.${
              this.isPrivate(row) ? '' : ' Committed files can still be restored with git.'
            }`,
      confirmLabel: `Delete ${kind}`,
      danger: true,
    });
    if (!confirmed) {
      return;
    }

    try {
      await CollectionsService.Delete(workspaceRoot, row.entry.Path);
    } catch (error) {
      this.errorMessage.set(describeError(error));
      return;
    }

    const removed = row.entry.Path;
    const contains = (path: string | null) =>
      !!path && (path === removed || path.startsWith(`${removed}/`));
    if (contains(this.openRequestPath())) {
      await this.router.navigate(['/workspace']);
    }
    if (contains(this.selected())) {
      this.selected.set(null);
    }
    notifyCollectionsChanged({ removed });
  }

  /** Requests and folders can be dragged; collections stay at the root. */
  protected canDrag(row: TreeRow): boolean {
    return !row.section && row.depth > 0;
  }

  protected startDrag(event: DragEvent, row: TreeRow): void {
    if (!this.canDrag(row)) {
      event.preventDefault();
      return;
    }
    this.dragged.set(row);
    this.rowMenu.set(null);
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', row.entry.Path);
    }
  }

  /**
   * Hovering a folder targets it; hovering a request targets its folder. A
   * collapsed folder opens after a short pause, so deep moves stay possible.
   */
  protected dragOver(event: DragEvent, row: TreeRow): void {
    const dragged = this.dragged();
    if (!dragged || row.section) {
      return;
    }
    const target = this.isRequest(row)
      ? row.entry.Path.slice(0, row.entry.Path.lastIndexOf('/'))
      : row.entry.Path;
    const source = dragged.entry.Path;
    const parent = source.slice(0, source.lastIndexOf('/'));
    const intoItself =
      !this.isRequest(dragged) && (target === source || target.startsWith(`${source}/`));
    if (intoItself || target === parent) {
      this.dropTarget.set(null);
      return;
    }

    event.preventDefault();
    if (event.dataTransfer) {
      event.dataTransfer.dropEffect = 'move';
    }
    if (this.dropTarget() !== target) {
      this.dropTarget.set(target);
      window.clearTimeout(this.expandTimer);
      if (!this.isRequest(row) && !row.expanded) {
        this.expandTimer = window.setTimeout(() => {
          const current = this.rows().find((candidate) => candidate.entry.Path === row.entry.Path);
          if (current && !current.expanded && this.dragged()) {
            void this.toggle(current);
          }
        }, 600);
      }
    }
  }

  protected async drop(event: DragEvent): Promise<void> {
    event.preventDefault();
    const dragged = this.dragged();
    const target = this.dropTarget();
    this.endDrag();
    const workspaceRoot = this.workspaceRoot();
    if (!dragged || !target || !workspaceRoot) {
      return;
    }

    try {
      const moved = await CollectionsService.Move(workspaceRoot, dragged.entry.Path, target);
      // The history follows the request; failing to update it must not undo the move.
      await HistoryService.RenameRequests(workspaceRoot, dragged.entry.Path, moved).catch(
        () => undefined,
      );
      this.errorMessage.set(null);
      notifyCollectionsChanged({
        moved: { from: dragged.entry.Path, to: moved },
        created: moved,
        kind: this.isRequest(dragged) ? 'request' : 'folder',
      });
    } catch (error) {
      this.errorMessage.set(describeMoveError(error));
    }
  }

  /** Requests directly inside the targeted folder are tinted with it. */
  protected isInDropTarget(row: TreeRow): boolean {
    const target = this.dropTarget();
    return (
      target !== null &&
      this.isRequest(row) &&
      row.entry.Path.slice(0, row.entry.Path.lastIndexOf('/')) === target
    );
  }

  protected endDrag(): void {
    window.clearTimeout(this.expandTimer);
    this.dragged.set(null);
    this.dropTarget.set(null);
  }

  /** A double click keeps the request open in its own tab. */
  /**
   * A double click pins a request in the tabs, and opens the settings of a
   * collection or folder.
   */
  protected openOnDoubleClick(row: TreeRow): void {
    if (row.section) {
      return;
    }
    if (this.isRequest(row)) {
      this.tabsStore.pin(row.entry.Path);
      return;
    }
    this.selected.set(row.entry.Path);
    void this.router.navigate(['/workspace/collection', ...row.entry.Path.split('/')]);
  }

  protected trackByPath(_index: number, row: TreeRow): string {
    return row.entry.Path;
  }

  /** Path of the request shown by the current page, if any. */
  private openRequestPath(): string | null {
    const prefix = '/workspace/request/';
    const url = this.router.url.split(/[?#]/)[0];
    return url.startsWith(prefix) ? decodeURIComponent(url.slice(prefix.length)) : null;
  }

  /** Path of the collection or folder whose settings are on screen, if any. */
  private openSettingsPath(): string | null {
    const prefix = '/workspace/collection/';
    const url = this.router.url.split(/[?#]/)[0];
    return url.startsWith(prefix) ? decodeURIComponent(url.slice(prefix.length)) : null;
  }

  /**
   * Highlights the page on screen and unfolds the folder of an open request,
   * so a request opened from the search palette, the history or a tab is also
   * visible in the tree.
   */
  private async syncSelectionWithRoute(): Promise<void> {
    const openRequest = this.openRequestPath();
    const openSettings = this.openSettingsPath();
    if (openSettings) {
      this.selected.set(openSettings);
    } else if (openRequest) {
      this.selected.set(openRequest);
      await this.revealDirectory(openRequest.slice(0, openRequest.lastIndexOf('/')));
    } else if (this.selected()?.endsWith('.yaml')) {
      this.selected.set(null);
    }
  }

  /** Folder that receives new entries: the selected folder or the request's. */
  private currentFolder(): string | undefined {
    const selected = this.selected();
    if (!selected) {
      return undefined;
    }
    return selected.endsWith('.yaml') ? selected.slice(0, selected.lastIndexOf('/')) : selected;
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
      this.selected.set(created);
    } else {
      await this.revealDirectory(created.slice(0, created.lastIndexOf('/')));
      this.selected.set(created);
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

function privateSectionRow(): TreeRow {
  return {
    entry: {
      Path: PRIVATE_SECTION_PATH,
      Name: 'Private',
      Kind: Kind.KindCollection,
      Method: '',
      HasChildren: false,
      Color: '',
    },
    depth: 0,
    expanded: false,
    section: true,
  };
}

function describeMoveError(error: unknown): string {
  const message = describeError(error);
  if (message.includes('already exists')) {
    return 'An entry with the same file name already exists there.';
  }
  if (message.includes('into itself')) {
    return 'A folder cannot move into itself.';
  }
  if (message.includes('reserved')) {
    return 'A folder named "environments" cannot sit at the root of a collection.';
  }
  return message;
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
