import { Component, computed, effect, inject, signal } from '@angular/core';
import { Router, RouterOutlet } from '@angular/router';
import * as EnvironmentsService from '@bindings/github.com/clementherve/awake/src/features/environments/service';
import { Summary } from '@bindings/github.com/clementherve/awake/src/features/environments/models';
import { EnvironmentStore } from '@core/environment-store';
import { WorkspaceStore } from '@core/workspace-store';
import { CollectionTree } from '@features/collections/collection-tree';

/**
 * Application frame for an open workspace: collection tree on the left, the
 * selected request on the right.
 */
@Component({
  selector: 'app-workspace-shell',
  imports: [RouterOutlet, CollectionTree],
  templateUrl: './workspace-shell.html',
  styleUrl: './workspace-shell.css',
})
export class WorkspaceShell {
  private readonly environmentStore = inject(EnvironmentStore);
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly router = inject(Router);

  protected readonly workspace = this.workspaceStore.workspace;
  protected readonly rootPath = computed(
    () => this.workspaceStore.rootPath() ?? '',
  );
  protected readonly environments = signal<Summary[]>([]);
  protected readonly selectedEnvironment = this.environmentStore.selected;
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
  }

  protected openRequest(path: string): void {
    void this.router.navigate(['/workspace/request', ...path.split('/')]);
  }

  protected selectEnvironment(event: Event): void {
    this.environmentStore.select((event.target as HTMLSelectElement).value);
  }

  protected switchWorkspace(): void {
    this.workspaceStore.close();
    void this.router.navigate(['/']);
  }

  private async loadEnvironments(rootPath: string): Promise<void> {
    try {
      const summaries = (await EnvironmentsService.List(rootPath)) ?? [];
      this.environments.set(summaries);

      const defaultEnvironment =
        this.workspaceStore.workspace()?.configuration.DefaultEnvironment ??
        '';
      if (
        defaultEnvironment &&
        summaries.some((summary) => summary.FileName === defaultEnvironment)
      ) {
        this.environmentStore.select(defaultEnvironment);
      }
    } catch {
      this.environments.set([]);
    }
  }
}
