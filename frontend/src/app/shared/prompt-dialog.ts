import { Component, ElementRef, effect, inject, signal, viewChild } from '@angular/core';
import { Prompt } from '@core/prompt';
import { Icon } from './icon';

/** Renders the pending prompt as a modal dialog with one text field. */
@Component({
  selector: 'app-prompt-dialog',
  imports: [Icon],
  template: `
    <dialog
      #dialog
      class="prompt"
      (cancel)="$event.preventDefault(); prompt.answer(null)"
      (click)="closeOnBackdrop($event)"
    >
      @if (prompt.pending(); as request) {
        <form (submit)="$event.preventDefault(); submit()">
          <div class="body">
            <span class="badge" aria-hidden="true">
              <app-icon [name]="request.icon ?? 'pencil'" [size]="16" />
            </span>
            <div class="text">
              <h2>{{ request.title }}</h2>
              @if (request.message) {
                <p>{{ request.message }}</p>
              }
              <label class="field">
                <span class="field-label">{{ request.label }}</span>
                <input
                  #input
                  type="text"
                  autocomplete="off"
                  spellcheck="false"
                  [value]="value()"
                  (input)="value.set(inputValue($event))"
                />
              </label>
            </div>
          </div>
          <footer>
            <button type="button" class="btn btn-ghost" (click)="prompt.answer(null)">
              Cancel
            </button>
            <button type="submit" class="btn btn-primary" [disabled]="value().trim() === ''">
              {{ request.confirmLabel }}
            </button>
          </footer>
        </form>
      }
    </dialog>
  `,
  styles: `
    .prompt {
      width: min(440px, calc(100vw - 48px));
      padding: 0;
      border: none;
      border-radius: var(--radius-xl);
      background: var(--color-surface);
      color: var(--color-text);
      box-shadow: var(--shadow-raised);
    }
    .prompt::backdrop {
      background: var(--color-backdrop);
      backdrop-filter: blur(3px);
    }
    .body {
      display: flex;
      gap: 14px;
      padding: 22px 22px 18px;
    }
    .badge {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      flex: 0 0 auto;
      width: 34px;
      height: 34px;
      border-radius: var(--radius-md);
      background: var(--color-accent-soft);
      color: var(--color-accent);
    }
    .text {
      flex: 1;
      min-width: 0;
    }
    h2 {
      margin: 4px 0 6px;
      font-size: var(--text-md);
      font-weight: 650;
      color: var(--color-text-strong);
    }
    p {
      margin: 0 0 12px;
      line-height: 1.55;
      color: var(--color-text-muted);
      overflow-wrap: anywhere;
    }
    .field {
      display: flex;
      flex-direction: column;
      gap: 6px;
      margin-top: 4px;
    }
    .field-label {
      font-size: var(--text-sm);
      color: var(--color-text-subtle);
    }
    input {
      width: 100%;
      height: var(--control-height);
      padding: 0 10px;
      border: 1px solid var(--color-border);
      border-radius: var(--radius-sm);
      background: var(--color-background);
      color: var(--color-text-strong);
    }
    input:focus {
      border-color: var(--color-accent-border);
      outline: none;
    }
    footer {
      display: flex;
      justify-content: flex-end;
      gap: 8px;
      padding: 12px 22px;
      border-top: 1px solid var(--color-border);
      background: color-mix(in srgb, var(--color-window) 35%, var(--color-surface));
    }
  `,
})
export class PromptDialog {
  protected readonly prompt = inject(Prompt);
  protected readonly value = signal('');
  private readonly dialog = viewChild.required<ElementRef<HTMLDialogElement>>('dialog');
  private readonly input = viewChild<ElementRef<HTMLInputElement>>('input');

  constructor() {
    effect(() => {
      const request = this.prompt.pending();
      const dialog = this.dialog().nativeElement;
      if (request) {
        this.value.set(request.value);
        if (!dialog.open) {
          dialog.showModal();
        }
        setTimeout(() => {
          const input = this.input()?.nativeElement;
          input?.focus();
          input?.select();
        });
      } else if (dialog.open) {
        dialog.close();
      }
    });
  }

  protected inputValue(event: Event): string {
    return (event.target as HTMLInputElement).value;
  }

  protected submit(): void {
    const value = this.value().trim();
    if (value !== '') {
      this.prompt.answer(value);
    }
  }

  protected closeOnBackdrop(event: MouseEvent): void {
    if (event.target === this.dialog().nativeElement) {
      this.prompt.answer(null);
    }
  }
}
