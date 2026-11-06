import { Injectable, signal } from '@angular/core';

const STORAGE_KEY = 'awake.environments';

/**
 * Holds the environment selected for the open workspace and remembers the
 * choice per workspace, so reopening the application restores it. The
 * memory stays on this computer: it is a personal preference, not a setting
 * shared through git.
 */
@Injectable({ providedIn: 'root' })
export class EnvironmentStore {
  private readonly current = signal('');
  private workspaceRoot = '';

  readonly selected = this.current.asReadonly();

  /** Environment last chosen in a workspace, or an empty string. */
  remembered(workspaceRoot: string): string {
    return readMemory()[workspaceRoot] ?? '';
  }

  /**
   * Scopes the following selections to a workspace. Opening another
   * workspace clears the selection of the previous one.
   */
  useWorkspace(workspaceRoot: string): void {
    if (workspaceRoot !== this.workspaceRoot) {
      this.workspaceRoot = workspaceRoot;
      this.current.set('');
    }
  }

  select(fileName: string): void {
    this.current.set(fileName);
    if (!this.workspaceRoot) {
      return;
    }
    const memory = readMemory();
    if (fileName) {
      memory[this.workspaceRoot] = fileName;
    } else {
      delete memory[this.workspaceRoot];
    }
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(memory));
    } catch {
      // Storage may be unavailable; the selection still applies to this session.
    }
  }
}

function readMemory(): Record<string, string> {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}');
    return parsed && typeof parsed === 'object' ? (parsed as Record<string, string>) : {};
  } catch {
    return {};
  }
}
