import { Component, computed, inject } from '@angular/core';
import { Router, RouterOutlet } from '@angular/router';
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
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly router = inject(Router);

  protected readonly workspace = this.workspaceStore.workspace;
  protected readonly rootPath = computed(
    () => this.workspaceStore.rootPath() ?? '',
  );
  protected readonly modeLabel = computed(() => {
    const mode = this.workspace()?.configuration.Mode;
    if (!mode) {
      return '';
    }
    return mode.charAt(0).toUpperCase() + mode.slice(1);
  });

  protected openRequest(path: string): void {
    void this.router.navigate(['/workspace/request', ...path.split('/')]);
  }

  protected switchWorkspace(): void {
    this.workspaceStore.close();
    void this.router.navigate(['/']);
  }
}
