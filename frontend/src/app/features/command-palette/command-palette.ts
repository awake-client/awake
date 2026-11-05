import {
  Component,
  ElementRef,
  computed,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import * as CollectionsService from '@bindings/github.com/clementherve/awake/src/features/collections/service';
import { Entry } from '@bindings/github.com/clementherve/awake/src/features/collections/models';
import { WorkspaceStore } from '@core/workspace-store';

const RESULT_LIMIT = 500;
const VISIBLE_RESULT_LIMIT = 50;
const COLLECTIONS_PREFIX = 'collections/';

/**
 * Quick switcher for requests. It opens with Ctrl+P or Cmd+P, searches across
 * every collection of the workspace and navigates to the chosen request.
 */
@Component({
  selector: 'app-command-palette',
  imports: [FormsModule],
  templateUrl: './command-palette.html',
  styleUrl: './command-palette.css',
  host: {
    '(document:keydown)': 'handleKeydown($event)',
    '[class.open]': 'isOpen()',
  },
})
export class CommandPalette {
  private readonly router = inject(Router);
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly searchInput =
    viewChild<ElementRef<HTMLInputElement>>('searchInput');

  protected readonly isOpen = signal(false);
  protected readonly isLoading = signal(false);
  protected readonly query = signal('');
  protected readonly entries = signal<Entry[]>([]);
  protected readonly activeIndex = signal(0);
  protected readonly errorMessage = signal<string | null>(null);

  protected readonly results = computed(() => {
    const terms = this.query()
      .trim()
      .toLowerCase()
      .split(/\s+/)
      .filter((term) => term !== '');

    return this.entries()
      .filter((entry) => {
        if (terms.length === 0) {
          return true;
        }
        const haystack =
          `${entry.Name} ${entry.Path} ${entry.Method}`.toLowerCase();
        return terms.every((term) => haystack.includes(term));
      })
      .slice(0, VISIBLE_RESULT_LIMIT);
  });

  protected readonly selectedIndex = computed(() => {
    const results = this.results();
    if (results.length === 0) {
      return -1;
    }
    return Math.min(this.activeIndex(), results.length - 1);
  });

  constructor() {
    effect(() => {
      if (this.isOpen()) {
        this.searchInput()?.nativeElement.focus();
      }
    });
  }

  /** Opens the palette and loads the requests of the current workspace. */
  async open(): Promise<void> {
    if (this.isOpen()) {
      return;
    }

    this.isOpen.set(true);
    this.query.set('');
    this.activeIndex.set(0);
    this.errorMessage.set(null);

    const workspaceRoot = this.workspaceStore.rootPath();
    if (!workspaceRoot) {
      this.entries.set([]);
      return;
    }

    this.isLoading.set(true);

    try {
      this.entries.set(
        (await CollectionsService.SearchRequests(
          workspaceRoot,
          '',
          RESULT_LIMIT,
        )) ?? [],
      );
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  protected close(): void {
    this.isOpen.set(false);
  }

  protected updateQuery(value: string): void {
    this.query.set(value);
    this.activeIndex.set(0);
  }

  protected async choose(entry: Entry): Promise<void> {
    this.close();
    await this.router.navigate([
      '/workspace/request',
      ...entry.Path.split('/'),
    ]);
  }

  protected chooseActive(): void {
    const entry = this.results()[this.selectedIndex()];
    if (entry) {
      void this.choose(entry);
    }
  }

  protected moveSelection(delta: number): void {
    const results = this.results();
    if (results.length === 0) {
      return;
    }

    const next = (this.selectedIndex() + delta + results.length) % results.length;
    this.activeIndex.set(next);
  }

  protected displayPath(entry: Entry): string {
    return entry.Path.startsWith(COLLECTIONS_PREFIX)
      ? entry.Path.slice(COLLECTIONS_PREFIX.length)
      : entry.Path;
  }

  protected handleKeydown(event: KeyboardEvent): void {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'p') {
      event.preventDefault();
      if (this.isOpen()) {
        this.close();
      } else {
        void this.open();
      }
      return;
    }

    if (!this.isOpen()) {
      return;
    }

    switch (event.key) {
      case 'Escape':
        event.preventDefault();
        this.close();
        break;
      case 'ArrowDown':
        event.preventDefault();
        this.moveSelection(1);
        break;
      case 'ArrowUp':
        event.preventDefault();
        this.moveSelection(-1);
        break;
      case 'Enter':
        event.preventDefault();
        this.chooseActive();
        break;
    }
  }
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
