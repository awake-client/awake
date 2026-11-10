import { Component, computed, input, output, signal } from '@angular/core';
import { Icon } from './icon';
import { VariableInput } from './variable-input';

/**
 * True when the text holds a variable reference, complete (`{{password}}`) or
 * still being typed (`{{pass`). Such a value is not a secret itself: it tells
 * where to read the secret, so it stays visible while it is edited.
 */
export function referencesVariables(value: string): boolean {
  return value.includes('{{');
}

/**
 * Field for a secret value. It is masked by default and an eye button shows
 * it, with the `{{variables}}` highlighting of the other fields. A value
 * referencing a variable is not a secret itself, so it is always visible.
 */
@Component({
  selector: 'app-secret-input',
  imports: [Icon, VariableInput],
  template: `
    <app-variable-input
      class="field"
      [value]="value()"
      [placeholder]="placeholder()"
      [ariaLabel]="ariaLabel()"
      [masked]="!visible()"
      (valueChange)="valueChange.emit($event)"
    />
    @if (!refersToVariable()) {
      <button
        type="button"
        class="reveal"
        [title]="visible() ? 'Hide' : 'Show'"
        [attr.aria-label]="visible() ? 'Hide the value' : 'Show the value'"
        [attr.aria-pressed]="visible()"
        (click)="toggle()"
      >
        <app-icon [name]="visible() ? 'eye-off' : 'eye'" [size]="15" />
      </button>
    }
  `,
  styles: `
    :host {
      position: relative;
      display: block;
      min-width: 0;
    }
    .field {
      width: 100%;
      --variable-padding-end: 36px;
    }
    .reveal {
      position: absolute;
      top: 50%;
      right: 4px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 26px;
      height: 24px;
      padding: 0;
      border: none;
      border-radius: var(--radius-sm);
      background: transparent;
      color: var(--color-text-subtle);
      transform: translateY(-50%);
      cursor: pointer;
      z-index: 1;
    }
    .reveal:hover {
      background: var(--color-active);
      color: var(--color-text-strong);
    }
  `,
})
export class SecretInput {
  readonly value = input('');
  readonly placeholder = input('');
  readonly ariaLabel = input('');
  readonly valueChange = output<string>();

  /** Explicit choice of the user; `null` means automatic. */
  private readonly revealed = signal<boolean | null>(null);

  /** A reference is never hidden, even after the user hid a literal secret. */
  protected readonly refersToVariable = computed(() => referencesVariables(this.value()));

  protected readonly visible = computed(() => this.refersToVariable() || this.revealed() === true);

  protected toggle(): void {
    this.revealed.set(!this.visible());
  }
}
