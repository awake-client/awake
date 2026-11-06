import { DestroyRef, Injectable, computed, effect, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router } from '@angular/router';
import { filter } from 'rxjs';
import * as HTTPClientService from '@bindings/github.com/clementherve/awake/src/features/httpclient/service';
import { COLLECTIONS_CHANGED_EVENT, CollectionsChange } from './collection-events';
import { WorkspaceStore } from './workspace-store';

const STORAGE_KEY = 'awake.tabs';
const REQUEST_ROUTE = '/workspace/request/';

/** One open request. */
export interface RequestTab {
  path: string;
  name: string;
  method: string;
  protocol: string;
  /** A preview tab is replaced by the next request opened; editing pins it. */
  preview: boolean;
  dirty: boolean;
}

/**
 * Editor state kept while a tab is in the background, so switching tabs
 * never loses unsaved edits or the last response. The editor decides what
 * goes in it; the store only keeps it and releases the response handle.
 */
export interface StashedEditorState {
  draft: unknown;
  snapshot: string;
  responseIdentifier?: string;
  [key: string]: unknown;
}

interface StoredTabs {
  tabs: Pick<RequestTab, 'path' | 'name' | 'method' | 'protocol'>[];
}

/**
 * Open request tabs. The store follows the router: opening any request,
 * from the tree, the search palette or the history, opens or focuses its
 * tab. Tabs are remembered per workspace.
 */
@Injectable({ providedIn: 'root' })
export class TabsStore {
  private readonly router = inject(Router);
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly stashes = new Map<string, StashedEditorState>();
  private workspaceRoot = '';

  readonly tabs = signal<RequestTab[]>([]);
  /** Path of the request on screen, or null on other pages. */
  readonly active = signal<string | null>(null);
  readonly activeTab = computed(() => this.tabs().find((tab) => tab.path === this.active()));

  constructor() {
    effect(() => this.useWorkspace(this.workspaceStore.rootPath() ?? ''));

    this.router.events
      .pipe(
        filter((event) => event instanceof NavigationEnd),
        takeUntilDestroyed(),
      )
      .subscribe(() => this.followRoute());

    const collectionsChanged = (event: Event) => {
      const change = (event as CustomEvent<CollectionsChange | null>).detail;
      if (change?.removed) {
        this.forgetUnder(change.removed);
      }
    };
    window.addEventListener(COLLECTIONS_CHANGED_EVENT, collectionsChanged);
    inject(DestroyRef).onDestroy(() =>
      window.removeEventListener(COLLECTIONS_CHANGED_EVENT, collectionsChanged),
    );
  }

  /** Records what the editor learned about a request after loading it. */
  describe(path: string, details: Pick<RequestTab, 'name' | 'method' | 'protocol'>): void {
    this.update(path, details);
  }

  markDirty(path: string, dirty: boolean): void {
    const tab = this.tabs().find((candidate) => candidate.path === path);
    if (!tab || tab.dirty === dirty) {
      return;
    }
    this.update(path, { dirty, preview: dirty ? false : tab.preview });
  }

  pin(path: string): void {
    this.update(path, { preview: false });
  }

  stash(path: string, state: StashedEditorState): void {
    if (this.tabs().some((tab) => tab.path === path)) {
      this.stashes.set(path, state);
    } else {
      this.release(state);
    }
  }

  takeStash(path: string): StashedEditorState | undefined {
    const state = this.stashes.get(path);
    this.stashes.delete(path);
    return state;
  }

  hasStash(path: string): boolean {
    return this.stashes.has(path);
  }

  /** Opens a request in a tab and shows it. */
  open(path: string): void {
    void this.router.navigate(['/workspace/request', ...path.split('/')]);
  }

  /**
   * Closes a tab. When it was on screen, its neighbour is shown instead, or
   * the overview when it was the last one.
   */
  close(path: string): void {
    const tabs = this.tabs();
    const index = tabs.findIndex((tab) => tab.path === path);
    if (index < 0) {
      return;
    }

    const remaining = tabs.filter((tab) => tab.path !== path);
    this.tabs.set(remaining);
    const state = this.stashes.get(path);
    this.stashes.delete(path);
    this.release(state);
    this.persist();

    if (this.active() === path) {
      const next = remaining[Math.min(index, remaining.length - 1)];
      if (next) {
        this.open(next.path);
      } else {
        void this.router.navigate(['/workspace']);
      }
    }
  }

  /** Shows the next or previous tab, wrapping around. */
  cycle(step: 1 | -1): void {
    const tabs = this.tabs();
    if (tabs.length === 0) {
      return;
    }
    const index = tabs.findIndex((tab) => tab.path === this.active());
    const next = tabs[(index + step + tabs.length) % tabs.length];
    this.open(next.path);
  }

  move(path: string, targetIndex: number): void {
    const tabs = [...this.tabs()];
    const index = tabs.findIndex((tab) => tab.path === path);
    if (index < 0) {
      return;
    }
    const [tab] = tabs.splice(index, 1);
    tabs.splice(Math.max(0, Math.min(targetIndex, tabs.length)), 0, tab);
    this.tabs.set(tabs);
    this.persist();
  }

  private followRoute(): void {
    const url = this.router.url.split(/[?#]/)[0];
    if (!url.startsWith(REQUEST_ROUTE)) {
      this.active.set(null);
      return;
    }

    const path = decodeURIComponent(url.slice(REQUEST_ROUTE.length));
    this.active.set(path);
    const tabs = this.tabs();
    if (tabs.some((tab) => tab.path === path)) {
      return;
    }

    const tab: RequestTab = {
      path,
      name: fileName(path),
      method: 'GET',
      protocol: 'http',
      preview: true,
      dirty: false,
    };
    const previewIndex = tabs.findIndex((candidate) => candidate.preview && !candidate.dirty);
    if (previewIndex >= 0) {
      const replaced = tabs[previewIndex];
      this.release(this.stashes.get(replaced.path));
      this.stashes.delete(replaced.path);
      this.tabs.set(tabs.map((candidate, index) => (index === previewIndex ? tab : candidate)));
    } else {
      this.tabs.set([...tabs, tab]);
    }
    this.persist();
  }

  private update(path: string, changes: Partial<RequestTab>): void {
    this.tabs.update((tabs) =>
      tabs.map((tab) => (tab.path === path ? { ...tab, ...changes } : tab)),
    );
    this.persist();
  }

  /** Closes the tabs of a deleted request or of the requests of a folder. */
  private forgetUnder(removed: string): void {
    for (const tab of this.tabs()) {
      if (tab.path === removed || tab.path.startsWith(`${removed}/`)) {
        this.close(tab.path);
      }
    }
  }

  private release(state: StashedEditorState | undefined): void {
    if (state?.responseIdentifier) {
      void HTTPClientService.Close(state.responseIdentifier).catch(() => undefined);
    }
  }

  private useWorkspace(workspaceRoot: string): void {
    if (workspaceRoot === this.workspaceRoot) {
      return;
    }
    for (const state of this.stashes.values()) {
      this.release(state);
    }
    this.stashes.clear();
    this.workspaceRoot = workspaceRoot;

    const stored = readStorage()[workspaceRoot];
    this.tabs.set((stored?.tabs ?? []).map((tab) => ({ ...tab, preview: false, dirty: false })));
    this.followRoute();
  }

  private persist(): void {
    if (!this.workspaceRoot) {
      return;
    }
    const storage = readStorage();
    storage[this.workspaceRoot] = {
      tabs: this.tabs()
        .filter((tab) => !tab.preview || tab.path === this.active())
        .map(({ path, name, method, protocol }) => ({ path, name, method, protocol })),
    };
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(storage));
    } catch {
      // Tabs still work for this session without storage.
    }
  }
}

function fileName(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1).replace(/\.yaml$/, '');
}

function readStorage(): Record<string, StoredTabs> {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}');
    return parsed && typeof parsed === 'object' ? (parsed as Record<string, StoredTabs>) : {};
  } catch {
    return {};
  }
}
