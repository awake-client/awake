import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  ElementRef,
  afterRenderEffect,
  computed,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { VariableInfo, VariableStore } from '@core/variable-store';
import { snapUrlLines, URL_FIELD } from './url-field';

/** A run of text drawn behind the field, highlighted when it is a reference. */
interface Segment {
  text: string;
  state:
    | 'text'
    | 'defined'
    | 'missing'
    | 'dynamic'
    | 'secret'
    | 'path-parameter'
    | 'query-name'
    | 'query-value'
    | 'query-separator';
}

/** Where a URL reader stands, carried across the variables of the URL. */
interface UrlPosition {
  inQuery: boolean;
  inValue: boolean;
}

const PATH_PARAMETER_START = /\/:[A-Za-z_]/;
const PATH_PARAMETER_NAME = /^:[A-Za-z_][A-Za-z0-9_]*/;
const QUERY_PLACEHOLDER_VALUE = /^(:[A-Za-z_][A-Za-z0-9_]*)(?=[&#]|$)/;

/**
 * Splits plain URL text into path parameters and query parts. Variables are
 * cut out beforehand, so position carries whether the text is in the query
 * and in a value.
 */
export function urlSegments(text: string, position: UrlPosition): Segment[] {
  const segments: Segment[] = [];
  const push = (part: string, state: Segment['state']) => {
    if (part === '') {
      return;
    }
    const last = segments[segments.length - 1];
    if (last?.state === state) {
      last.text += part;
    } else {
      segments.push({ text: part, state });
    }
  };

  let index = 0;
  while (index < text.length) {
    const character = text[index];
    if (!position.inQuery) {
      if (character === '?') {
        push(character, 'query-separator');
        position.inQuery = true;
        index += 1;
        continue;
      }
      if (character === '/' && PATH_PARAMETER_START.test(text.slice(index, index + 3))) {
        push(character, 'text');
        const name = PATH_PARAMETER_NAME.exec(text.slice(index + 1))![0];
        push(name, 'path-parameter');
        index += 1 + name.length;
        continue;
      }
      push(character, 'text');
    } else if (character === '&' || character === '#') {
      push(character, 'query-separator');
      position.inValue = false;
    } else if (character === '=' && !position.inValue) {
      push(character, 'query-separator');
      position.inValue = true;
      // A whole value written :name is a placeholder, like a path segment.
      const placeholder = QUERY_PLACEHOLDER_VALUE.exec(text.slice(index + 1));
      if (placeholder) {
        push(placeholder[1], 'path-parameter');
        index += 1 + placeholder[1].length;
        continue;
      }
    } else {
      push(character, position.inValue ? 'query-value' : 'query-name');
    }
    index += 1;
  }
  return segments;
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
 *
 * A multiline field wraps a long value instead of scrolling it sideways and
 * grows up to its maximum number of lines, then scrolls vertically.
 */
@Component({
  selector: 'app-variable-input',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    '[class.bare]': 'bare()',
    '[class.mono]': 'mono()',
    '[class.masked]': 'masked()',
    '[class.multiline]': 'multiline()',
  },
  template: `
    <div class="highlight" aria-hidden="true">
      <div class="highlight-clip">
        <div
          class="highlight-text"
          [style.transform]="'translate(' + -scrollLeft() + 'px, ' + -scrollTop() + 'px)'"
        >
          @for (segment of segments(); track $index) {
            <span [class]="segment.state">{{ segment.text }}</span>
          }
        </div>
      </div>
    </div>
    @if (multiline()) {
      <textarea
        #multiLineField
        rows="1"
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
        (focus)="followScroll()"
        (blur)="close(); stopFollowingScroll()"
      ></textarea>
    } @else {
      <input
        #singleLineField
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
        (focus)="followScroll()"
        (blur)="close(); stopFollowingScroll()"
      />
    }
    @if (resizable()) {
      <span
        class="resize-grip"
        role="separator"
        aria-orientation="horizontal"
        aria-label="Resize the URL field"
        title="Drag to resize, double click to reset"
        (pointerdown)="startResize($event)"
        (dblclick)="resetHeight.emit()"
      ></span>
    }
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
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly singleLineField = viewChild<ElementRef<HTMLInputElement>>('singleLineField');
  private readonly multiLineField = viewChild<ElementRef<HTMLTextAreaElement>>('multiLineField');
  private lastWidth = 0;

  readonly value = input('');
  readonly placeholder = input('');
  readonly ariaLabel = input('');
  /** Drops the border, for fields embedded in a larger control. */
  readonly bare = input(false);
  readonly mono = input(true);
  /** Hides the characters, for secrets. */
  readonly masked = input(false);
  /** Highlights path parameters and the query, for a request URL. */
  readonly url = input(false);
  /** Wraps long values and grows vertically instead of scrolling sideways. */
  readonly multiline = input(false);
  /** Number of lines shown before a multiline field scrolls. */
  readonly maxLines = input(3);
  /** A height the user chose, in pixels; null fits the content. */
  readonly height = input<number | null>(null);
  /** Shows a grip that resizes the field vertically. */
  readonly resizable = input(false);
  readonly valueChange = output<string>();
  /** New height while the user drags the grip. */
  readonly heightChange = output<number>();
  /** Asked for the content height again, on a double click of the grip. */
  readonly resetHeight = output<void>();

  protected readonly scrollLeft = signal(0);
  protected readonly scrollTop = signal(0);
  private scrollFrame: number | null = null;

  constructor() {
    // The drawing layer follows the field's scroll and the field grows with
    // its content. A wrapped value changes height when the pane is resized.
    afterRenderEffect(() => {
      this.value();
      this.height();
      if (this.multiline()) {
        this.resize();
      }
    });

    const observer =
      typeof ResizeObserver === 'undefined'
        ? null
        : new ResizeObserver((entries) => {
            const width = entries[0]?.contentRect.width ?? 0;
            if (!this.multiline() || width === this.lastWidth) {
              return;
            }
            this.lastWidth = width;
            this.resize();
          });
    observer?.observe(this.host.nativeElement);

    inject(DestroyRef).onDestroy(() => {
      observer?.disconnect();
      if (this.scrollFrame !== null) {
        cancelAnimationFrame(this.scrollFrame);
      }
    });
  }

  private field(): HTMLInputElement | HTMLTextAreaElement {
    const multiline = this.multiLineField()?.nativeElement;
    const singleLine = this.singleLineField()?.nativeElement;
    if (this.multiline() && multiline) {
      return multiline;
    }
    if (singleLine) {
      return singleLine;
    }
    return multiline ?? singleLine!;
  }
  protected readonly pending = signal<PendingReference | null>(null);
  protected readonly activeIndex = signal(0);
  protected readonly popupPosition = signal({ top: 0, left: 0, width: 0 });

  protected readonly segments = computed<Segment[]>(() => {
    const text = this.value();
    const segments: Segment[] = [];
    const position: UrlPosition = { inQuery: false, inValue: false };
    const pushText = (part: string) => {
      if (this.url()) {
        segments.push(...urlSegments(part, position));
      } else {
        segments.push({ text: part, state: 'text' });
      }
    };
    let cursor = 0;
    for (const reference of this.store.references(text)) {
      if (reference.start > cursor) {
        pushText(text.slice(cursor, reference.start));
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
      pushText(text.slice(cursor));
    }
    return segments;
  });

  /** The resolved text, or which variables are missing. */
  protected readonly tooltip = computed(() => {
    if (this.masked()) {
      return '';
    }
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
    this.field().focus();
  }

  protected handleInput(): void {
    this.valueChange.emit(this.field().value);
    this.refreshSuggestions();
    this.resize();
    this.syncScroll();
  }

  protected handleKeydown(event: KeyboardEvent): void {
    if (!this.isOpen()) {
      // A multiline field wraps, it never stores a line break.
      if (this.multiline() && event.key === 'Enter' && !event.metaKey && !event.ctrlKey) {
        event.preventDefault();
      }
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
    const element = this.field();
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
    const element = this.field();
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
    this.resize();
    this.syncScroll();
  }

  /** Grows the field up to its maximum number of lines, then scrolls. A
   * height chosen by the user wins over the content. */
  private resize(): void {
    const field = this.multiLineField()?.nativeElement;
    if (!field || !this.multiline()) {
      return;
    }

    const chosen = this.height();
    if (chosen !== null) {
      field.style.height = `${chosen}px`;
      field.style.overflowY = field.scrollHeight > chosen ? 'auto' : 'hidden';
      return;
    }

    // scrollHeight never reports less than the current box, so the height is
    // reset first: a field widened back would otherwise keep its old height.
    const scrollTop = field.scrollTop;
    const maximum = URL_FIELD.lineHeight * this.maxLines() + 2 * URL_FIELD.padding;
    field.style.height = 'auto';
    const scrollHeight = field.scrollHeight;
    if (scrollHeight === 0) {
      // Hidden or unmeasurable: let the stylesheet set the height.
      field.style.height = '';
      field.style.overflowY = 'hidden';
      return;
    }
    field.style.height = `${Math.min(scrollHeight, maximum)}px`;
    field.style.overflowY = scrollHeight > maximum ? 'auto' : 'hidden';
    field.scrollTop = scrollTop;
  }

  /** Turns a drag of the grip into a height, relative to where it started.
   * Heights snap to whole lines, so the field never cuts a line in half. */
  protected startResize(event: PointerEvent): void {
    if (event.button !== 0) {
      return;
    }
    event.preventDefault();

    const grip = event.currentTarget as HTMLElement;
    const startY = event.clientY;
    const startHeight = this.host.nativeElement.getBoundingClientRect().height;
    grip.setPointerCapture(event.pointerId);
    document.body.style.cursor = 'row-resize';

    // Heights snap to whole lines, and only a line change is reported.
    let lastHeight = snapUrlLines(startHeight);
    const move = (moveEvent: PointerEvent) => {
      const height = snapUrlLines(startHeight + (moveEvent.clientY - startY));
      if (height !== lastHeight) {
        lastHeight = height;
        this.heightChange.emit(height);
      }
    };
    const stop = () => {
      document.body.style.cursor = '';
      grip.removeEventListener('pointermove', move);
      grip.removeEventListener('pointerup', stop);
      grip.removeEventListener('pointercancel', stop);
    };
    grip.addEventListener('pointermove', move);
    grip.addEventListener('pointerup', stop);
    grip.addEventListener('pointercancel', stop);
  }

  protected syncScroll(): void {
    const field = this.field();
    this.scrollLeft.set(field.scrollLeft);
    this.scrollTop.set(field.scrollTop);
  }

  /**
   * Keeps the drawn text under the field's text while it has the focus.
   * WebKit scrolls an input without a scroll event when a selection is
   * dragged past its edge, so the offset is read on every frame instead.
   */
  protected followScroll(): void {
    this.stopFollowingScroll();
    const follow = () => {
      this.syncScroll();
      this.scrollFrame = requestAnimationFrame(follow);
    };
    this.scrollFrame = requestAnimationFrame(follow);
  }

  protected stopFollowingScroll(): void {
    if (this.scrollFrame !== null) {
      cancelAnimationFrame(this.scrollFrame);
      this.scrollFrame = null;
    }
    this.syncScroll();
  }
}
