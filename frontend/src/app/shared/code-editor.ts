import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  ElementRef,
  afterNextRender,
  effect,
  inject,
  input,
  output,
} from '@angular/core';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { json } from '@codemirror/lang-json';
import { xml } from '@codemirror/lang-xml';
import {
  HighlightStyle,
  bracketMatching,
  foldGutter,
  foldKeymap,
  indentOnInput,
  syntaxHighlighting,
} from '@codemirror/language';
import { Compartment, EditorState, Extension } from '@codemirror/state';
import {
  EditorView,
  highlightActiveLine,
  highlightActiveLineGutter,
  keymap,
  lineNumbers,
  placeholder as placeholderExtension,
} from '@codemirror/view';
import { tags } from '@lezer/highlight';

import { ScriptKind, scriptLanguage } from './script-language';

/** Languages the editor highlights. */
export type CodeLanguage = 'json' | 'xml' | 'text' | 'javascript';

/**
 * Syntax colors, taken from the application tokens so they follow the light
 * and dark themes.
 */
const highlightStyle = HighlightStyle.define([
  { tag: tags.propertyName, color: 'var(--method-put)' },
  { tag: [tags.string, tags.special(tags.string)], color: 'var(--color-success)' },
  { tag: tags.number, color: 'var(--color-warning)' },
  { tag: [tags.bool, tags.null, tags.atom], color: 'var(--method-patch)' },
  { tag: [tags.tagName], color: 'var(--method-put)' },
  { tag: [tags.attributeName], color: 'var(--method-patch)' },
  { tag: [tags.attributeValue], color: 'var(--color-success)' },
  { tag: [tags.comment], color: 'var(--color-text-subtle)', fontStyle: 'italic' },
  { tag: [tags.punctuation, tags.bracket, tags.separator], color: 'var(--color-text-muted)' },
  { tag: tags.invalid, color: 'var(--color-danger)' },
  {
    tag: [tags.keyword, tags.controlKeyword, tags.definitionKeyword, tags.moduleKeyword],
    color: 'var(--method-delete)',
  },
  { tag: [tags.operatorKeyword, tags.self], color: 'var(--method-delete)' },
  {
    tag: [tags.function(tags.variableName), tags.function(tags.propertyName)],
    color: 'var(--method-put)',
  },
  { tag: tags.definition(tags.variableName), color: 'var(--color-text-strong)' },
  { tag: [tags.regexp, tags.escape], color: 'var(--method-patch)' },
  { tag: tags.operator, color: 'var(--color-text-muted)' },
]);

const theme = EditorView.theme({
  '&': {
    height: '100%',
    fontSize: 'var(--text-sm)',
    color: 'var(--color-text-strong)',
    backgroundColor: 'transparent',
  },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': {
    fontFamily: 'var(--font-mono)',
    lineHeight: '1.6',
  },
  '.cm-content': { padding: '10px 0', caretColor: 'var(--color-accent)' },
  '.cm-line': { padding: '0 12px 0 6px' },
  '.cm-gutters': {
    backgroundColor: 'transparent',
    color: 'var(--color-text-subtle)',
    border: 'none',
  },
  '.cm-lineNumbers .cm-gutterElement': { padding: '0 6px 0 12px', minWidth: '28px' },
  '.cm-foldGutter .cm-gutterElement': { padding: '0 4px', cursor: 'pointer' },
  '.cm-activeLine': { backgroundColor: 'var(--color-hover)' },
  '.cm-activeLineGutter': {
    backgroundColor: 'transparent',
    color: 'var(--color-text-muted)',
  },
  '&.cm-focused .cm-cursor': { borderLeftColor: 'var(--color-accent)' },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': {
    backgroundColor: 'var(--color-selection) !important',
  },
  '.cm-matchingBracket': {
    backgroundColor: 'var(--color-accent-soft)',
    outline: '1px solid var(--color-accent-border)',
  },
  '.cm-placeholder': { color: 'var(--color-text-subtle)' },
  '.cm-tooltip': {
    backgroundColor: 'var(--color-surface-raised)',
    color: 'var(--color-text)',
    border: '1px solid var(--color-border-strong)',
    borderRadius: 'var(--radius-md)',
    overflow: 'hidden',
  },
  '.cm-tooltip.cm-tooltip-autocomplete > ul': {
    fontFamily: 'var(--font-mono)',
    fontSize: 'var(--text-sm)',
    maxHeight: '240px',
  },
  '.cm-tooltip.cm-tooltip-autocomplete > ul > li': { padding: '2px 8px' },
  '.cm-tooltip.cm-tooltip-autocomplete > ul > li[aria-selected]': {
    backgroundColor: 'var(--color-accent-soft)',
    color: 'var(--color-text-strong)',
  },
  '.cm-completionDetail': { color: 'var(--color-text-subtle)', fontStyle: 'normal' },
  '.cm-completionMatchedText': { textDecoration: 'none', color: 'var(--color-accent)' },
  '.cm-completionInfo, .cm-script-documentation': {
    maxWidth: '360px',
    padding: '8px 10px',
    fontFamily: 'var(--font-sans)',
    fontSize: 'var(--text-sm)',
    lineHeight: '1.5',
  },
  '.cm-script-documentation code': {
    display: 'block',
    marginBottom: '4px',
    fontFamily: 'var(--font-mono)',
    color: 'var(--color-text-strong)',
  },
  '.cm-script-documentation p': { margin: 0, color: 'var(--color-text-muted)' },
  '.cm-diagnostic': {
    padding: '6px 10px',
    fontFamily: 'var(--font-sans)',
    fontSize: 'var(--text-sm)',
  },
  '.cm-diagnostic-error': { borderLeft: '3px solid var(--color-danger)' },
  '.cm-diagnostic-warning': { borderLeft: '3px solid var(--color-warning)' },
  '.cm-lintRange-error': {
    backgroundImage: 'none',
    textDecoration: 'underline wavy var(--color-danger)',
    textUnderlineOffset: '3px',
  },
  '.cm-lintRange-warning': {
    backgroundImage: 'none',
    textDecoration: 'underline wavy var(--color-warning)',
    textUnderlineOffset: '3px',
  },
  '.cm-gutter-lint': { width: '14px' },
  '.cm-foldPlaceholder': {
    backgroundColor: 'var(--color-active)',
    border: 'none',
    color: 'var(--color-text-muted)',
    padding: '0 6px',
    borderRadius: '4px',
  },
});

/**
 * Code editor and viewer built on CodeMirror: syntax highlighting, line
 * numbers, folding, bracket matching and undo history. Read-only, it shows a
 * response; editable, it edits a request body or a request script.
 */
@Component({
  selector: 'app-code-editor',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: '',
  styles: `
    :host {
      display: block;
      min-height: 0;
      overflow: hidden;
      border: 1px solid var(--color-border-strong);
      border-radius: var(--radius-md);
      background: var(--color-field);
    }
    :host(:focus-within) {
      border-color: var(--color-accent);
    }
    :host(.read-only:focus-within) {
      border-color: var(--color-border-strong);
    }
  `,
  host: {
    '[class.read-only]': 'readOnly()',
  },
})
export class CodeEditor {
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly language = new Compartment();
  private readonly editable = new Compartment();
  private view: EditorView | null = null;

  readonly value = input('');
  readonly lang = input<CodeLanguage>('json');
  readonly readOnly = input(false);
  readonly placeholder = input('');
  /** For JavaScript, the script whose globals are completed and checked. */
  readonly scriptKind = input<ScriptKind>('post-response');
  readonly valueChange = output<string>();

  constructor() {
    afterNextRender(() => {
      this.view = new EditorView({
        parent: this.host.nativeElement,
        state: EditorState.create({
          doc: this.value(),
          extensions: [
            lineNumbers(),
            foldGutter(),
            highlightActiveLineGutter(),
            highlightActiveLine(),
            history(),
            indentOnInput(),
            bracketMatching(),
            EditorView.lineWrapping,
            syntaxHighlighting(highlightStyle),
            theme,
            keymap.of([...defaultKeymap, ...historyKeymap, ...foldKeymap, indentWithTab]),
            this.language.of(languageExtension(this.lang(), this.scriptKind())),
            this.editable.of(editableExtension(this.readOnly())),
            placeholderExtension(this.placeholder()),
            EditorView.updateListener.of((update) => {
              if (update.docChanged) {
                this.valueChange.emit(update.state.doc.toString());
              }
            }),
          ],
        }),
      });
    });

    // Follow outside changes without resetting the cursor on every keystroke.
    effect(() => {
      const value = this.value();
      const view = this.view;
      if (view && view.state.doc.toString() !== value) {
        view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } });
      }
    });
    effect(() => {
      const extension = languageExtension(this.lang(), this.scriptKind());
      this.view?.dispatch({ effects: this.language.reconfigure(extension) });
    });
    effect(() => {
      const extension = editableExtension(this.readOnly());
      this.view?.dispatch({ effects: this.editable.reconfigure(extension) });
    });

    inject(DestroyRef).onDestroy(() => this.view?.destroy());
  }
}

function languageExtension(language: CodeLanguage, scriptKind: ScriptKind): Extension {
  switch (language) {
    case 'javascript':
      return scriptLanguage(scriptKind);
    case 'json':
      return json();
    case 'xml':
      return xml();
    default:
      return [];
  }
}

function editableExtension(readOnly: boolean): Extension {
  return [EditorState.readOnly.of(readOnly), EditorView.editable.of(!readOnly)];
}

/** Indents JSON for display, leaving anything else untouched. */
export function prettyJson(text: string): string {
  const trimmed = text.trim();
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) {
    return text;
  }
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2);
  } catch {
    return text;
  }
}
