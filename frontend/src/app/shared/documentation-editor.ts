import { Component, computed, input, output, signal } from '@angular/core';
import { MarkdownView } from './markdown-view';

type DocumentationMode = 'write' | 'preview';

/**
 * Edits a Markdown documentation and previews it. Written documentation opens
 * as a preview; an empty one opens for writing. Double click the preview to
 * edit it.
 */
@Component({
  selector: 'app-documentation-editor',
  imports: [MarkdownView],
  template: `
    <div class="header">
      <span class="field-label">{{ label() }}</span>
      <div class="segmented" role="tablist" aria-label="Documentation mode">
        <button
          type="button"
          role="tab"
          [class.selected]="mode() === 'write'"
          [attr.aria-selected]="mode() === 'write'"
          (click)="chosenMode.set('write')"
        >
          Write
        </button>
        <button
          type="button"
          role="tab"
          [class.selected]="mode() === 'preview'"
          [attr.aria-selected]="mode() === 'preview'"
          (click)="chosenMode.set('preview')"
        >
          Preview
        </button>
      </div>
    </div>

    @if (mode() === 'write') {
      <textarea
        class="input editor"
        [value]="value()"
        (input)="write($event)"
        [placeholder]="placeholder()"
        aria-label="Documentation in Markdown"
      ></textarea>
      <span class="field-hint">Markdown: headings, lists, links, code, quotes and tables.</span>
    } @else {
      <div class="preview" title="Double click to edit" (dblclick)="chosenMode.set('write')">
        @if (value().trim()) {
          <app-markdown-view [source]="value()" />
        } @else {
          <p class="empty">Nothing documented yet.</p>
        }
      </div>
    }
  `,
  styleUrl: './documentation-editor.css',
})
export class DocumentationEditor {
  readonly value = input.required<string>();
  readonly label = input('Documentation');
  readonly placeholder = input('');
  readonly valueChange = output<string>();

  /** The mode picked by the person, if any; otherwise it follows the content. */
  protected readonly chosenMode = signal<DocumentationMode | null>(null);
  protected readonly mode = computed<DocumentationMode>(
    () => this.chosenMode() ?? (this.value().trim() ? 'preview' : 'write'),
  );

  protected write(event: Event): void {
    this.chosenMode.set('write');
    this.valueChange.emit((event.target as HTMLTextAreaElement).value);
  }
}
