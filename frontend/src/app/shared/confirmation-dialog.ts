import { Component, ElementRef, effect, inject, viewChild } from '@angular/core';
import { Confirmation } from '@core/confirmation';
import { Icon } from './icon';

/** Renders the pending confirmation as a modal dialog. */
@Component({
  selector: 'app-confirmation-dialog',
  imports: [Icon],
  template: `
    <dialog
      #dialog
      class="confirmation"
      (cancel)="$event.preventDefault(); confirmation.answer(false)"
      (click)="closeOnBackdrop($event)"
    >
      @if (confirmation.pending(); as request) {
        <div class="body">
          <span class="badge" [class.danger]="request.danger" aria-hidden="true">
            <app-icon [name]="request.danger ? 'trash' : 'alert'" [size]="16" />
          </span>
          <div class="text">
            <h2>{{ request.title }}</h2>
            <p>{{ request.message }}</p>
          </div>
        </div>
        <footer>
          <button type="button" class="btn btn-ghost" (click)="confirmation.answer(false)">
            Cancel
          </button>
          <button
            #confirmButton
            type="button"
            class="btn"
            [class.btn-primary]="!request.danger"
            [class.btn-danger-solid]="request.danger"
            (click)="confirmation.answer(true)"
          >
            {{ request.confirmLabel }}
          </button>
        </footer>
      }
    </dialog>
  `,
  styles: `
    .confirmation {
      width: min(440px, calc(100vw - 48px));
      padding: 0;
      border: none;
      border-radius: var(--radius-xl);
      background: var(--color-surface);
      color: var(--color-text);
      box-shadow: var(--shadow-raised);
    }
    .confirmation::backdrop {
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
    .badge.danger {
      background: color-mix(in srgb, var(--color-danger) 14%, transparent);
      color: var(--color-danger);
    }
    h2 {
      margin: 4px 0 6px;
      font-size: var(--text-md);
      font-weight: 650;
      color: var(--color-text-strong);
    }
    p {
      margin: 0;
      line-height: 1.55;
      color: var(--color-text-muted);
      overflow-wrap: anywhere;
    }
    footer {
      display: flex;
      justify-content: flex-end;
      gap: 8px;
      padding: 12px 22px;
      border-top: 1px solid var(--color-border);
      background: color-mix(in srgb, var(--color-window) 35%, var(--color-surface));
    }
    .btn-danger-solid {
      border-color: transparent;
      background: var(--color-danger);
      color: #ffffff;
      font-weight: 600;
    }
    .btn-danger-solid:hover:not(:disabled) {
      border-color: transparent;
      background: var(--color-danger);
      color: #ffffff;
      filter: brightness(1.08);
    }
  `,
})
export class ConfirmationDialog {
  protected readonly confirmation = inject(Confirmation);
  private readonly dialog = viewChild.required<ElementRef<HTMLDialogElement>>('dialog');
  private readonly confirmButton = viewChild<ElementRef<HTMLButtonElement>>('confirmButton');

  constructor() {
    effect(() => {
      const dialog = this.dialog().nativeElement;
      if (this.confirmation.pending()) {
        if (!dialog.open) {
          dialog.showModal();
        }
        setTimeout(() => this.confirmButton()?.nativeElement.focus());
      } else if (dialog.open) {
        dialog.close();
      }
    });
  }

  protected closeOnBackdrop(event: MouseEvent): void {
    if (event.target === this.dialog().nativeElement) {
      this.confirmation.answer(false);
    }
  }
}
