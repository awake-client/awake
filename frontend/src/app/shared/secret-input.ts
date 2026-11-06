import { Component, computed, input, output, signal } from '@angular/core';
import { Icon } from './icon';
import { VariableInput } from './variable-input';

/** A value made only of references, such as `{{password}}` or `{{$uuid}}`. */
const REFERENCES_ONLY = /^\s*(\{\{\s*\$?[A-Za-z_][A-Za-z0-9_]*\s*\}\}\s*)+$/;

/**
 * Field for a secret value. It is masked by default and an eye button shows
 * it, with the `{{variables}}` highlighting of the other fields. A value
 * made only of variable references is not a secret itself, so it starts
 * visible.
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

  protected readonly visible = computed(
    () => this.revealed() ?? REFERENCES_ONLY.test(this.value()),
  );

  protected toggle(): void {
    this.revealed.set(!this.visible());
  }
}
