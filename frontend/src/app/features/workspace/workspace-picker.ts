import { Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import * as WorkspaceService from '@bindings/github.com/clementherve/awake/src/features/workspace/service';
import * as UpdatesService from '@bindings/github.com/clementherve/awake/src/features/updates/service';
import {
  Configuration,
  CreateOptions,
  Mode,
} from '@bindings/github.com/clementherve/awake/src/features/workspace/models';
import { chooseDirectory } from '@core/dialogs';
import { RecentWorkspace, WorkspaceStore } from '@core/workspace-store';
import { Icon } from '@shared/icon';

@Component({
  selector: 'app-workspace-picker',
  imports: [FormsModule, Icon],
  templateUrl: './workspace-picker.html',
  styleUrl: './workspace-picker.css',
})
export class WorkspacePicker {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly router = inject(Router);

  protected readonly workspacePath = signal('');
  protected readonly createPath = signal('');
  protected readonly workspaceName = signal('');
  protected readonly mode = signal<Mode>(Mode.ModeTeam);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly isBusy = signal(false);
  protected readonly version = signal('');
  protected readonly updateError = signal<string | null>(null);
  /** Workspaces opened on this computer, most recent first. */
  protected readonly recent = this.workspaceStore.recent;
  protected readonly busyPath = signal<string | null>(null);

  async ngOnInit(): Promise<void> {
    void UpdatesService.CurrentVersion()
      .then((version) => this.version.set(version))
      .catch(() => this.version.set(''));

    const stored = this.workspaceStore.workspace();
    if (!stored) {
      return;
    }

    try {
      const configuration = await WorkspaceService.Open(stored.rootPath);
      this.workspaceStore.open(stored.rootPath, configuration);
      await this.router.navigate(['/workspace']);
    } catch {
      this.workspaceStore.close();
    }
  }

  protected async openWorkspace(): Promise<void> {
    await this.runOperation(this.workspacePath(), (rootPath) => WorkspaceService.Open(rootPath));
  }

  protected async openRecent(workspace: RecentWorkspace): Promise<void> {
    this.busyPath.set(workspace.rootPath);
    try {
      await this.runOperation(workspace.rootPath, (rootPath) => WorkspaceService.Open(rootPath));
    } finally {
      this.busyPath.set(null);
    }
  }

  protected forgetRecent(workspace: RecentWorkspace): void {
    this.workspaceStore.forget(workspace.rootPath);
  }

  /** Lets the user pick an existing workspace and opens it right away. */
  protected async browseWorkspace(): Promise<void> {
    const directory = await this.pickDirectory({
      title: 'Open a workspace',
      buttonText: 'Open',
      directory: this.workspacePath().trim(),
    });
    if (directory === null) {
      return;
    }
    this.workspacePath.set(directory);
    await this.openWorkspace();
  }

  protected async browseCreateLocation(): Promise<void> {
    const directory = await this.pickDirectory({
      title: 'Choose where to create the workspace',
      buttonText: 'Choose',
      directory: this.createPath().trim(),
    });
    if (directory !== null) {
      this.createPath.set(directory);
    }
  }

  protected async createWorkspace(): Promise<void> {
    await this.runOperation(this.createPath(), (rootPath) => {
      const options: CreateOptions = {
        Name: this.workspaceName().trim(),
        Mode: this.mode(),
        Git: null,
      };
      return WorkspaceService.Create(rootPath, options);
    });
  }

  protected selectMode(event: Event): void {
    this.mode.set((event.target as HTMLSelectElement).value as Mode);
  }

  protected async checkForUpdates(): Promise<void> {
    this.updateError.set(null);
    try {
      await UpdatesService.CheckForUpdates();
    } catch (error) {
      this.updateError.set(describeError(error));
    }
  }

  private async pickDirectory(
    options: Parameters<typeof chooseDirectory>[0],
  ): Promise<string | null> {
    this.errorMessage.set(null);
    try {
      return await chooseDirectory(options);
    } catch (error) {
      this.errorMessage.set(describeError(error));
      return null;
    }
  }

  private async runOperation(
    path: string,
    operation: (rootPath: string) => Promise<Configuration>,
  ): Promise<void> {
    const rootPath = path.trim();
    if (rootPath === '') {
      this.errorMessage.set('Choose a workspace directory first.');
      return;
    }

    this.isBusy.set(true);
    this.errorMessage.set(null);

    try {
      const configuration = await operation(rootPath);
      this.workspaceStore.open(rootPath, configuration);
      await this.router.navigate(['/workspace']);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isBusy.set(false);
    }
  }
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
