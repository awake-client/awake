import { Injectable, signal } from '@angular/core';
import { IconName } from '@shared/icon';

/** What a text prompt asks. */
export interface PromptRequest {
  title: string;
  message?: string;
  label: string;
  /** Icon of the dialog badge; a pencil when omitted. */
  icon?: IconName;
  /** Initial text of the field. */
  value: string;
  confirmLabel: string;
}

interface PendingPrompt extends PromptRequest {
  resolve: (value: string | null) => void;
}

/**
 * Asks the user for a short text. The dialog is rendered once by the
 * workspace shell; callers only await the answer, or null when cancelled.
 */
@Injectable({ providedIn: 'root' })
export class Prompt {
  private readonly current = signal<PendingPrompt | null>(null);

  readonly pending = this.current.asReadonly();

  ask(request: PromptRequest): Promise<string | null> {
    this.current()?.resolve(null);
    return new Promise((resolve) => this.current.set({ ...request, resolve }));
  }

  answer(value: string | null): void {
    const pending = this.current();
    this.current.set(null);
    pending?.resolve(value);
  }
}
