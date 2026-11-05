import { Component, computed, inject } from '@angular/core';
import { WorkspaceStore } from '@core/workspace-store';

/** Placeholder shown in the editor area until a request is selected. */
@Component({
  selector: 'app-workspace-welcome',
  template: `
    <div class="welcome">
      <h1>{{ workspaceName() }}</h1>
      <p>Select a request in the collection tree, or create a collection to get started.</p>
    </div>
  `,
  styles: `
    :host {
      display: grid;
      place-items: center;
      height: 100%;
      padding: 32px;
      text-align: center;
    }

    h1 {
      margin: 0 0 8px;
      font-size: 1.25rem;
    }

    p {
      margin: 0;
      max-width: 42ch;
      color: var(--color-text-muted);
    }
  `,
})
export class WorkspaceWelcome {
  private readonly workspaceStore = inject(WorkspaceStore);

  protected readonly workspaceName = computed(
    () => this.workspaceStore.workspace()?.configuration.Name ?? 'Workspace',
  );
}
