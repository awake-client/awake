import { DestroyRef, Injectable, computed, effect, inject, signal } from '@angular/core';
import * as GitService from '@bindings/github.com/clementherve/awake/src/features/git/service';
import { SyncOutcome } from '@bindings/github.com/clementherve/awake/src/features/git/models';
import { notifyCollectionsChanged } from './collection-events';
import { WorkspaceStore } from './workspace-store';

/** Where automatic synchronization stands. */
export type SyncState = 'off' | 'idle' | 'syncing' | 'synced' | 'skipped' | 'conflicts' | 'error';

/** Default and minimum delays between two synchronizations. */
const DEFAULT_PERIOD = 5 * 60_000;
const MINIMUM_PERIOD = 30_000;
/** Lets the window settle after opening a workspace before the first sync. */
const FIRST_SYNC_DELAY = 5_000;

/**
 * Pulls and pushes the workspace on a timer, following the git settings of
 * awake.yaml. After a pull that brought commits, the collection tree and the
 * environments reload so the team's changes show up without a restart.
 */
@Injectable({ providedIn: 'root' })
export class GitSync {
  private readonly workspaceStore = inject(WorkspaceStore);
  private timer: number | undefined;
  private firstRun: number | undefined;

  readonly state = signal<SyncState>('off');
  readonly message = signal('');
  readonly conflicts = signal<string[]>([]);
  readonly lastSync = signal<Date | null>(null);

  private readonly clock = signal(Date.now());

  readonly settings = computed(() => this.workspaceStore.workspace()?.configuration.Git ?? null);
  readonly enabled = computed(() => {
    const settings = this.settings();
    return !!settings && (settings.AutomaticPull || settings.AutomaticPush);
  });

  /** Short status for the sidebar, such as "Synced 3 min ago". */
  readonly label = computed(() => {
    switch (this.state()) {
      case 'off':
        return 'Sync off';
      case 'syncing':
        return 'Syncing…';
      case 'conflicts':
        return 'Conflicts to resolve';
      case 'error':
        return 'Sync failed';
      case 'skipped':
        return 'Not synced';
      case 'synced':
        return `Synced ${relativeTime(this.lastSync(), this.clock())}`;
      default:
        return 'Waiting to sync';
    }
  });

  constructor() {
    effect(() => {
      const rootPath = this.workspaceStore.rootPath();
      const settings = this.settings();
      this.stop();
      if (!rootPath || !this.enabled()) {
        this.state.set('off');
        return;
      }
      this.state.set('idle');
      const period = Math.max(parsePeriod(settings?.SynchronizationInterval ?? ''), MINIMUM_PERIOD);
      this.firstRun = window.setTimeout(() => void this.syncNow(), FIRST_SYNC_DELAY);
      this.timer = window.setInterval(() => void this.syncNow(), period);
    });

    const tick = window.setInterval(() => this.clock.set(Date.now()), 30_000);
    inject(DestroyRef).onDestroy(() => {
      this.stop();
      window.clearInterval(tick);
    });
  }

  /** Runs one synchronization now, unless one is already running. */
  async syncNow(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const settings = this.settings();
    if (!rootPath || this.state() === 'syncing') {
      return;
    }

    this.state.set('syncing');
    try {
      const result = await GitService.Synchronize(
        rootPath,
        settings?.AutomaticPull ?? true,
        settings?.AutomaticPush ?? true,
      );
      this.conflicts.set(result.Conflicts ?? []);
      switch (result.Outcome) {
        case SyncOutcome.SyncConflicts:
          this.state.set('conflicts');
          this.message.set(`${result.Conflicts?.length ?? 0} conflicted file(s)`);
          break;
        case SyncOutcome.SyncSkipped:
          this.state.set('skipped');
          this.message.set(result.Reason);
          break;
        default:
          this.state.set('synced');
          this.lastSync.set(new Date());
          this.clock.set(Date.now());
          this.message.set(describeResult(result.Pulled, result.Pushed));
      }
      if (result.Pulled) {
        notifyCollectionsChanged();
        window.dispatchEvent(new Event('awake:environments-changed'));
      }
    } catch (error) {
      this.state.set('error');
      this.message.set(error instanceof Error ? error.message : String(error));
    }
  }

  private stop(): void {
    window.clearTimeout(this.firstRun);
    window.clearInterval(this.timer);
  }
}

/** Parses a Go duration such as "90s", "5m" or "1h30m" into milliseconds. */
export function parsePeriod(value: string): number {
  const units: Record<string, number> = { ms: 1, s: 1_000, m: 60_000, h: 3_600_000 };
  let total = 0;
  let matched = '';
  for (const match of value.trim().matchAll(/(\d+(?:\.\d+)?)(ms|s|m|h)/g)) {
    total += Number(match[1]) * units[match[2]];
    matched += match[0];
  }
  return matched !== '' && matched === value.trim() ? total : DEFAULT_PERIOD;
}

function relativeTime(date: Date | null, now: number): string {
  if (!date) {
    return '';
  }
  const minutes = Math.floor((now - date.getTime()) / 60_000);
  if (minutes < 1) {
    return 'just now';
  }
  if (minutes < 60) {
    return `${minutes} min ago`;
  }
  return `${Math.floor(minutes / 60)} h ago`;
}

function describeResult(pulled: boolean, pushed: boolean): string {
  if (pulled && pushed) {
    return 'Pulled the team changes and pushed yours.';
  }
  if (pulled) {
    return 'Pulled the team changes.';
  }
  if (pushed) {
    return 'Pushed your commits.';
  }
  return 'Already up to date.';
}
