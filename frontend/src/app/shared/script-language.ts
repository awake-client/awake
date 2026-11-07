import {
  Completion,
  CompletionContext,
  CompletionResult,
  autocompletion,
  closeBrackets,
  closeBracketsKeymap,
  completionKeymap,
} from '@codemirror/autocomplete';
import { javascript, javascriptLanguage } from '@codemirror/lang-javascript';
import { ensureSyntaxTree, syntaxTree } from '@codemirror/language';
import { Diagnostic, linter, lintGutter } from '@codemirror/lint';
import { EditorState, Extension, Prec } from '@codemirror/state';
import { EditorView, hoverTooltip, keymap } from '@codemirror/view';
import type { SyntaxNode } from '@lezer/common';

/** When a script runs, which decides the globals it can use. */
export type ScriptKind = 'pre-request' | 'post-response';

/**
 * One value of the script API. Members are listed for objects whose shape is
 * fixed; open maps such as headers leave them out so any key is accepted.
 */
interface ApiEntry {
  type: 'variable' | 'property' | 'method' | 'namespace';
  /** Type or signature shown next to the name. */
  detail: string;
  documentation: string;
  members?: Record<string, ApiEntry>;
  /** For a method, what a call returns, so a chain can continue after it. */
  returns?: ApiEntry;
}

const LINT_DELAY_MILLISECONDS = 300;
const PARSE_TIMEOUT_MILLISECONDS = 200;
/** Call arguments without nested parentheses, such as ("Production"). */
const CALL_ARGUMENTS = /\((?:[^()'"`]|'[^']*'|"[^"]*")*\)/g;
const IDENTIFIER_CHAIN = /^[A-Za-z_$][\w$]*(?:\(\))?(?:\s*\.\s*[A-Za-z_$][\w$]*(?:\(\))?)*$/;
const CHAIN_BEFORE_CURSOR =
  /(?:[A-Za-z_$][\w$]*(?:\((?:[^()'"`]|'[^']*'|"[^"]*")*\))?\s*\.\s*)*[A-Za-z_$]*$/;

const requestEntry = (editable: boolean): ApiEntry => ({
  type: 'variable',
  detail: 'Request',
  documentation: editable
    ? 'The request about to be sent. Changes to it are applied before sending.'
    : 'The request that was sent, as a read-only copy.',
  members: {
    method: { type: 'property', detail: 'string', documentation: 'HTTP method, such as GET.' },
    url: {
      type: 'property',
      detail: 'string',
      documentation: 'URL, before variable interpolation.',
    },
    headers: {
      type: 'property',
      detail: 'Record<string, string>',
      documentation: 'Enabled headers by name. Delete a key to drop the header.',
    },
    query: {
      type: 'property',
      detail: 'Record<string, string>',
      documentation: 'Enabled query parameters by name.',
    },
    body: { type: 'property', detail: 'string', documentation: 'Raw body content.' },
  },
});

const responseEntry: ApiEntry = {
  type: 'variable',
  detail: 'Response',
  documentation: 'The response received. The body holds the first 256 KiB.',
  members: {
    status: { type: 'property', detail: 'number', documentation: 'Status code, such as 200.' },
    statusText: {
      type: 'property',
      detail: 'string',
      documentation: 'Status line, such as "200 OK".',
    },
    headers: {
      type: 'property',
      detail: 'Record<string, string>',
      documentation: 'Response headers by canonical name; repeated values are joined with ", ".',
    },
    body: {
      type: 'property',
      detail: 'string',
      documentation: 'Body preview, base64 encoded when encoding is "base64".',
    },
    encoding: { type: 'property', detail: 'string', documentation: '"utf-8" or "base64".' },
    duration: { type: 'property', detail: 'number', documentation: 'Duration in milliseconds.' },
    cookies: {
      type: 'property',
      detail: 'Record<string, string>',
      documentation: 'Cookies set by the response, by name, parsed from every Set-Cookie header.',
    },
    cookieHeader: {
      type: 'property',
      detail: 'string',
      documentation:
        'The cookies set by the response as "name=value; other=value", ready for a Cookie request header.',
    },
    json: {
      type: 'method',
      detail: '(): any',
      documentation: 'Parses the body as JSON. Throws when the body is not valid JSON.',
    },
  },
};

const environmentMembers = (subject: string): Record<string, ApiEntry> => ({
  get: {
    type: 'method',
    detail: '(name: string): string | null',
    documentation: `Reads a variable of ${subject}.`,
  },
  set: {
    type: 'method',
    detail: '(name: string, value: any): void',
    documentation: `Saves a variable in ${subject}, in its local overrides, which git ignores. A declared secret is saved in the keychain. Non-string values are stored as JSON.`,
  },
  unset: {
    type: 'method',
    detail: '(name: string): void',
    documentation: `Removes a variable from the local overrides of ${subject}, or a secret from the keychain.`,
  },
});

const namedEnvironmentEntry: ApiEntry = {
  type: 'variable',
  detail: 'NamedEnvironment',
  documentation:
    'An environment opened by name. Its changes are saved but do not affect the running request.',
  members: {
    name: {
      type: 'property',
      detail: 'string',
      documentation: 'Display name of the environment, such as "Production".',
    },
    fileName: {
      type: 'property',
      detail: 'string',
      documentation: 'File name of the environment, without the .yaml extension.',
    },
    ...environmentMembers('this environment'),
  },
};

const logEntry = (documentation: string): ApiEntry => ({
  type: 'method',
  detail: '(...values: any[]): void',
  documentation,
});

const consoleEntry: ApiEntry = {
  type: 'namespace',
  detail: 'Console',
  documentation: 'Writes to the script logs shown with the response.',
  members: {
    log: logEntry('Appends a line to the script logs.'),
    info: logEntry('Appends a line to the script logs.'),
    debug: logEntry('Appends a line to the script logs.'),
    warn: logEntry('Appends a line prefixed with "warning:" to the script logs.'),
    error: logEntry('Appends a line prefixed with "error:" to the script logs.'),
  },
};

const awakeEntry: ApiEntry = {
  type: 'namespace',
  detail: 'Awake',
  documentation: 'Helpers provided by Awake.',
  members: {
    log: {
      type: 'method',
      detail: '(...values: any[]): void',
      documentation: 'Appends a line to the script logs shown with the response.',
    },
    test: {
      type: 'method',
      detail: '(name: string, assertion: boolean | (() => boolean | void)): void',
      documentation: 'Records an assertion. A function fails when it throws or returns false.',
    },
    getVariable: {
      type: 'method',
      detail: '(name: string): string | null',
      documentation: 'Reads a resolved variable.',
    },
    setVariable: {
      type: 'method',
      detail: '(name: string, value: any): void',
      documentation: 'Sets a variable for this execution only. Nothing is saved.',
    },
    environment: {
      type: 'namespace',
      detail: 'Environment',
      documentation:
        'The selected environment. Values set here apply to this request and are saved for the next ones.',
      members: environmentMembers('the selected environment'),
    },
    environments: {
      type: 'namespace',
      detail: 'Environments',
      documentation: 'Any environment visible to this request, opened by name.',
      members: {
        get: {
          type: 'method',
          detail: '(name: string): NamedEnvironment',
          documentation:
            'Opens an environment by file name or display name, such as "production" or "Production". Throws when no environment matches.',
          returns: namedEnvironmentEntry,
        },
      },
    },
  },
};

const variablesEntry: ApiEntry = {
  type: 'variable',
  detail: 'Record<string, string>',
  documentation: 'Every resolved variable by name. Writes apply to this execution only.',
};

function globalsFor(kind: ScriptKind): Record<string, ApiEntry> {
  const globals: Record<string, ApiEntry> = {
    awake: awakeEntry,
    console: consoleEntry,
    request: requestEntry(kind === 'pre-request'),
    variables: variablesEntry,
  };
  if (kind === 'post-response') {
    globals['response'] = responseEntry;
  }
  return globals;
}

/**
 * JavaScript support for request scripts: highlighting, completions and
 * documentation for the Awake API, and diagnostics for syntax errors and
 * misspelled API members.
 */
export function scriptLanguage(kind: ScriptKind): Extension {
  const globals = globalsFor(kind);
  return [
    javascript(),
    javascriptLanguage.data.of({ autocomplete: apiCompletions(globals) }),
    autocompletion({ activateOnTyping: true }),
    closeBrackets(),
    Prec.high(keymap.of([...closeBracketsKeymap, ...completionKeymap])),
    lintGutter(),
    linter((view) => diagnose(view.state, globals, kind), { delay: LINT_DELAY_MILLISECONDS }),
    hoverTooltip((view, position) => apiTooltip(view, position, globals)),
  ];
}

/** Diagnostics of a script, as the editor reports them. */
export function diagnoseScript(source: string, kind: ScriptKind): Diagnostic[] {
  const state = EditorState.create({ doc: source, extensions: [javascript()] });
  return diagnose(state, globalsFor(kind), kind);
}

// Completions -----------------------------------------------------------------

function apiCompletions(globals: Record<string, ApiEntry>) {
  return (context: CompletionContext): CompletionResult | null => {
    const node = syntaxTree(context.state).resolveInner(context.pos, -1);
    if (['String', 'TemplateString', 'LineComment', 'BlockComment'].includes(node.name)) {
      return null;
    }

    const chain = context.matchBefore(CHAIN_BEFORE_CURSOR);
    if (!chain || (chain.text === '' && !context.explicit)) {
      return null;
    }

    const segments = chainSegments(chain.text.trim() === '' ? '' : chain.text + '_');
    if (!segments) {
      return null;
    }
    const typed = (segments.pop() ?? '').slice(0, -1);
    const members = segments.length === 0 ? globals : resolve(globals, segments)?.members;
    if (!members) {
      return null;
    }

    return {
      from: context.pos - typed.length,
      options: Object.entries(members).map(([name, entry]) => toCompletion(name, entry)),
      validFor: /^[\w$]*$/,
    };
  };
}

function toCompletion(name: string, entry: ApiEntry): Completion {
  return {
    label: name,
    type: entry.type === 'namespace' ? 'namespace' : entry.type,
    detail: entry.detail,
    info: entry.documentation,
    boost: 1,
  };
}

/**
 * Splits a member chain such as awake.environments.get("x").set into its
 * segments, calls ending in "()". Returns null for anything more complex.
 */
function chainSegments(text: string): string[] | null {
  if (text === '') {
    return [''];
  }
  const simplified = text.replace(CALL_ARGUMENTS, '()');
  if (!IDENTIFIER_CHAIN.test(simplified)) {
    return null;
  }
  return simplified.split('.').map((segment) => segment.trim());
}

function resolve(globals: Record<string, ApiEntry>, path: string[]): ApiEntry | undefined {
  let members: Record<string, ApiEntry> | undefined = globals;
  let entry: ApiEntry | undefined;
  for (const segment of path) {
    const called = segment.endsWith('()');
    const name = called ? segment.slice(0, -2) : segment;
    entry = members && Object.hasOwn(members, name) ? members[name] : undefined;
    if (entry && called) {
      entry = entry.returns;
    }
    if (!entry) {
      return undefined;
    }
    members = entry.members;
  }
  return entry;
}

// Diagnostics -----------------------------------------------------------------

function diagnose(
  state: EditorState,
  globals: Record<string, ApiEntry>,
  kind: ScriptKind,
): Diagnostic[] {
  const tree = ensureSyntaxTree(state, state.doc.length, PARSE_TIMEOUT_MILLISECONDS);
  if (!tree) {
    return [];
  }

  const diagnostics: Diagnostic[] = [];
  const declared = declaredNames(state, tree.topNode);

  tree.iterate({
    enter: (reference) => {
      if (reference.type.isError) {
        diagnostics.push(syntaxError(state, reference.from, reference.to));
        return false;
      }
      if (reference.name === 'VariableName' && kind === 'pre-request') {
        const name = state.sliceDoc(reference.from, reference.to);
        if (name === 'response' && !declared.has(name)) {
          diagnostics.push({
            from: reference.from,
            to: reference.to,
            severity: 'error',
            message: 'response is only available in the post-response script.',
          });
        }
      }
      if (reference.name === 'MemberExpression') {
        const diagnostic = unknownMember(state, reference.node, globals, declared);
        if (diagnostic) {
          diagnostics.push(diagnostic);
        }
      }
      return undefined;
    },
  });

  return diagnostics;
}

function syntaxError(state: EditorState, from: number, to: number): Diagnostic {
  // An error node may be empty, at the end of the document for example; the
  // range still needs a character to underline.
  const start = Math.min(from, Math.max(0, state.doc.length - 1));
  const end = Math.max(to, Math.min(start + 1, state.doc.length));
  const line = state.doc.lineAt(start);
  return {
    from: start,
    to: end,
    severity: 'error',
    message: `Syntax error on line ${line.number}.`,
  };
}

function unknownMember(
  state: EditorState,
  node: SyntaxNode,
  globals: Record<string, ApiEntry>,
  declared: Set<string>,
): Diagnostic | null {
  const property = node.lastChild;
  if (!property || property.name !== 'PropertyName') {
    return null;
  }
  const segments = chainSegments(state.sliceDoc(node.from, node.to));
  if (!segments || declared.has(segments[0].replace('()', ''))) {
    return null;
  }
  const name = segments.pop() ?? '';
  const owner = resolve(globals, segments);
  if (!owner?.members || Object.hasOwn(owner.members, name)) {
    return null;
  }

  const suggestion = closestName(name, Object.keys(owner.members));
  return {
    from: property.from,
    to: property.to,
    severity: 'warning',
    message:
      `${segments.join('.')} has no member "${name}".` +
      (suggestion ? ` Did you mean "${suggestion}"?` : ''),
  };
}

/** Names the script declares itself, which shadow the Awake globals. */
function declaredNames(state: EditorState, top: SyntaxNode): Set<string> {
  const names = new Set<string>();
  const cursor = top.cursor();
  do {
    if (cursor.name === 'VariableDefinition') {
      names.add(state.sliceDoc(cursor.from, cursor.to));
    }
  } while (cursor.next());
  return names;
}

function closestName(name: string, candidates: string[]): string | null {
  const maximumDistance = Math.max(1, Math.floor(name.length / 3));
  let best: string | null = null;
  let bestDistance = Number.POSITIVE_INFINITY;
  for (const candidate of candidates) {
    const distance = editDistance(name.toLowerCase(), candidate.toLowerCase());
    if (distance < bestDistance) {
      best = candidate;
      bestDistance = distance;
    }
  }
  return bestDistance <= maximumDistance ? best : null;
}

function editDistance(first: string, second: string): number {
  let previous = Array.from({ length: second.length + 1 }, (_, position) => position);
  for (let row = 1; row <= first.length; row++) {
    const current = [row];
    for (let column = 1; column <= second.length; column++) {
      const substitution = first[row - 1] === second[column - 1] ? 0 : 1;
      current[column] = Math.min(
        previous[column] + 1,
        current[column - 1] + 1,
        previous[column - 1] + substitution,
      );
    }
    previous = current;
  }
  return previous[second.length];
}

// Documentation on hover ------------------------------------------------------

function apiTooltip(view: EditorView, position: number, globals: Record<string, ApiEntry>) {
  const node = syntaxTree(view.state).resolveInner(position, 1);
  if (node.name !== 'VariableName' && node.name !== 'PropertyName') {
    return null;
  }

  const expression = node.name === 'PropertyName' && node.parent ? node.parent : node;
  const path = chainSegments(view.state.sliceDoc(expression.from, expression.to));
  const entry = path ? resolve(globals, path) : undefined;
  if (!path || !entry) {
    return null;
  }
  if (!entry) {
    return null;
  }

  return {
    pos: node.from,
    end: node.to,
    above: true,
    create: () => {
      const dom = document.createElement('div');
      dom.className = 'cm-script-documentation';
      const signature = document.createElement('code');
      signature.textContent = `${path.join('.')}${entry.type === 'method' ? '' : ': '}${entry.detail}`;
      const documentation = document.createElement('p');
      documentation.textContent = entry.documentation;
      dom.append(signature, documentation);
      return { dom };
    },
  };
}
