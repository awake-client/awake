import { Component, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import * as WorkspaceService from '@bindings/github.com/clementherve/awake/src/features/workspace/service';
import {
  Configuration,
  CreateOptions,
  Mode,
} from '@bindings/github.com/clementherve/awake/src/features/workspace/models';

@Component({
  selector: 'app-workspace-picker',
  imports: [FormsModule],
  templateUrl: './workspace-picker.html',
  styleUrl: './workspace-picker.css',
})
export class WorkspacePicker {
  protected readonly workspacePath = signal('');
  protected readonly workspaceName = signal('');
  protected readonly mode = signal<Mode>(Mode.ModeTeam);
  protected readonly openedWorkspace = signal<Configuration | null>(null);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly isBusy = signal(false);

  protected async openWorkspace(): Promise<void> {
    this.runOperation(async () => {
      const configuration = await WorkspaceService.Open(this.workspacePath().trim());
      return configuration;
    });
  }

  protected async createWorkspace(): Promise<void> {
    this.runOperation(async () => {
      const options: CreateOptions = {
        Name: this.workspaceName().trim(),
        Mode: this.mode(),
        Git: null,
      };
      return WorkspaceService.Create(this.workspacePath().trim(), options);
    });
  }

  protected selectMode(event: Event): void {
    this.mode.set((event.target as HTMLSelectElement).value as Mode);
  }

  private async runOperation(operation: () => Promise<Configuration>): Promise<void> {
    this.isBusy.set(true);
    this.errorMessage.set(null);

    try {
      const configuration = await operation();
      this.openedWorkspace.set(configuration);
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
