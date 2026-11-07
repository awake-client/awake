import { Component, DestroyRef, computed, effect, inject, signal, viewChild } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { filter } from 'rxjs';
import { ScopedSummary } from '@bindings/github.com/clementherve/awake/src/features/environments/models';
import * as EnvironmentsService from '@bindings/github.com/clementherve/awake/src/features/environments/service';
import * as UpdatesService from '@bindings/github.com/clementherve/awake/src/features/updates/service';
import { EnvironmentEditing } from '@core/environment-editing';
import { EnvironmentStore } from '@core/environment-store';
import { EnvironmentDialog } from '../environments/environment-dialog';
import { GitSync } from '@core/git-sync';
import { LayoutStore, SIDEBAR_WIDTH } from '@core/layout-store';
import { TabsStore } from '@core/tabs-store';
import { WorkspaceStore } from '@core/workspace-store';
import { CollectionTree } from '@features/collections/collection-tree';
import { CommandPalette } from '@features/command-palette/command-palette';
import { NewEntryDialog } from '@features/new-entry/new-entry-dialog';
import { ConfirmationDialog } from '@shared/confirmation-dialog';
import { Icon, IconName } from '@shared/icon';
import { PromptDialog } from '@shared/prompt-dialog';
import { ResizeHandle } from '@shared/resize-handle';
import { collectionScopeOf, visibleEnvironments } from './environment-scope';
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
    PromptDialog,
    RequestTabs,
    EnvironmentPicker,
    EnvironmentDialog,
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
  private readonly environmentEditing = inject(EnvironmentEditing);
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
  /** Every environment of the workspace, with its scope. */
  private readonly scopedEnvironments = signal<ScopedSummary[]>([]);
  /** Collection whose environments are offered; a request or its settings set it. */
  private readonly activeScope = signal('');
  private readonly environmentsLoaded = signal(false);
  protected readonly environments = computed(() =>
    visibleEnvironments(this.scopedEnvironments(), this.activeScope()),
  );
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

    // A request, or the settings of its collection, narrows the environments
    // offered to the global ones and those of that collection. Other pages
    // keep the collection of the last request on screen while a request tab
    // is open, and go back to the global environments once none is.
    this.activeScope.set(collectionScopeOf(this.router.url));
    this.router.events
      .pipe(
        filter((event) => event instanceof NavigationEnd),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe(() => {
        const scope =
          collectionScopeOf(this.router.url) ||
          (this.tabs.tabs().length > 0 ? this.activeScope() : '');
        if (scope !== this.activeScope()) {
          this.activeScope.set(scope);
          // Each scope keeps the environment last chosen in it.
          this.environmentStore.useScope(scope);
        }
      });

    // A selection hidden by the new context falls back to the default one.
    effect(() => {
      if (!this.environmentsLoaded()) {
        return;
      }
      const options = this.environments();
      const selected = this.environmentStore.selected();
      if (selected && !options.some((option) => option.FileName === selected)) {
        this.environmentStore.select(this.defaultEnvironment(options));
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

  protected editEnvironment(environment: EnvironmentOption): void {
    this.environmentEditing.open({ fileName: environment.FileName, scopes: environment.Scopes });
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
    this.environmentStore.useScope(this.activeScope());
    // Keep the choice remembered for this collection in this workspace.
    const previousSelection = this.environmentStore.selected();

    try {
      const summaries = (await EnvironmentsService.ListAll(rootPath)) ?? [];
      this.scopedEnvironments.set(summaries);
      this.environmentsLoaded.set(true);

      const options = visibleEnvironments(summaries, this.activeScope());
      if (previousSelection && options.some((option) => option.FileName === previousSelection)) {
        this.environmentStore.select(previousSelection);
        return;
      }

      this.environmentStore.select(this.defaultEnvironment(options));
    } catch {
      this.scopedEnvironments.set([]);
      this.environmentsLoaded.set(true);
      this.environmentStore.select(previousSelection);
    }
  }

  /** The environment configured as default, when it is offered here. */
  private defaultEnvironment(options: EnvironmentOption[]): string {
    const defaultEnvironment =
      this.workspaceStore.workspace()?.configuration.DefaultEnvironment ?? '';
    return options.some((option) => option.FileName === defaultEnvironment)
      ? defaultEnvironment
      : '';
  }
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
