import { Component, DestroyRef, computed, effect, inject, signal, viewChild } from '@angular/core';
import { Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import * as EnvironmentsService from '@bindings/github.com/clementherve/awake/src/features/environments/service';
import * as UpdatesService from '@bindings/github.com/clementherve/awake/src/features/updates/service';
import { EnvironmentStore } from '@core/environment-store';
import { GitSync } from '@core/git-sync';
import { LayoutStore, SIDEBAR_WIDTH } from '@core/layout-store';
import { TabsStore } from '@core/tabs-store';
import { WorkspaceStore } from '@core/workspace-store';
import { CollectionTree } from '@features/collections/collection-tree';
import { CommandPalette } from '@features/command-palette/command-palette';
import { NewEntryDialog } from '@features/new-entry/new-entry-dialog';
import { ConfirmationDialog } from '@shared/confirmation-dialog';
import { Icon, IconName } from '@shared/icon';
import { ResizeHandle } from '@shared/resize-handle';
import { EnvironmentChoice, EnvironmentPicker } from './environment-picker';
import { RequestTabs } from './request-tabs';
import { ThemePicker } from './theme-picker';
import { WorkspaceSwitcher } from './workspace-switcher';

type EnvironmentOption = EnvironmentChoice;

/** Secondary workspace tools listed under the collection tree. */
interface SidebarSection {
  route: string;
  label: string;
  icon: IconName;
}

const SIDEBAR_SECTIONS: SidebarSection[] = [
  { route: '/workspace/assistant', label: 'Assistant', icon: 'sparkles' },
  { route: '/workspace/environments', label: 'Environments', icon: 'globe' },
  { route: '/workspace/history', label: 'History', icon: 'history' },
  { route: '/workspace/git', label: 'Git', icon: 'git' },
  { route: '/workspace/interop', label: 'Import / Export', icon: 'download' },
];

/**
 * Application frame for an open workspace: collection tree on the left, the
 * selected request on the right.
 */
@Component({
  selector: 'app-workspace-shell',
  imports: [
    RouterOutlet,
    RouterLink,
    RouterLinkActive,
    CollectionTree,
    CommandPalette,
    NewEntryDialog,
    ConfirmationDialog,
    RequestTabs,
    EnvironmentPicker,
    ThemePicker,
    WorkspaceSwitcher,
    ResizeHandle,
    Icon,
  ],
  templateUrl: './workspace-shell.html',
  styleUrl: './workspace-shell.css',
})
export class WorkspaceShell {
  private readonly environmentStore = inject(EnvironmentStore);
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly router = inject(Router);
  private readonly destroyRef = inject(DestroyRef);
  private readonly commandPalette = viewChild(CommandPalette);

  /** Created with the shell so synchronization runs while a workspace is open. */
  protected readonly gitSync = inject(GitSync);
  protected readonly tabs = inject(TabsStore);
  protected readonly layout = inject(LayoutStore);
  protected readonly defaultSidebarWidth = SIDEBAR_WIDTH.default;
  protected readonly sections = SIDEBAR_SECTIONS;
  protected readonly workspace = this.workspaceStore.workspace;
  protected readonly rootPath = computed(() => this.workspaceStore.rootPath() ?? '');
  protected readonly environments = signal<EnvironmentOption[]>([]);
  protected readonly selectedEnvironment = this.environmentStore.selected;
  protected readonly version = signal('');
  protected readonly updateError = signal<string | null>(null);
  /** Color of the active environment, shown along the top of the work area. */
  protected readonly environmentColor = computed(
    () =>
      this.environments().find((environment) => environment.FileName === this.selectedEnvironment())
        ?.Color ?? '',
  );

  protected readonly modeLabel = computed(() => {
    const mode = this.workspace()?.configuration.Mode;
    if (!mode) {
      return '';
    }
    return mode.charAt(0).toUpperCase() + mode.slice(1);
  });

  constructor() {
    effect(() => {
      const rootPath = this.rootPath();
      if (rootPath) {
        void this.loadEnvironments(rootPath);
      }
    });

    const reloadEnvironments = () => {
      const rootPath = this.rootPath();
      if (rootPath) {
        void this.loadEnvironments(rootPath);
      }
    };

    window.addEventListener('awake:environments-changed', reloadEnvironments);
    this.destroyRef.onDestroy(() =>
      window.removeEventListener('awake:environments-changed', reloadEnvironments),
    );

    void UpdatesService.CurrentVersion()
      .then((version) => this.version.set(version))
      .catch(() => this.version.set(''));
  }

  protected openRequest(path: string): void {
    void this.router.navigate(['/workspace/request', ...path.split('/')]);
  }

  protected openCommandPalette(): void {
    void this.commandPalette()?.open();
  }

  protected chooseEnvironment(fileName: string): void {
    this.environmentStore.select(fileName);
  }

  protected switchWorkspace(): void {
    this.workspaceStore.close();
    void this.router.navigate(['/']);
  }

  protected async checkForUpdates(): Promise<void> {
    this.updateError.set(null);
    try {
      await UpdatesService.CheckForUpdates();
    } catch (error) {
      this.updateError.set(describeError(error));
    }
  }

  private async loadEnvironments(rootPath: string): Promise<void> {
    this.environmentStore.useWorkspace(rootPath);
    // Keep the current choice, or restore the one remembered for this workspace.
    const previousSelection =
      this.environmentStore.selected() || this.environmentStore.remembered(rootPath);

    try {
      const options = new Map<string, EnvironmentOption>();
      for (const scoped of (await EnvironmentsService.ListAll(rootPath)) ?? []) {
        const existing = options.get(scoped.FileName);
        if (!existing || (existing.CollectionOnly && scoped.Scope === '')) {
          options.set(scoped.FileName, {
            FileName: scoped.FileName,
            Name: scoped.Name,
            HasLocalOverrides: scoped.HasLocalOverrides || !!existing?.HasLocalOverrides,
            CollectionOnly: scoped.Scope !== '',
            Color: scoped.Color || existing?.Color || '',
          });
        } else if (scoped.HasLocalOverrides) {
          existing.HasLocalOverrides = true;
        }
      }
      const summaries = [...options.values()];
      this.environments.set(summaries);

      if (
        previousSelection &&
        summaries.some((summary) => summary.FileName === previousSelection)
      ) {
        this.environmentStore.select(previousSelection);
        return;
      }

      const defaultEnvironment =
        this.workspaceStore.workspace()?.configuration.DefaultEnvironment ?? '';
      if (
        defaultEnvironment &&
        summaries.some((summary) => summary.FileName === defaultEnvironment)
      ) {
        this.environmentStore.select(defaultEnvironment);
        return;
      }

      this.environmentStore.select('');
    } catch {
      this.environments.set([]);
      this.environmentStore.select(previousSelection);
    }
  }
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
