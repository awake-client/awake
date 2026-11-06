import { Component, computed, effect, inject, signal } from '@angular/core';
import * as GitService from '@bindings/github.com/clementherve/awake/src/features/git/service';
import {
  Change,
  ConflictVersions,
  Status,
} from '@bindings/github.com/clementherve/awake/src/features/git/models';
import * as AssistantService from '@bindings/github.com/clementherve/awake/src/features/assistant/service';
import * as WorkspaceService from '@bindings/github.com/clementherve/awake/src/features/workspace/service';
import { notifyCollectionsChanged } from '@core/collection-events';
import { GitSync } from '@core/git-sync';
import { WorkspaceStore } from '@core/workspace-store';
import { CodeEditor, CodeLanguage } from '@shared/code-editor';
import { Icon } from '@shared/icon';

/** Intervals offered for automatic synchronization, as Go durations. */
const SYNC_INTERVALS = [
  { value: '1m', label: 'Every minute' },
  { value: '5m', label: 'Every 5 minutes' },
  { value: '15m', label: 'Every 15 minutes' },
  { value: '30m', label: 'Every 30 minutes' },
  { value: '1h', label: 'Every hour' },
];

/** How a changed file is shown: a readable label and a tone. */
interface ChangeKind {
  label: string;
  tone: 'modified' | 'added' | 'deleted' | 'renamed' | 'untracked' | 'conflict';
}

/**
 * Git status of the workspace in plain words: what changed, what is waiting
 * to be pushed or pulled, one button to sync, conflicts resolved side by side
 * and the automatic synchronization settings.
 */
@Component({
  selector: 'app-git-panel',
  imports: [CodeEditor, Icon],
  templateUrl: './git-panel.html',
  styleUrl: './git-panel.css',
})
export class GitPanel {
  private readonly workspaceStore = inject(WorkspaceStore);
  protected readonly gitSync = inject(GitSync);

  protected readonly syncIntervals = SYNC_INTERVALS;
  protected readonly isRepository = signal<boolean | null>(null);
  protected readonly status = signal<Status | null>(null);
  protected readonly isLoading = signal(true);
  protected readonly busy = signal<string | null>(null);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly statusMessage = signal<string | null>(null);
  protected readonly commitMessage = signal('');

  protected readonly conflictPath = signal<string | null>(null);
  protected readonly conflictVersions = signal<ConflictVersions | null>(null);
  protected readonly isLoadingConflict = signal(false);

  /** Claude Code, used to propose the resolution of a conflict. */
  protected readonly claudeInstalled = signal(false);
  protected readonly isSuggesting = signal(false);
  protected readonly suggestion = signal<{ content: string; summary: string } | null>(null);
  protected readonly suggestionError = signal<string | null>(null);

  protected readonly changes = computed(() => this.status()?.Changes ?? []);
  protected readonly conflicts = computed(() =>
    this.changes().filter((change) => change.Conflicted),
  );
  protected readonly pending = computed(() =>
    this.changes().filter((change) => !change.Conflicted),
  );
  protected readonly ahead = computed(() => this.status()?.Ahead ?? 0);
  protected readonly behind = computed(() => this.status()?.Behind ?? 0);
  protected readonly hasUpstream = computed(() => !!this.status()?.Upstream);

  /** One sentence telling where the branch stands. */
  protected readonly summary = computed(() => {
    const status = this.status();
    if (!status) {
      return '';
    }
    if (this.conflicts().length > 0) {
      return `${plural(this.conflicts().length, 'conflict')} to resolve before syncing`;
    }
    if (!status.Upstream) {
      return 'This branch is not published to a remote yet';
    }
    const parts: string[] = [];
    if (status.Ahead > 0) {
      parts.push(`${plural(status.Ahead, 'commit')} to push`);
    }
    if (status.Behind > 0) {
      parts.push(`${plural(status.Behind, 'commit')} to pull`);
    }
    return parts.length > 0 ? parts.join(' · ') : `Up to date with ${status.Upstream}`;
  });

  protected readonly settings = this.gitSync.settings;

  constructor() {
    void this.load();
    void AssistantService.Status()
      .then((status) => this.claudeInstalled.set(status.Installed))
      .catch(() => this.claudeInstalled.set(false));
    // Reload after each automatic synchronization.
    effect(() => {
      if (this.gitSync.lastSync()) {
        void this.load();
      }
    });
  }

  protected changeKind(change: Change): ChangeKind {
    if (change.Conflicted) {
      return { label: 'Conflict', tone: 'conflict' };
    }
    if (change.Untracked) {
      return { label: 'New', tone: 'untracked' };
    }
    const code = `${change.IndexStatus}${change.WorktreeStatus}`;
    if (code.includes('D')) {
      return { label: 'Deleted', tone: 'deleted' };
    }
    if (code.includes('A')) {
      return { label: 'Added', tone: 'added' };
    }
    if (code.includes('R')) {
      return { label: 'Renamed', tone: 'renamed' };
    }
    return { label: 'Modified', tone: 'modified' };
  }

  /** Splits a path into its folder and file name, for a lighter display. */
  protected pathParts(path: string): { folder: string; file: string } {
    const index = path.lastIndexOf('/');
    return index < 0
      ? { folder: '', file: path }
      : { folder: path.slice(0, index + 1), file: path.slice(index + 1) };
  }

  protected inputValue(event: Event): string {
    return (event.target as HTMLInputElement | HTMLTextAreaElement).value;
  }

  protected async load(): Promise<void> {
    const workspaceRoot = this.workspaceStore.rootPath();
    if (!workspaceRoot) {
      return;
    }
    this.isLoading.set(this.status() === null);
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
    await this.run('initialize', async (workspaceRoot) => {
      await GitService.Initialize(workspaceRoot, 'main');
      return 'Repository created on branch main.';
    });
  }

  /** Pulls the team's commits, then pushes yours when there are some. */
  protected async sync(): Promise<void> {
    await this.run('sync', async (workspaceRoot) => {
      const pulled = await GitService.Pull(workspaceRoot);
      if ((pulled.Conflicts ?? []).length > 0) {
        return `The pull produced ${plural(pulled.Conflicts?.length ?? 0, 'conflict')}. Resolve them below.`;
      }
      const status = await GitService.Status(workspaceRoot);
      if (status.Ahead > 0) {
        await GitService.Push(workspaceRoot);
      }
      notifyCollectionsChanged();
      if (!pulled.AlreadyUpToDate && status.Ahead > 0) {
        return 'Pulled the team changes and pushed yours.';
      }
      if (!pulled.AlreadyUpToDate) {
        return 'Pulled the team changes.';
      }
      return status.Ahead > 0 ? 'Pushed your commits.' : 'Already up to date.';
    });
  }

  protected async pull(): Promise<void> {
    await this.run('pull', async (workspaceRoot) => {
      const result = await GitService.Pull(workspaceRoot);
      notifyCollectionsChanged();
      if ((result.Conflicts ?? []).length > 0) {
        return `The pull produced ${plural(result.Conflicts?.length ?? 0, 'conflict')}. Resolve them below.`;
      }
      return result.AlreadyUpToDate ? 'Already up to date.' : 'Pulled the team changes.';
    });
  }

  protected async push(): Promise<void> {
    await this.run('push', async (workspaceRoot) => {
      await GitService.Push(workspaceRoot);
      return 'Pushed your commits.';
    });
  }

  protected async commit(andPush: boolean): Promise<void> {
    const message = this.commitMessage().trim();
    if (message === '') {
      this.errorMessage.set('Describe your changes in the commit message.');
      return;
    }
    const done = await this.run(andPush ? 'commit-push' : 'commit', async (workspaceRoot) => {
      const hash = await GitService.Commit(workspaceRoot, message);
      if (andPush && this.hasUpstream()) {
        await GitService.Push(workspaceRoot);
        return `Committed ${hash.slice(0, 7)} and pushed.`;
      }
      return `Committed ${hash.slice(0, 7)}.`;
    });
    if (done) {
      this.commitMessage.set('');
    }
  }

  protected handleCommitKeydown(event: KeyboardEvent): void {
    if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      event.preventDefault();
      void this.commit(false);
    }
  }

  protected async openConflict(path: string): Promise<void> {
    const workspaceRoot = this.workspaceStore.rootPath();
    if (!workspaceRoot) {
      return;
    }
    this.conflictPath.set(path);
    this.conflictVersions.set(null);
    this.isLoadingConflict.set(true);
    try {
      this.conflictVersions.set(await GitService.ConflictVersions(workspaceRoot, path));
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoadingConflict.set(false);
    }
  }

  protected closeConflict(): void {
    this.conflictPath.set(null);
    this.conflictVersions.set(null);
    this.discardSuggestion();
  }

  /** Asks Claude Code to combine both versions of the open conflict. */
  protected async suggestWithClaude(): Promise<void> {
    const workspaceRoot = this.workspaceStore.rootPath();
    const path = this.conflictPath();
    const versions = this.conflictVersions();
    if (!workspaceRoot || !path || !versions || this.isSuggesting()) {
      return;
    }
    this.isSuggesting.set(true);
    this.suggestionError.set(null);
    this.suggestion.set(null);
    try {
      const result = await AssistantService.SuggestMerge(
        workspaceRoot,
        path,
        decodeContent(versions.Base),
        decodeContent(versions.Local),
        decodeContent(versions.Incoming),
      );
      if (this.conflictPath() === path) {
        this.suggestion.set({ content: result.Content, summary: result.Summary });
      }
    } catch (error) {
      this.suggestionError.set(describeError(error));
    } finally {
      this.isSuggesting.set(false);
    }
  }

  protected editSuggestion(content: string): void {
    this.suggestion.update((suggestion) => (suggestion ? { ...suggestion, content } : null));
  }

  protected discardSuggestion(): void {
    this.suggestion.set(null);
    this.suggestionError.set(null);
  }

  /** Resolves the conflict with Claude's proposal, as edited by the user. */
  protected async useSuggestion(): Promise<void> {
    const suggestion = this.suggestion();
    if (suggestion) {
      await this.resolveWith(suggestion.content, 'the merged');
    }
  }

  protected async resolve(side: 'local' | 'incoming'): Promise<void> {
    const path = this.conflictPath();
    const versions = this.conflictVersions();
    if (!path || !versions) {
      return;
    }
    const content = decodeContent(side === 'local' ? versions.Local : versions.Incoming);
    await this.resolveWith(content, side === 'local' ? 'your' : 'the incoming');
  }

  private async resolveWith(content: string, version: string): Promise<void> {
    const path = this.conflictPath();
    if (!path) {
      return;
    }
    const done = await this.run('resolve', async (workspaceRoot) => {
      await GitService.ResolveWith(workspaceRoot, path, content);
      notifyCollectionsChanged();
      const kept = `Kept ${version} version of ${path}.`;
      // Once the last conflict is resolved, finish the merge so the branch
      // can be pushed: the user never has to know a merge commit is needed.
      if (await GitService.HasConflicts(workspaceRoot)) {
        return kept;
      }
      await GitService.Commit(workspaceRoot, "Merge the team's changes");
      return `${kept} The merge is complete; sync to share it.`;
    });
    if (done) {
      this.closeConflict();
    }
  }

  protected decode(value: string | null): string {
    return decodeContent(value);
  }

  protected languageOf(path: string): CodeLanguage {
    return path.endsWith('.json') ? 'json' : 'text';
  }

  /** Saves a synchronization setting as soon as it changes. */
  protected async updateSync(changes: {
    AutomaticPull?: boolean;
    AutomaticPush?: boolean;
    SynchronizationInterval?: string;
  }): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      return;
    }
    const current = this.settings();
    try {
      const configuration = await WorkspaceService.UpdateGit(rootPath, {
        Remote: current?.Remote ?? '',
        Branch: current?.Branch ?? '',
        AutomaticPull: current?.AutomaticPull ?? false,
        AutomaticPush: current?.AutomaticPush ?? false,
        SynchronizationInterval: current?.SynchronizationInterval || '5m',
        ...changes,
      });
      this.workspaceStore.open(rootPath, configuration);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  protected checked(event: Event): boolean {
    return (event.target as HTMLInputElement).checked;
  }

  /** Runs one git operation, reporting its outcome, then reloads the status. */
  private async run(
    name: string,
    operation: (workspaceRoot: string) => Promise<string>,
  ): Promise<boolean> {
    const workspaceRoot = this.workspaceStore.rootPath();
    if (!workspaceRoot || this.busy()) {
      return false;
    }
    this.busy.set(name);
    this.errorMessage.set(null);
    this.statusMessage.set(null);
    try {
      this.statusMessage.set(await operation(workspaceRoot));
      return true;
    } catch (error) {
      this.errorMessage.set(describeError(error));
      return false;
    } finally {
      this.busy.set(null);
      await this.load();
    }
  }
}

function plural(count: number, word: string): string {
  return `${count} ${word}${count === 1 ? '' : 's'}`;
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
  return error instanceof Error ? error.message : String(error);
}
