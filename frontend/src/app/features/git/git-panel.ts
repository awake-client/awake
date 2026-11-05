import { Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import * as GitService from '@bindings/github.com/clementherve/awake/src/features/git/service';
import {
  Change,
  ConflictVersions,
  Status,
} from '@bindings/github.com/clementherve/awake/src/features/git/models';
import { WorkspaceStore } from '@core/workspace-store';

/** Shows the git state of the workspace and lets the user synchronize it. */
@Component({
  selector: 'app-git-panel',
  imports: [FormsModule],
  templateUrl: './git-panel.html',
  styleUrl: './git-panel.css',
})
export class GitPanel {
  private readonly workspaceStore = inject(WorkspaceStore);

  protected readonly isRepository = signal<boolean | null>(null);
  protected readonly status = signal<Status | null>(null);
  protected readonly isLoading = signal(true);
  protected readonly isBusy = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly statusMessage = signal<string | null>(null);
  protected readonly commitMessage = signal('');

  protected readonly conflictPath = signal<string | null>(null);
  protected readonly conflictVersions = signal<ConflictVersions | null>(null);
  protected readonly isLoadingConflict = signal(false);

  constructor() {
    void this.load();
  }

  protected get changes(): Change[] {
    return this.status()?.Changes ?? [];
  }

  protected get conflicts(): Change[] {
    return this.changes.filter((change) => change.Conflicted);
  }

  protected async load(): Promise<void> {
    const workspaceRoot = this.workspaceStore.rootPath();
    if (!workspaceRoot) {
      return;
    }

    this.isLoading.set(true);
    this.errorMessage.set(null);

    try {
      const isRepository = await GitService.IsRepository(workspaceRoot);
      this.isRepository.set(isRepository);

      if (isRepository) {
        this.status.set(await GitService.Status(workspaceRoot));
      }
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  protected async initialize(): Promise<void> {
    await this.run(async (workspaceRoot) => {
      await GitService.Initialize(workspaceRoot, 'main');
      return 'Repository initialized on branch main.';
    });
    await this.load();
  }

  protected async commit(): Promise<void> {
    const message = this.commitMessage().trim();
    if (message === '') {
      this.errorMessage.set('Write a commit message first.');
      return;
    }

    const hash = await this.run(async (workspaceRoot) => {
      const hash = await GitService.Commit(workspaceRoot, message);
      return `Committed ${hash.slice(0, 8)}.`;
    });

    if (hash) {
      this.commitMessage.set('');
    }
    await this.load();
  }

  protected async pull(): Promise<void> {
    const result = await this.run(async (workspaceRoot) => {
      return GitService.Pull(workspaceRoot);
    });

    if (!result) {
      return;
    }

    if ((result.Conflicts ?? []).length > 0) {
      this.statusMessage.set(
        `Pull produced ${result.Conflicts?.length ?? 0} conflict(s). Resolve them below.`,
      );
    } else {
      this.statusMessage.set(
        result.AlreadyUpToDate ? 'Already up to date.' : 'Pulled changes.',
      );
    }

    await this.load();
  }

  protected async push(): Promise<void> {
    await this.run(async (workspaceRoot) => {
      await GitService.Push(workspaceRoot);
      return 'Pushed changes.';
    });
    await this.load();
  }

  protected async openConflict(path: string): Promise<void> {
    const workspaceRoot = this.workspaceStore.rootPath();
    if (!workspaceRoot) {
      return;
    }

    this.conflictPath.set(path);
    this.conflictVersions.set(null);
    this.isLoadingConflict.set(true);
    this.errorMessage.set(null);

    try {
      this.conflictVersions.set(
        await GitService.ConflictVersions(workspaceRoot, path),
      );
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoadingConflict.set(false);
    }
  }

  protected closeConflict(): void {
    this.conflictPath.set(null);
    this.conflictVersions.set(null);
  }

  protected async resolve(side: 'local' | 'incoming'): Promise<void> {
    const workspaceRoot = this.workspaceStore.rootPath();
    const path = this.conflictPath();
    const versions = this.conflictVersions();
    if (!workspaceRoot || !path || !versions) {
      return;
    }

    const encoded = side === 'local' ? versions.Local : versions.Incoming;
    await this.run(async () => {
      await GitService.ResolveWith(workspaceRoot, path, decodeContent(encoded));
      return `Kept the ${side} version of ${path}.`;
    });

    this.closeConflict();
    await this.load();
  }

  protected changeLabel(change: Change): string {
    if (change.Conflicted) {
      return 'U';
    }
    if (change.Untracked) {
      return 'U';
    }

    const label = `${change.IndexStatus}${change.WorktreeStatus}`.trim();
    return label || '?';
  }

  protected decode(value: string | null): string {
    return decodeContent(value);
  }

  private async run<T>(operation: (workspaceRoot: string) => Promise<T>): Promise<T | null> {
    const workspaceRoot = this.workspaceStore.rootPath();
    if (!workspaceRoot || this.isBusy()) {
      return null;
    }

    this.isBusy.set(true);
    this.errorMessage.set(null);
    this.statusMessage.set(null);

    try {
      const result = await operation(workspaceRoot);
      if (typeof result === 'string') {
        this.statusMessage.set(result);
      }
      return result;
    } catch (error) {
      this.errorMessage.set(describeError(error));
      return null;
    } finally {
      this.isBusy.set(false);
    }
  }
}

function decodeContent(value: string | null): string {
  if (!value) {
    return '';
  }

  try {
    const binary = atob(value);
    const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
    return new TextDecoder().decode(bytes);
  } catch {
    // Wails may return the raw text when the content is not base64.
    return value;
  }
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
