import {
  Component,
  DestroyRef,
  ElementRef,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { Icon } from './icon';

/** One choice of a select. */
export interface SelectOption {
  value: string;
  label: string;
  /** Secondary text, such as the raw value written to the file. */
  hint?: string;
}

let nextId = 0;

/**
 * A dropdown drawn by the application instead of the native select, so it
 * follows the theme. It is keyboard accessible: arrows move, Enter or Space
 * choose, Escape closes. A value outside the options is shown as invalid.
 */
@Component({
  selector: 'app-select',
  imports: [Icon],
  host: {
    '(document:mousedown)': 'closeOutside($event)',
    '(window:resize)': 'close()',
  },
  template: `
    <button
      #trigger
      type="button"
      class="trigger"
      [class.open]="isOpen()"
      [class.invalid]="!selected() && value() !== ''"
      role="combobox"
      aria-haspopup="listbox"
      [attr.aria-expanded]="isOpen()"
      [attr.aria-controls]="listId"
      [attr.aria-label]="ariaLabel() || null"
      [attr.aria-activedescendant]="isOpen() ? optionId(activeIndex()) : null"
      (click)="toggle()"
      (keydown)="onKeydown($event)"
    >
      @if (selected(); as option) {
        <span class="label">{{ option.label }}</span>
        @if (option.hint) {
          <span class="hint">{{ option.hint }}</span>
        }
      } @else if (value() !== '') {
        <span class="label">{{ value() }}</span>
        <span class="hint">not accepted</span>
      } @else {
        <span class="label placeholder">{{ placeholder() }}</span>
      }
      <app-icon class="chevron" name="chevron-down" [size]="13" />
    </button>
    @if (isOpen()) {
      <ul
        class="list"
        role="listbox"
        [id]="listId"
        [style.top.px]="position().top"
        [style.left.px]="position().left"
        [style.min-width.px]="position().width"
      >
        @for (option of options(); track option.value; let index = $index) {
          <li
            role="option"
            [id]="optionId(index)"
            class="option"
            [class.active]="index === activeIndex()"
            [class.selected]="option.value === value()"
            [attr.aria-selected]="option.value === value()"
            (mouseenter)="activeIndex.set(index)"
            (mousedown)="$event.preventDefault()"
            (click)="choose(option)"
          >
            <app-icon class="check" name="check" [size]="13" />
            <span class="label">{{ option.label }}</span>
            @if (option.hint) {
              <span class="hint">{{ option.hint }}</span>
            }
          </li>
        }
      </ul>
    }
  `,
  styles: `
    :host {
      display: block;
      min-width: 0;
    }
    .trigger {
      display: flex;
      align-items: center;
      gap: 8px;
      width: 100%;
      min-height: 33px;
      padding: 0 9px;
      border: 1px solid var(--color-border);
      border-radius: var(--radius-sm);
      background: var(--color-background);
      color: var(--color-text-strong);
      font-size: var(--text-base);
      text-align: left;
      cursor: pointer;
    }
    .trigger:hover {
      border-color: var(--color-border-strong);
    }
    .trigger:focus-visible,
    .trigger.open {
      outline: none;
      border-color: var(--color-accent);
    }
    .trigger.invalid {
      border-color: var(--color-danger);
    }
    .trigger.invalid .hint {
      color: var(--color-danger);
    }
    .label {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .placeholder {
      color: var(--color-text-subtle);
    }
    .hint {
      font-family: var(--font-mono);
      font-size: var(--text-sm);
      color: var(--color-text-subtle);
    }
    .chevron {
      margin-left: auto;
      flex: 0 0 auto;
      color: var(--color-text-muted);
      transition: transform 120ms ease;
    }
    .trigger.open .chevron {
      transform: rotate(180deg);
    }
    .list {
      position: fixed;
      z-index: 1000;
      max-height: 260px;
      margin: 0;
      padding: 5px;
      overflow: auto;
      list-style: none;
      border-radius: var(--radius-lg);
      background: var(--color-surface-raised);
      box-shadow: var(--shadow-raised);
    }
    .option {
      display: flex;
      align-items: center;
      gap: 8px;
      height: 30px;
      padding: 0 10px 0 8px;
      border-radius: var(--radius-md);
      color: var(--color-text);
      font-size: var(--text-base);
      cursor: pointer;
    }
    .option.active {
      background: var(--color-active);
    }
    .option .check {
      visibility: hidden;
      color: var(--color-accent);
    }
    .option.selected {
      color: var(--color-text-strong);
      font-weight: 500;
    }
    .option.selected .check {
      visibility: visible;
    }
    .option .hint {
      margin-left: auto;
      padding-left: 16px;
    }
  `,
})
export class Select {
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly options = input.required<SelectOption[]>();
  readonly value = input('');
  readonly placeholder = input('Choose…');
  readonly ariaLabel = input('');
  readonly valueChange = output<string>();

  protected readonly listId = `app-select-${nextId++}`;
  protected readonly isOpen = signal(false);
  protected readonly activeIndex = signal(0);
  protected readonly position = signal({ top: 0, left: 0, width: 0 });
  protected readonly selected = computed(
    () => this.options().find((option) => option.value === this.value()) ?? null,
  );

  /** Closes the list when anything but the list itself scrolls. */
  private readonly onScroll = (event: Event): void => {
    if (!this.host.nativeElement.contains(event.target as Node)) {
      this.close();
    }
  };

  constructor() {
    inject(DestroyRef).onDestroy(() => this.close());
  }

  protected optionId(index: number): string {
    return `${this.listId}-${index}`;
  }

  protected toggle(): void {
    if (this.isOpen()) {
      this.close();
    } else {
      this.open();
    }
  }

  protected close(): void {
    this.isOpen.set(false);
    window.removeEventListener('scroll', this.onScroll, true);
  }

  protected choose(option: SelectOption): void {
    this.close();
    if (option.value !== this.value()) {
      this.valueChange.emit(option.value);
    }
  }

  protected onKeydown(event: KeyboardEvent): void {
    const count = this.options().length;
    switch (event.key) {
      case 'ArrowDown':
      case 'ArrowUp': {
        event.preventDefault();
        if (!this.isOpen()) {
          this.open();
          return;
        }
        const step = event.key === 'ArrowDown' ? 1 : -1;
        this.activeIndex.update((index) => (index + step + count) % count);
        return;
      }
      case 'Home':
      case 'End':
        if (this.isOpen()) {
          event.preventDefault();
          this.activeIndex.set(event.key === 'Home' ? 0 : count - 1);
        }
        return;
      case 'Enter':
      case ' ':
        event.preventDefault();
        if (this.isOpen()) {
          const option = this.options()[this.activeIndex()];
          if (option) {
            this.choose(option);
          }
        } else {
          this.open();
        }
        return;
      case 'Escape':
        if (this.isOpen()) {
          event.preventDefault();
          event.stopPropagation();
          this.close();
        }
        return;
      case 'Tab':
        this.close();
        return;
    }
  }

  protected closeOutside(event: MouseEvent): void {
    if (this.isOpen() && !this.host.nativeElement.contains(event.target as Node)) {
      this.close();
    }
  }

  private open(): void {
    const trigger = this.host.nativeElement.querySelector('.trigger');
    if (!trigger) {
      return;
    }
    const rect = trigger.getBoundingClientRect();
    const listHeight = Math.min(260, this.options().length * 30 + 10);
    const below = rect.bottom + 4;
    const top =
      below + listHeight > window.innerHeight && rect.top > listHeight + 4
        ? rect.top - listHeight - 4
        : below;
    this.position.set({ top, left: rect.left, width: rect.width });
    const index = this.options().findIndex((option) => option.value === this.value());
    this.activeIndex.set(Math.max(0, index));
    this.isOpen.set(true);
    window.addEventListener('scroll', this.onScroll, true);
  }
}
