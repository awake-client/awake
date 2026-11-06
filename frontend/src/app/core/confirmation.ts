import { Injectable, signal } from '@angular/core';

/** What a confirmation dialog asks. */
export interface ConfirmationRequest {
  title: string;
  message: string;
  confirmLabel: string;
  /** Styles the confirm button as destructive. */
  danger?: boolean;
}

interface PendingConfirmation extends ConfirmationRequest {
  resolve: (confirmed: boolean) => void;
}

/**
 * Asks the user to confirm an action. The dialog itself is rendered once by
 * the workspace shell; callers only await the answer.
 */
@Injectable({ providedIn: 'root' })
export class Confirmation {
  private readonly current = signal<PendingConfirmation | null>(null);

  readonly pending = this.current.asReadonly();

  ask(request: ConfirmationRequest): Promise<boolean> {
    this.current()?.resolve(false);
    return new Promise((resolve) => this.current.set({ ...request, resolve }));
  }

  answer(confirmed: boolean): void {
    const pending = this.current();
    this.current.set(null);
    pending?.resolve(confirmed);
  }
}
