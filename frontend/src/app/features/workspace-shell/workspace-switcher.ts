import { Component, ElementRef, computed, inject, signal } from '@angular/core';
import { Router } from '@angular/router';
import * as WorkspaceService from '@bindings/github.com/awake-client/awake/src/features/workspace/service';
import { chooseDirectory } from '@core/dialogs';
import { RecentWorkspace, WorkspaceStore } from '@core/workspace-store';
import { Icon } from '@shared/icon';

/**
 * Workspace menu of the title bar: the workspaces opened before, one click
 * away, plus opening another folder or creating a workspace.
 */
@Component({
  selector: 'app-workspace-switcher',
  imports: [Icon],
  host: {
    '(document:click)': 'closeOutside($event)',
    '(document:keydown.escape)': 'isOpen.set(false)',
  },
  templateUrl: './workspace-switcher.html',
  styleUrl: './workspace-switcher.css',
})
export class WorkspaceSwitcher {
  private readonly store = inject(WorkspaceStore);
  private readonly router = inject(Router);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  protected readonly isOpen = signal(false);
  protected readonly busyPath = signal<string | null>(null);
  protected readonly errors = signal<Record<string, string>>({});

  protected readonly current = this.store.workspace;
  protected readonly recent = this.store.recent;
  protected readonly name = computed(() => this.current()?.configuration.Name ?? 'Workspace');
  protected readonly color = computed(() => this.current()?.configuration.Color ?? '');
  protected readonly initial = computed(() => this.name().trim().charAt(0).toUpperCase() || 'W');

  protected toggle(): void {
    this.isOpen.update((open) => !open);
    this.errors.set({});
  }

  protected isCurrent(workspace: RecentWorkspace): boolean {
    return workspace.rootPath === this.current()?.rootPath;
  }

  protected async switchTo(workspace: RecentWorkspace): Promise<void> {
    if (this.isCurrent(workspace)) {
      this.isOpen.set(false);
      return;
    }
    await this.open(workspace.rootPath);
  }

  protected async openFolder(): Promise<void> {
    this.isOpen.set(false);
    const directory = await chooseDirectory({ title: 'Open a workspace', buttonText: 'Open' });
    if (directory) {
      await this.open(directory);
    }
  }

  protected overview(): void {
    this.isOpen.set(false);
    void this.router.navigate(['/workspace']);
  }

  /** Leaves the workspace for the start screen, to create a new one. */
  protected createWorkspace(): void {
    this.isOpen.set(false);
    this.store.close();
    void this.router.navigate(['/']);
  }

  protected forget(workspace: RecentWorkspace, event: Event): void {
    event.stopPropagation();
    this.store.forget(workspace.rootPath);
  }

  protected closeOutside(event: MouseEvent): void {
    if (this.isOpen() && !this.host.nativeElement.contains(event.target as Node)) {
      this.isOpen.set(false);
    }
  }

  private async open(rootPath: string): Promise<void> {
    this.busyPath.set(rootPath);
    try {
      const configuration = await WorkspaceService.Open(rootPath);
      this.store.open(rootPath, configuration);
      this.isOpen.set(false);
      await this.router.navigate(['/workspace']);
    } catch (error) {
      // Keep the menu open and say what went wrong next to the workspace.
      this.isOpen.set(true);
      this.errors.update((errors) => ({
        ...errors,
        [rootPath]: error instanceof Error ? error.message : 'This workspace cannot be opened.',
      }));
    } finally {
      this.busyPath.set(null);
    }
  }
}
