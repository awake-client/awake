import { Component, ElementRef, effect, inject, signal, viewChild } from '@angular/core';
import { EnvironmentEditing } from '@core/environment-editing';
import { Icon } from '@shared/icon';
import { EnvironmentEditor } from './environment-editor';

/**
 * Edits an environment over the current page, opened from the pencil of the
 * environment picker. When a collection refines a global environment of the
 * same name, tabs switch between the two files.
 */
@Component({
  selector: 'app-environment-dialog',
  imports: [Icon, EnvironmentEditor],
  template: `
    <dialog
      #dialog
      class="environment-dialog"
      aria-labelledby="environment-dialog-title"
      (cancel)="$event.preventDefault(); close()"
      (click)="closeOnBackdrop($event)"
    >
      @if (editing.request(); as request) {
        <header>
          <span class="badge" aria-hidden="true">
            <app-icon name="pencil" [size]="16" />
          </span>
          <h2 id="environment-dialog-title">Edit environment</h2>
          <button type="button" class="close" aria-label="Close" (click)="close()">
            <app-icon name="close" [size]="15" />
          </button>
        </header>

        @if (scopes().length > 1) {
          <nav class="scopes" aria-label="Scope">
            @for (scope of scopes(); track scope) {
              <button
                type="button"
                [class.active]="scope === activeScope()"
                [attr.aria-pressed]="scope === activeScope()"
                (click)="activeScope.set(scope)"
              >
                {{ scopeLabel(scope) }}
              </button>
            }
          </nav>
          <p class="scope-hint">
            The collection environment refines the global one: variables it doesn't define are
            inherited.
          </p>
        }

        <div class="content">
          <app-environment-editor
            [scope]="activeScope()"
            [fileName]="fileName()"
            (cloned)="showClone($event)"
            (deleted)="close()"
            (renamed)="showClone($event)"
          />
        </div>
      }
    </dialog>
  `,
  styles: `
    .environment-dialog {
      width: min(760px, calc(100vw - 48px));
      max-height: calc(100vh - 64px);
      padding: 0;
      border: none;
      border-radius: var(--radius-xl);
      background: var(--color-surface);
      color: var(--color-text);
      box-shadow: var(--shadow-raised);
    }
    .environment-dialog:focus,
    .environment-dialog:focus-visible {
      outline: none;
    }
    .environment-dialog[open] {
      display: flex;
      flex-direction: column;
    }
    .environment-dialog::backdrop {
      background: var(--color-backdrop);
      backdrop-filter: blur(3px);
    }
    header {
      display: flex;
      align-items: center;
      gap: 12px;
      padding: 18px 22px 12px;
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
    h2 {
      flex: 1;
      margin: 0;
      font-size: var(--text-md);
      font-weight: 650;
      color: var(--color-text-strong);
    }
    .close {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 28px;
      height: 28px;
      padding: 0;
      border: none;
      border-radius: var(--radius-sm);
      background: transparent;
      color: var(--color-text-muted);
      cursor: pointer;
    }
    .close:hover {
      background: var(--color-hover);
      color: var(--color-text-strong);
    }
    .scopes {
      display: flex;
      gap: 4px;
      margin: 0 22px;
      padding: 3px;
      border-radius: var(--radius-md);
      background: var(--color-active);
      align-self: flex-start;
    }
    .scopes button {
      padding: 4px 12px;
      border: none;
      border-radius: var(--radius-sm);
      background: transparent;
      color: var(--color-text-muted);
      font-size: var(--text-sm);
      cursor: pointer;
    }
    .scopes button.active {
      background: var(--color-surface);
      color: var(--color-text-strong);
      box-shadow: var(--shadow-card);
    }
    .scope-hint {
      margin: 8px 22px 0;
      font-size: var(--text-sm);
      color: var(--color-text-subtle);
    }
    /* Sized by its content and shrunk, with a scrollbar, only when the
       dialog reaches its maximum height: a zero flex basis collapses it. */
    .content {
      flex: 0 1 auto;
      min-height: 0;
      overflow-y: auto;
      padding: 14px 22px 20px;
    }
  `,
})
export class EnvironmentDialog {
  protected readonly editing = inject(EnvironmentEditing);
  private readonly dialog = viewChild.required<ElementRef<HTMLDialogElement>>('dialog');

  protected readonly fileName = signal('');
  protected readonly scopes = signal<string[]>([]);
  protected readonly activeScope = signal('');

  constructor() {
    effect(() => {
      const request = this.editing.request();
      const dialog = this.dialog().nativeElement;
      if (request) {
        this.fileName.set(request.fileName);
        this.scopes.set(request.scopes);
        this.activeScope.set(request.scopes[0] ?? '');
        if (!dialog.open) {
          dialog.showModal();
        }
      } else if (dialog.open) {
        dialog.close();
      }
    });
  }

  protected scopeLabel(scope: string): string {
    if (scope === '') {
      return 'Global';
    }
    const name = scope.split('/')[1] ?? scope;
    return scope.startsWith('local/') ? `${name} (private collection)` : `${name} collection`;
  }

  /**
   * A clone, or a renamed file, only exists in the scope being edited, so
   * the dialog follows it there.
   */
  protected showClone(fileName: string): void {
    this.scopes.set([this.activeScope()]);
    this.fileName.set(fileName);
  }

  protected close(): void {
    this.editing.close();
  }

  protected closeOnBackdrop(event: MouseEvent): void {
    if (event.target === this.dialog().nativeElement) {
      this.close();
    }
  }
}
