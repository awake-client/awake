import { Injectable, computed, signal } from '@angular/core';
import { Configuration } from '@bindings/github.com/awake-client/awake/src/features/workspace/models';

const STORAGE_KEY = 'awake.workspace';
const RECENT_KEY = 'awake.recentWorkspaces';
const MAX_RECENT = 12;

/** A workspace opened before, offered by the workspace switcher. */
export interface RecentWorkspace {
  rootPath: string;
  name: string;
  color: string;
}

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
  /** Workspaces opened on this computer, most recent first. */
  readonly recent = signal<RecentWorkspace[]>(readRecent());
  readonly rootPath = computed(() => this.current()?.rootPath ?? null);
  readonly isOpen = computed(() => this.current() !== null);

  open(rootPath: string, configuration: Configuration): void {
    const workspace: OpenedWorkspace = { rootPath, configuration };
    this.current.set(workspace);
    this.remember({ rootPath, name: configuration.Name, color: configuration.Color ?? '' });

    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(workspace));
    } catch {
      // Storage may be unavailable; the workspace stays open for this session.
    }
  }

  constructor() {
    const current = this.current();
    if (current) {
      this.remember({
        rootPath: current.rootPath,
        name: current.configuration.Name,
        color: current.configuration.Color ?? '',
      });
    }
  }

  /** Removes a workspace from the recent list, for instance once deleted. */
  forget(rootPath: string): void {
    this.saveRecent(this.recent().filter((workspace) => workspace.rootPath !== rootPath));
  }

  private remember(workspace: RecentWorkspace): void {
    this.saveRecent(
      [workspace, ...this.recent().filter((other) => other.rootPath !== workspace.rootPath)].slice(
        0,
        MAX_RECENT,
      ),
    );
  }

  private saveRecent(recent: RecentWorkspace[]): void {
    this.recent.set(recent);
    try {
      localStorage.setItem(RECENT_KEY, JSON.stringify(recent));
    } catch {
      // The list still applies to this session.
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

function readRecent(): RecentWorkspace[] {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(RECENT_KEY) ?? '[]');
    return Array.isArray(parsed)
      ? parsed.filter(
          (item): item is RecentWorkspace =>
            typeof item?.rootPath === 'string' && typeof item?.name === 'string',
        )
      : [];
  } catch {
    return [];
  }
}
