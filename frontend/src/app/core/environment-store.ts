import { Injectable, signal } from '@angular/core';

const STORAGE_KEY = 'awake.environments';
const PINS_KEY = 'awake.environment-pins';

/**
 * Holds the environment selected for the open workspace and remembers the
 * choice per workspace and per collection, so switching tabs between
 * collections restores the environment chosen in each of them, and reopening
 * the application restores the last one. It also remembers the environments
 * pinned in the picker. The memory stays on this computer: it is a personal
 * preference, not a setting shared through git.
 */
@Injectable({ providedIn: 'root' })
export class EnvironmentStore {
  private readonly current = signal('');
  private readonly pinnedFiles = signal<readonly string[]>([]);
  private workspaceRoot = '';
  /** Collection whose slot is active; empty for the global environments. */
  private scope = '';

  readonly selected = this.current.asReadonly();
  /** File names pinned in the picker of the open workspace. */
  readonly pinned = this.pinnedFiles.asReadonly();

  /**
   * Scopes the following selections to a workspace. Opening another
   * workspace clears the selection of the previous one.
   */
  useWorkspace(workspaceRoot: string): void {
    if (workspaceRoot !== this.workspaceRoot) {
      this.workspaceRoot = workspaceRoot;
      this.scope = '';
      this.current.set(readMemory()[workspaceRoot]?.[''] ?? '');
      this.pinnedFiles.set(readPins()[workspaceRoot] ?? []);
    }
  }

  /**
   * Activates the slot of a collection or of one of its folders: its
   * remembered environment becomes the current one. A scope never chosen in
   * starts from the choice of its collection, then of the workspace, so the
   * environment selected before opening it carries over.
   */
  useScope(scope: string): void {
    if (scope === this.scope) {
      return;
    }
    this.scope = scope;
    this.current.set(
      this.slot(this.workspaceRoot, scope) ??
        this.slot(this.workspaceRoot, parentScopeOf(scope)) ??
        this.slot(this.workspaceRoot, '') ??
        '',
    );
  }

  select(fileName: string): void {
    this.current.set(fileName);
    if (!this.workspaceRoot) {
      return;
    }
    writeMemory(this.workspaceRoot, this.scope, fileName);
  }

  /** Pins or unpins an environment of the open workspace. */
  togglePin(fileName: string): void {
    if (!fileName || !this.workspaceRoot) {
      return;
    }
    const pinned = this.pinnedFiles();
    const next = pinned.includes(fileName)
      ? pinned.filter((pinnedFile) => pinnedFile !== fileName)
      : [...pinned, fileName];
    this.pinnedFiles.set(next);
    writePins(this.workspaceRoot, next);
  }

  /** Stored choice of a slot, undefined when the user never chose there. */
  private slot(workspaceRoot: string, scope: string): string | undefined {
    return readMemory()[workspaceRoot]?.[scope];
  }
}

/** Selection per workspace, then per collection scope. */
type Memory = Record<string, Record<string, string>>;

/**
 * Collection a folder scope inherits its choice from. A collection scope has
 * no parent of its own and falls back to the workspace slot.
 */
function parentScopeOf(scope: string): string {
  const segments = scope.split('/');
  return segments.length > 2 ? segments.slice(0, 2).join('/') : '';
}

function writeMemory(workspaceRoot: string, scope: string, fileName: string): void {
  const memory = readMemory();
  memory[workspaceRoot] = { ...(memory[workspaceRoot] ?? {}), [scope]: fileName };

  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(memory));
  } catch {
    // Storage may be unavailable; the selection still applies to this session.
  }
}

function readMemory(): Memory {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}');
    if (!parsed || typeof parsed !== 'object') {
      return {};
    }
    return normalize(parsed as Record<string, unknown>);
  } catch {
    return {};
  }
}

/** Pinned file names per workspace. */
type Pins = Record<string, string[]>;

function writePins(workspaceRoot: string, pinned: readonly string[]): void {
  const pins = readPins();
  pins[workspaceRoot] = [...pinned];

  try {
    localStorage.setItem(PINS_KEY, JSON.stringify(pins));
  } catch {
    // Storage may be unavailable; the pins still apply to this session.
  }
}

function readPins(): Pins {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(PINS_KEY) ?? '{}');
    if (!parsed || typeof parsed !== 'object') {
      return {};
    }

    const pins: Pins = {};
    for (const [workspaceRoot, value] of Object.entries(parsed as Record<string, unknown>)) {
      if (!Array.isArray(value)) {
        continue;
      }
      pins[workspaceRoot] = value.filter(
        (fileName): fileName is string => typeof fileName === 'string',
      );
    }
    return pins;
  } catch {
    return {};
  }
}

/** Reads the current shape, and the per-workspace string of older versions. */
function normalize(stored: Record<string, unknown>): Memory {
  const memory: Memory = {};
  for (const [workspaceRoot, value] of Object.entries(stored)) {
    if (typeof value === 'string') {
      memory[workspaceRoot] = { '': value };
      continue;
    }
    if (!value || typeof value !== 'object') {
      continue;
    }

    const scopes: Record<string, string> = {};
    for (const [scope, fileName] of Object.entries(value as Record<string, unknown>)) {
      if (typeof fileName === 'string') {
        scopes[scope] = fileName;
      }
    }
    memory[workspaceRoot] = scopes;
  }
  return memory;
}
