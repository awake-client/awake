import { Component, input, output } from '@angular/core';

/** Color names accepted by the backend, in display order. */
export const COLOR_NAMES = [
  'orange',
  'amber',
  'green',
  'teal',
  'blue',
  'violet',
  'pink',
  'red',
  'gray',
];

/** A row of swatches choosing one of the named colors, or none. */
@Component({
  selector: 'app-color-picker',
  template: `
    <div class="swatches" role="radiogroup" [attr.aria-label]="label()">
      <button
        type="button"
        role="radio"
        class="swatch none"
        title="No color"
        aria-label="No color"
        [class.selected]="!value()"
        [attr.aria-checked]="!value()"
        (click)="valueChange.emit('')"
      ></button>
      @for (color of colors; track color) {
        <button
          type="button"
          role="radio"
          class="swatch"
          [attr.data-color]="color"
          [title]="color"
          [attr.aria-label]="color"
          [class.selected]="value() === color"
          [attr.aria-checked]="value() === color"
          (click)="valueChange.emit(color)"
        ></button>
      }
    </div>
  `,
  styles: `
    .swatches {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
    }
    .swatch {
      width: 20px;
      height: 20px;
      padding: 0;
      border: 2px solid transparent;
      border-radius: 50%;
      background: var(--entity-color);
      background-clip: content-box;
      outline: none;
      cursor: pointer;
      box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.08);
    }
    .swatch.selected {
      border-color: var(--entity-color, var(--color-text-strong));
      padding: 2px;
    }
    .swatch.none {
      background:
        linear-gradient(
          135deg,
          transparent 45%,
          var(--color-text-subtle) 45%,
          var(--color-text-subtle) 55%,
          transparent 55%
        ),
        var(--color-field);
      border: 1px solid var(--color-border-strong);
    }
    .swatch.none.selected {
      border: 2px solid var(--color-text-strong);
    }
    .swatch:focus-visible {
      outline: 2px solid var(--color-accent-border);
      outline-offset: 2px;
    }
  `,
})
export class ColorPicker {
  readonly value = input('');
  readonly label = input('Color');
  readonly valueChange = output<string>();

  protected readonly colors = COLOR_NAMES;
}
