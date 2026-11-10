import { Injectable, signal } from '@angular/core';

const STORAGE_KEY = 'awake.hiddenCollections';

/**
 * Collections of the open workspace the user chose to hide from the tree.
 * The memory stays on this computer: it is a personal preference, not a
 * setting shared through git.
 */
@Injectable({ providedIn: 'root' })
export class HiddenCollectionsStore {
  private readonly current = signal<string[]>([]);
  private workspaceRoot = '';

  /** Paths of the collections hidden in the open workspace. */
  readonly hiddenPaths = this.current.asReadonly();

  /**
   * Whether the tree shows the hidden collections anyway, dimmed. It lasts
   * for the session only, so they are hidden again at the next start.
   */
  readonly showHidden = signal(false);

  /** Scopes the hidden collections to a workspace. */
  useWorkspace(workspaceRoot: string): void {
    if (workspaceRoot === this.workspaceRoot) {
      return;
    }
    this.workspaceRoot = workspaceRoot;
    this.current.set(readMemory()[workspaceRoot] ?? []);
  }

  /** Reports whether a collection is hidden from the tree. */
  isHidden(path: string): boolean {
    return this.current().includes(path);
  }

  /** Adds a collection to the hidden ones, keeping its files on disk. */
  hide(path: string): void {
    this.current.update((hidden) => (hidden.includes(path) ? hidden : [...hidden, path]));
    this.persist();
  }

  /** Removes a collection from the hidden ones, so it reappears in the tree. */
  unhide(path: string): void {
    this.current.update((hidden) => hidden.filter((entry) => entry !== path));
    this.persist();
  }

  /** Brings every hidden collection back, so the tree shows them again. */
  unhideAll(): void {
    if (this.current().length === 0) {
      return;
    }
    this.current.set([]);
    this.persist();
  }

  /** Reports whether a path is a hidden collection or lies inside one. */
  isInHidden(path: string): boolean {
    return this.current().some((hidden) => path === hidden || path.startsWith(`${hidden}/`));
  }

  /** Shows or hides again the hidden collections in the tree. */
  toggleShowHidden(): void {
    this.showHidden.update((shown) => !shown);
  }

  private persist(): void {
    if (!this.workspaceRoot) {
      return;
    }
    const memory = readMemory();
    memory[this.workspaceRoot] = this.current();
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(memory));
    } catch {
      // Hiding still applies to this session without storage.
    }
  }
}

/** Hidden collections per workspace, keyed by workspace root. */
type Memory = Record<string, string[]>;

function readMemory(): Memory {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}');
    if (!parsed || typeof parsed !== 'object') {
      return {};
    }
    return parsed as Memory;
  } catch {
    return {};
  }
}
