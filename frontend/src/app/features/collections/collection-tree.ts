import { ScrollingModule } from '@angular/cdk/scrolling';
import {
  Component,
  computed,
  effect,
  input,
  output,
  signal,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import * as CollectionsService from '@bindings/github.com/clementherve/awake/src/features/collections/service';
import {
  Entry,
  Kind,
} from '@bindings/github.com/clementherve/awake/src/features/collections/models';
import { Protocol } from '@bindings/github.com/clementherve/awake/src/domain/models';

const ROW_HEIGHT = 28;
const COLLECTIONS_DIRECTORY = 'collections';
const REQUEST_FILE_EXTENSION = '.yaml';
const DEFAULT_REQUEST_URL = 'https://api.example.com/resource';

/** Kind of entry the tree can create. */
type CreationKind = 'collection' | 'request';

/** One visible line of the flattened collection tree. */
interface TreeRow {
  entry: Entry;
  depth: number;
  expanded: boolean;
}

/**
 * Lists the collection tree, lazily loads directories as they expand and
 * creates collections or requests. Only expanded rows are kept in memory and
 * rendered through a virtual viewport, so a workspace with thousands of
 * requests stays responsive.
 */
@Component({
  selector: 'app-collection-tree',
  imports: [ScrollingModule, FormsModule],
  templateUrl: './collection-tree.html',
  styleUrl: './collection-tree.css',
})
export class CollectionTree {
  private readonly childrenCache = new Map<string, Entry[]>();
  private readonly expandedPaths = new Set<string>();

  readonly workspaceRoot = input('');
  readonly requestSelected = output<string>();

  protected readonly rowHeight = ROW_HEIGHT;
  protected readonly rows = signal<TreeRow[]>([]);
  protected readonly isLoading = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly selectedPath = signal<string | null>(null);
  protected readonly selectedFolder = signal<string | null>(null);

  protected readonly isCreating = signal(false);
  protected readonly creationKind = signal<CreationKind>('collection');
  protected readonly creationName = signal('');
  protected readonly creationUrl = signal(DEFAULT_REQUEST_URL);
  protected readonly creationError = signal<string | null>(null);
  protected readonly isSavingEntry = signal(false);
  protected readonly canCreateRequest = computed(
    () => this.selectedFolder() !== null,
  );

  constructor() {
    effect(() => {
      const workspaceRoot = this.workspaceRoot();
      if (workspaceRoot) {
        void this.reload(workspaceRoot);
      }
    });
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

  protected async reload(workspaceRoot: string): Promise<void> {
    this.isLoading.set(true);
    this.errorMessage.set(null);

    try {
      const entries = await CollectionsService.List(workspaceRoot, '');

      this.childrenCache.clear();
      this.expandedPaths.clear();
      this.childrenCache.set('', entries ?? []);
      this.rows.set(this.toRows(entries ?? [], 0));
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

  protected openCreation(): void {
    this.creationKind.set(
      this.selectedFolder() === null ? 'collection' : 'request',
    );
    this.creationName.set('');
    this.creationUrl.set(DEFAULT_REQUEST_URL);
    this.creationError.set(null);
    this.isCreating.set(true);
  }

  protected closeCreation(): void {
    this.isCreating.set(false);
    this.creationError.set(null);
  }

  protected selectCreationKind(event: Event): void {
    this.creationKind.set(
      (event.target as HTMLSelectElement).value as CreationKind,
    );
  }

  protected async createEntry(event: Event): Promise<void> {
    event.preventDefault();

    const workspaceRoot = this.workspaceRoot();
    const name = this.creationName().trim();
    const kind = this.creationKind();

    if (!workspaceRoot) {
      return;
    }
    if (name === '') {
      this.creationError.set('Give the entry a name.');
      return;
    }

    const slug = slugify(name);
    if (slug === '') {
      this.creationError.set('The name needs at least one letter or digit.');
      return;
    }

    if (kind === 'request' && this.selectedFolder() === null) {
      this.creationError.set('Select a collection or folder first.');
      return;
    }

    this.isSavingEntry.set(true);
    this.creationError.set(null);

    try {
      if (kind === 'collection') {
        const collectionPath = `${COLLECTIONS_DIRECTORY}/${slug}`;
        await CollectionsService.CreateCollection(workspaceRoot, collectionPath, {
          Name: name,
          Variables: null,
          Authentication: null,
          Documentation: '',
        });
        await this.reload(workspaceRoot);
      } else {
        const directory = this.selectedFolder() as string;
        const requestPath = `${directory}/${slug}${REQUEST_FILE_EXTENSION}`;
        await CollectionsService.CreateRequest(workspaceRoot, requestPath, {
          Name: name,
          Method: 'GET',
          URL: this.creationUrl().trim() || DEFAULT_REQUEST_URL,
          Protocol: Protocol.ProtocolHTTP,
          Parameters: null,
          Headers: null,
          Authentication: null,
          Body: null,
          Documentation: '',
        });

        await this.reload(workspaceRoot);
        await this.revealDirectory(directory);
        this.selectedPath.set(requestPath);
        this.requestSelected.emit(requestPath);
      }

      this.isCreating.set(false);
    } catch (error) {
      this.creationError.set(describeError(error));
    } finally {
      this.isSavingEntry.set(false);
    }
  }

  protected trackByPath(_index: number, row: TreeRow): string {
    return row.entry.Path;
  }

  private async toggle(row: TreeRow): Promise<void> {
    const rows = this.rows();
    const index = rows.findIndex(
      (candidate) => candidate.entry.Path === row.entry.Path,
    );
    if (index < 0) {
      return;
    }

    if (row.expanded) {
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
        children =
          (await CollectionsService.List(workspaceRoot, row.entry.Path)) ?? [];
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
    if (currentIndex < 0) {
      return;
    }

    const updatedRows = [...currentRows];
    updatedRows[currentIndex] = { ...row, expanded: true };
    updatedRows.splice(
      currentIndex + 1,
      0,
      ...this.toRows(children, row.depth + 1),
    );

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

    const updatedRows = [...rows];
    updatedRows[index] = { ...row, expanded: false };
    updatedRows.splice(index + 1, endIndex - index - 1);
    this.rows.set(updatedRows);
  }

  /** Expands each ancestor of a directory so a freshly created request shows. */
  private async revealDirectory(directoryPath: string): Promise<void> {
    const segments = directoryPath.split('/');
    let prefix = segments[0];

    for (let index = 1; index < segments.length; index += 1) {
      prefix = `${prefix}/${segments[index]}`;
      const row = this.rows().find(
        (candidate) => candidate.entry.Path === prefix,
      );
      if (row && !row.expanded) {
        await this.toggle(row);
      }
    }
  }

  private toRows(entries: Entry[], depth: number): TreeRow[] {
    return entries.map((entry) => ({
      entry,
      depth,
      expanded: this.expandedPaths.has(entry.Path),
    }));
  }
}

function slugify(value: string): string {
  return value
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
