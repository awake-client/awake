import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  computed,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { VariableInfo, VariableStore } from '@core/variable-store';

/** A run of text drawn behind the field, highlighted when it is a reference. */
interface Segment {
  text: string;
  state: 'text' | 'defined' | 'missing' | 'dynamic' | 'secret';
}

/** The `{{partial` being typed before the caret. */
interface PendingReference {
  start: number;
  query: string;
}

const PENDING_PATTERN = /\{\{\s*(\$?[A-Za-z0-9_]*)$/;
const MAX_SUGGESTIONS = 8;

/**
 * Single-line field aware of `{{variables}}`. References are colored by
 * state (defined, missing, secret, dynamic), typing `{{` opens a list of the
 * variables of the active environment, and the tooltip shows the resolved
 * text. The text itself is drawn by a layer behind a transparent input, so
 * native editing, selection and undo keep working.
 */
@Component({
  selector: 'app-variable-input',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    '[class.bare]': 'bare()',
    '[class.mono]': 'mono()',
  },
  template: `
    <div class="highlight" aria-hidden="true">
      <div class="highlight-text" [style.transform]="'translateX(' + -scrollLeft() + 'px)'">
        @for (segment of segments(); track $index) {
          <span [class]="segment.state">{{ segment.text }}</span>
        }
      </div>
    </div>
    <input
      #field
      type="text"
      [value]="value()"
      [placeholder]="placeholder()"
      [attr.aria-label]="ariaLabel() || null"
      [title]="tooltip()"
      spellcheck="false"
      autocomplete="off"
      role="combobox"
      [attr.aria-expanded]="isOpen()"
      (input)="handleInput()"
      (keydown)="handleKeydown($event)"
      (keyup)="handleCaretMove($event)"
      (click)="refreshSuggestions()"
      (scroll)="syncScroll()"
      (blur)="close()"
    />
    @if (isOpen()) {
      <ul
        class="suggestions"
        role="listbox"
        [style.top.px]="popupPosition().top"
        [style.left.px]="popupPosition().left"
        [style.min-width.px]="popupPosition().width"
      >
        @for (variable of matches(); track variable.name; let index = $index) {
          <li
            role="option"
            [class.active]="index === activeIndex()"
            [attr.aria-selected]="index === activeIndex()"
            (mousedown)="$event.preventDefault(); insert(variable)"
            (mouseenter)="activeIndex.set(index)"
          >
            <span class="suggestion-name" [class]="variable.source">{{ variable.name }}</span>
            <span class="suggestion-value">{{ variable.value }}</span>
            <span class="suggestion-source">{{ variable.source }}</span>
          </li>
        }
      </ul>
    }
  `,
  styleUrl: './variable-input.css',
})
export class VariableInput {
  private readonly store = inject(VariableStore);
  private readonly field = viewChild.required<ElementRef<HTMLInputElement>>('field');

  readonly value = input('');
  readonly placeholder = input('');
  readonly ariaLabel = input('');
  /** Drops the border, for fields embedded in a larger control. */
  readonly bare = input(false);
  readonly mono = input(true);
  readonly valueChange = output<string>();

  protected readonly scrollLeft = signal(0);
  protected readonly pending = signal<PendingReference | null>(null);
  protected readonly activeIndex = signal(0);
  protected readonly popupPosition = signal({ top: 0, left: 0, width: 0 });

  protected readonly segments = computed<Segment[]>(() => {
    const text = this.value();
    const segments: Segment[] = [];
    let cursor = 0;
    for (const reference of this.store.references(text)) {
      if (reference.start > cursor) {
        segments.push({ text: text.slice(cursor, reference.start), state: 'text' });
      }
      const source = reference.variable?.source;
      segments.push({
        text: text.slice(reference.start, reference.end),
        state: !reference.variable
          ? 'missing'
          : source === 'dynamic'
            ? 'dynamic'
            : source === 'secret'
              ? 'secret'
              : 'defined',
      });
      cursor = reference.end;
    }
    if (cursor < text.length) {
      segments.push({ text: text.slice(cursor), state: 'text' });
    }
    return segments;
  });

  /** The resolved text, or which variables are missing. */
  protected readonly tooltip = computed(() => {
    const references = this.store.references(this.value());
    if (references.length === 0) {
      return '';
    }
    const missing = references.filter((reference) => !reference.variable);
    if (missing.length > 0) {
      const names = [...new Set(missing.map((reference) => reference.name))];
      return `Undefined in the active environment: ${names.join(', ')}`;
    }
    return this.store.preview(this.value());
  });

  protected readonly matches = computed<VariableInfo[]>(() => {
    const pending = this.pending();
    if (!pending) {
      return [];
    }
    const query = pending.query.toLowerCase();
    const suggestions = this.store.suggestions();
    const startsWith = suggestions.filter((variable) =>
      variable.name.toLowerCase().startsWith(query),
    );
    const contains = suggestions.filter(
      (variable) =>
        !variable.name.toLowerCase().startsWith(query) &&
        variable.name.toLowerCase().includes(query),
    );
    return [...startsWith, ...contains].slice(0, MAX_SUGGESTIONS);
  });

  protected readonly isOpen = computed(() => this.matches().length > 0);

  focus(): void {
    this.field().nativeElement.focus();
  }

  protected handleInput(): void {
    this.valueChange.emit(this.field().nativeElement.value);
    this.refreshSuggestions();
    this.syncScroll();
  }

  protected handleKeydown(event: KeyboardEvent): void {
    if (!this.isOpen()) {
      return;
    }
    const count = this.matches().length;
    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault();
        this.activeIndex.update((index) => (index + 1) % count);
        break;
      case 'ArrowUp':
        event.preventDefault();
        this.activeIndex.update((index) => (index - 1 + count) % count);
        break;
      case 'Enter':
      case 'Tab':
        if (event.metaKey || event.ctrlKey) {
          return;
        }
        event.preventDefault();
        event.stopPropagation();
        this.insert(this.matches()[this.activeIndex()]);
        break;
      case 'Escape':
        event.preventDefault();
        event.stopPropagation();
        this.close();
        break;
    }
  }

  protected handleCaretMove(event: KeyboardEvent): void {
    if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) {
      this.refreshSuggestions();
    }
    this.syncScroll();
  }

  protected refreshSuggestions(): void {
    const element = this.field().nativeElement;
    const caret = element.selectionStart ?? element.value.length;
    const match = PENDING_PATTERN.exec(element.value.slice(0, caret));
    if (!match) {
      this.close();
      return;
    }
    const previous = this.pending();
    this.pending.set({ start: caret - match[0].length, query: match[1] });
    if (!previous || previous.query !== match[1]) {
      this.activeIndex.set(0);
    }
    const rect = element.getBoundingClientRect();
    this.popupPosition.set({
      top: rect.bottom + 4,
      left: rect.left,
      width: Math.min(rect.width, 420),
    });
  }

  protected close(): void {
    this.pending.set(null);
  }

  /** Replaces the `{{partial` before the caret with the full reference. */
  protected insert(variable: VariableInfo | undefined): void {
    const pending = this.pending();
    if (!variable || !pending) {
      return;
    }
    const element = this.field().nativeElement;
    const text = element.value;
    const caret = element.selectionStart ?? text.length;
    const after = text.slice(caret).replace(/^\s*\}\}/, '');
    const inserted = `{{${variable.name}}}`;
    const next = text.slice(0, pending.start) + inserted + after;

    element.value = next;
    const position = pending.start + inserted.length;
    element.setSelectionRange(position, position);
    this.valueChange.emit(next);
    this.close();
    this.syncScroll();
  }

  protected syncScroll(): void {
    this.scrollLeft.set(this.field().nativeElement.scrollLeft);
  }
}
