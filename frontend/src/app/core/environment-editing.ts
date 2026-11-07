import { Injectable, signal } from '@angular/core';

/** The environment to edit and the scopes defining it. */
export interface EnvironmentEditRequest {
  fileName: string;
  /** '' for the global environment, else a collection path; at least one. */
  scopes: string[];
}

/**
 * Opens the environment edit dialog from anywhere. The dialog is rendered
 * once by the workspace shell.
 */
@Injectable({ providedIn: 'root' })
export class EnvironmentEditing {
  private readonly current = signal<EnvironmentEditRequest | null>(null);

  readonly request = this.current.asReadonly();

  open(request: EnvironmentEditRequest): void {
    this.current.set(request);
  }

  close(): void {
    this.current.set(null);
  }
}
