import { Injectable, computed, signal } from '@angular/core';
import { Configuration } from '@bindings/github.com/clementherve/awake/src/features/workspace/models';

const STORAGE_KEY = 'awake.workspace';

/** A workspace opened by the user, with the location it was loaded from. */
export interface OpenedWorkspace {
  rootPath: string;
  configuration: Configuration;
}

/**
 * Holds the workspace currently opened by the user. The backend only knows a
 * workspace by its files, so the frontend remembers the root path and reopens
 * it across restarts.
 */
@Injectable({ providedIn: 'root' })
export class WorkspaceStore {
  private readonly current = signal<OpenedWorkspace | null>(readStoredWorkspace());

  readonly workspace = this.current.asReadonly();
  readonly rootPath = computed(() => this.current()?.rootPath ?? null);
  readonly isOpen = computed(() => this.current() !== null);

  open(rootPath: string, configuration: Configuration): void {
    const workspace: OpenedWorkspace = { rootPath, configuration };
    this.current.set(workspace);

    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(workspace));
    } catch {
      // Storage may be unavailable; the workspace stays open for this session.
    }
  }

  close(): void {
    this.current.set(null);

    try {
      localStorage.removeItem(STORAGE_KEY);
    } catch {
      // Ignore storage failures when closing.
    }
  }
}

function readStoredWorkspace(): OpenedWorkspace | null {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) {
      return null;
    }

    const parsed = JSON.parse(stored) as OpenedWorkspace;
    if (!parsed.rootPath || !parsed.configuration) {
      return null;
    }

    return parsed;
  } catch {
    return null;
  }
}
