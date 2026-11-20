import { Injectable, computed, signal } from '@angular/core';
import { Browser, Events } from '@wailsio/runtime';
import {
  Release,
  Status,
} from '@bindings/github.com/awake-client/awake/src/features/updates/models';
import * as UpdatesService from '@bindings/github.com/awake-client/awake/src/features/updates/service';

/** Where the update flow stands, as the dialog and the sidebar show it. */
export type UpdatePhase =
  | 'idle'
  | 'checking'
  | 'up-to-date'
  | 'available'
  | 'downloading'
  | 'installing'
  | 'ready'
  | 'restarting'
  | 'check-failed'
  | 'install-failed';

/** Bytes downloaded so far, and the total when the server announced it. */
export interface DownloadProgress {
  written: number;
  total: number;
}

/** Events the Wails updater emits while it installs an update. */
const UPDATER_EVENTS = {
  downloadStarted: 'wails:updater:download-started',
  downloadProgress: 'wails:updater:download-progress',
  verifying: 'wails:updater:verifying',
  installing: 'wails:updater:installing',
  ready: 'wails:updater:update-ready',
  error: 'wails:updater:error',
} as const;

/** Delay before the first silent check, so it never slows the start. */
const STARTUP_CHECK_DELAY = 5_000;

const INSTALLING_PHASES: ReadonlySet<UpdatePhase> = new Set([
  'downloading',
  'installing',
  'ready',
  'restarting',
]);

/**
 * Drives application updates: a check that finds a release and its changes,
 * an installation the user confirms, and the restart into the new version.
 * The update dialog renders this state; the sidebar shows a hint when an
 * update waits.
 */
@Injectable({ providedIn: 'root' })
export class UpdateStore {
  readonly currentVersion = signal('');
  readonly phase = signal<UpdatePhase>('idle');
  readonly release = signal<Release | null>(null);
  /** Why the update must be installed by hand; empty when it installs here. */
  readonly installBlocker = signal('');
  readonly progress = signal<DownloadProgress | null>(null);
  readonly error = signal<string | null>(null);
  readonly dialogOpen = signal(false);

  /** A short label for the sidebar while an update waits for the user. */
  readonly hint = computed(() => {
    switch (this.phase()) {
      case 'available':
        return 'Update available';
      case 'downloading':
      case 'installing':
        return 'Updating…';
      case 'ready':
        return 'Restart to update';
      case 'install-failed':
        return 'Update failed';
      default:
        return null;
    }
  });

  private startupCheckScheduled = false;

  constructor() {
    void UpdatesService.CurrentVersion()
      .then((version) => this.currentVersion.set(version))
      .catch(() => this.currentVersion.set(''));
    this.listenToUpdater();
  }

  /** Checks once, silently, a few seconds after a release build starts. */
  scheduleStartupCheck(): void {
    if (this.startupCheckScheduled) {
      return;
    }
    this.startupCheckScheduled = true;
    setTimeout(() => void this.checkQuietly(), STARTUP_CHECK_DELAY);
  }

  /** Opens the dialog and looks for an update, unless one is under way. */
  async check(): Promise<void> {
    this.dialogOpen.set(true);
    if (INSTALLING_PHASES.has(this.phase())) {
      return;
    }
    this.phase.set('checking');
    this.error.set(null);
    try {
      this.apply(await UpdatesService.Check());
    } catch (error) {
      this.error.set(describeError(error));
      this.phase.set('check-failed');
    }
  }

  /** Downloads and stages the update the user confirmed. */
  async install(): Promise<void> {
    const release = this.release();
    if (!release || this.installBlocker()) {
      return;
    }
    this.error.set(null);
    this.progress.set({ written: 0, total: release.size });
    this.phase.set('downloading');
    try {
      await UpdatesService.Install();
    } catch (error) {
      this.fail(error);
    }
  }

  /** Quits and starts the installed version. */
  async restart(): Promise<void> {
    this.phase.set('restarting');
    try {
      await UpdatesService.Restart();
    } catch (error) {
      this.fail(error);
    }
  }

  /** Opens the release page, where the update can be downloaded by hand. */
  openReleasePage(): void {
    const url = this.release()?.url;
    if (url) {
      void Browser.OpenURL(url);
    }
  }

  openDialog(): void {
    this.dialogOpen.set(true);
  }

  /** Hides the dialog; a download in progress carries on. */
  closeDialog(): void {
    this.dialogOpen.set(false);
    const phase = this.phase();
    if (phase === 'checking' || phase === 'up-to-date' || phase === 'check-failed') {
      this.phase.set('idle');
    }
  }

  private async checkQuietly(): Promise<void> {
    if (this.phase() !== 'idle') {
      return;
    }
    try {
      const version = this.currentVersion() || (await UpdatesService.CurrentVersion());
      if (!isReleaseVersion(version)) {
        return;
      }
      const status = await UpdatesService.Check();
      // A check the user started meanwhile has the last word.
      if (this.phase() === 'idle' && status.release) {
        this.apply(status);
      }
    } catch {
      // A silent check stays silent: the user can still check by hand.
    }
  }

  private apply(status: Status): void {
    this.currentVersion.set(status.currentVersion);
    this.release.set(status.release);
    this.installBlocker.set(status.installBlocker);
    this.phase.set(phaseOf(status));
  }

  private fail(error: unknown): void {
    this.error.set(describeError(error));
    this.phase.set('install-failed');
    this.dialogOpen.set(true);
  }

  private listenToUpdater(): void {
    Events.On(UPDATER_EVENTS.downloadStarted, () => this.phase.set('downloading'));
    Events.On(UPDATER_EVENTS.downloadProgress, (event) => {
      const progress = payloadOf<Partial<DownloadProgress>>(event.data);
      this.progress.set({ written: progress?.written ?? 0, total: progress?.total ?? 0 });
    });
    Events.On(UPDATER_EVENTS.verifying, () => this.phase.set('installing'));
    Events.On(UPDATER_EVENTS.installing, () => this.phase.set('installing'));
    Events.On(UPDATER_EVENTS.ready, () => {
      this.phase.set('ready');
      this.dialogOpen.set(true);
    });
    Events.On(UPDATER_EVENTS.error, (event) => {
      // Check failures come back from Check itself; only an installation
      // running in the background needs the event.
      if (this.phase() === 'downloading' || this.phase() === 'installing') {
        const info = payloadOf<{ message?: string }>(event.data);
        this.fail(info?.message ?? 'The update could not be installed.');
      }
    });
  }
}

/** Maps the updater state reported by Go to the phase the dialog renders. */
export function phaseOf(status: Status): UpdatePhase {
  switch (status.state) {
    case 'downloading':
      return 'downloading';
    case 'verifying':
    case 'installing':
      return 'installing';
    case 'ready':
      return 'ready';
    default:
      return status.release ? 'available' : 'up-to-date';
  }
}

/** Reports whether a version is a published release rather than "dev". */
export function isReleaseVersion(version: string): boolean {
  return /^v?\d+(\.\d+)*(-[0-9A-Za-z.-]+)?$/.test(version.trim());
}

/** Wails wraps the payload in an array when an event carries several. */
function payloadOf<T>(data: unknown): T | undefined {
  return (Array.isArray(data) ? data[0] : data) as T | undefined;
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
