import { Injectable, signal } from '@angular/core';

/**
 * Holds the environment selected for the current session. The selection is not
 * persisted so each launch starts from the workspace default.
 */
@Injectable({ providedIn: 'root' })
export class EnvironmentStore {
  private readonly current = signal('');

  readonly selected = this.current.asReadonly();

  select(fileName: string): void {
    this.current.set(fileName);
  }
}
