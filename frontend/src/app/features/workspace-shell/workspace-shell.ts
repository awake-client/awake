import {
  Component,
  DestroyRef,
  computed,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { Router, RouterLink, RouterOutlet } from '@angular/router';
import * as EnvironmentsService from '@bindings/github.com/clementherve/awake/src/features/environments/service';
import { Summary } from '@bindings/github.com/clementherve/awake/src/features/environments/models';
import * as UpdatesService from '@bindings/github.com/clementherve/awake/src/features/updates/service';
import { EnvironmentStore } from '@core/environment-store';
import { WorkspaceStore } from '@core/workspace-store';
import { CollectionTree } from '@features/collections/collection-tree';
import { CommandPalette } from '@features/command-palette/command-palette';

/**
 * Application frame for an open workspace: collection tree on the left, the
 * selected request on the right.
 */
@Component({
  selector: 'app-workspace-shell',
  imports: [RouterOutlet, RouterLink, CollectionTree, CommandPalette],
  templateUrl: './workspace-shell.html',
  styleUrl: './workspace-shell.css',
})
export class WorkspaceShell {
  private readonly environmentStore = inject(EnvironmentStore);
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly router = inject(Router);
  private readonly destroyRef = inject(DestroyRef);
  private readonly commandPalette = viewChild(CommandPalette);

  protected readonly workspace = this.workspaceStore.workspace;
  protected readonly rootPath = computed(
    () => this.workspaceStore.rootPath() ?? '',
  );
  protected readonly environments = signal<Summary[]>([]);
  protected readonly selectedEnvironment = this.environmentStore.selected;
  protected readonly version = signal('');
  protected readonly updateError = signal<string | null>(null);
  protected readonly modeLabel = computed(() => {
    const mode = this.workspace()?.configuration.Mode;
    if (!mode) {
      return '';
    }
    return mode.charAt(0).toUpperCase() + mode.slice(1);
  });
  protected readonly selectedEnvironmentLabel = computed(() => {
    const selected = this.selectedEnvironment();
    if (!selected) {
      return 'No environment';
    }
    const environment = this.environments().find(
      (summary) => summary.FileName === selected,
    );
    return environment?.Name ?? selected;
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
      window.removeEventListener(
        'awake:environments-changed',
        reloadEnvironments,
      ),
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

  protected selectEnvironment(event: Event): void {
    this.environmentStore.select((event.target as HTMLSelectElement).value);
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
    const previousSelection = this.environmentStore.selected();

    try {
      const summaries = (await EnvironmentsService.List(rootPath)) ?? [];
      this.environments.set(summaries);

      if (
        previousSelection &&
        summaries.some((summary) => summary.FileName === previousSelection)
      ) {
        this.environmentStore.select(previousSelection);
        return;
      }

      const defaultEnvironment =
        this.workspaceStore.workspace()?.configuration.DefaultEnvironment ??
        '';
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
