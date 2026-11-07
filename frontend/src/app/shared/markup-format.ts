/** Markup tags that never wrap content, so they do not push an indent level. */
const VOID_ELEMENTS = new Set([
  'area',
  'base',
  'br',
  'col',
  'embed',
  'hr',
  'img',
  'input',
  'link',
  'meta',
  'param',
  'source',
  'track',
  'wbr',
]);

/** Elements whose content is preserved as-is instead of being re-indented. */
const RAW_TEXT_ELEMENTS = new Set(['pre', 'script', 'style', 'textarea']);

type TokenKind = 'open' | 'close' | 'self' | 'text' | 'comment' | 'cdata' | 'declaration';

interface MarkupToken {
  kind: TokenKind;
  value: string;
  name?: string;
  /** Text captured inside a raw element, kept exactly as it arrived. */
  raw?: boolean;
}

/**
 * True when the body looks like XML or HTML. Used when the response has no
 * useful content type, so a JSON or plain text body is left alone.
 */
export function looksLikeMarkup(text: string): boolean {
  return /^\s*(?:<\?xml|<!doctype|<!--|<!\[CDATA\[|<\/?[a-z][\w:.-]*[\s/>])/i.test(text);
}

/**
 * Indents XML and HTML so a minified response becomes readable. Elements that
 * only hold text stay on one line, and the content of pre, script, style and
 * textarea is never touched.
 */
export function prettyMarkup(input: string): string {
  const tokens = tokenizeMarkup(input);
  const lines: string[] = [];
  let depth = 0;

  const push = (value: string) => {
    const trimmed = value.trim();
    if (!trimmed) {
      return;
    }
    const prefix = '  '.repeat(depth);
    const [first, ...rest] = trimmed.split('\n');
    lines.push(prefix + first, ...rest.map((line) => prefix + line));
  };

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];

    switch (token.kind) {
      case 'text':
        if (token.raw) {
          const raw = token.value.replace(/^\n+|\n+$/g, '');
          if (raw.trim()) {
            lines.push(raw);
          }
        } else {
          push(token.value);
        }
        break;

      case 'close':
        depth = Math.max(0, depth - 1);
        push(token.value);
        break;

      case 'self':
      case 'comment':
      case 'cdata':
      case 'declaration':
        push(token.value);
        break;

      case 'open': {
        const next = tokens[index + 1];
        const after = tokens[index + 2];

        // <name></name>
        if (next?.kind === 'close' && next.name === token.name) {
          push(token.value + next.value);
          index += 1;
          break;
        }

        // A raw element keeps its content exactly, surrounding tags included.
        if (
          next?.kind === 'text' &&
          next.raw &&
          after?.kind === 'close' &&
          after.name === token.name
        ) {
          lines.push('  '.repeat(depth) + token.value + next.value + after.value);
          index += 2;
          break;
        }

        // <name>value</name> reads better on a single line.
        if (
          next?.kind === 'text' &&
          !next.raw &&
          after?.kind === 'close' &&
          after.name === token.name &&
          !next.value.includes('\n')
        ) {
          push(`${token.value}${next.value.trim()}${after.value}`);
          index += 2;
          break;
        }

        push(token.value);
        depth += 1;
        break;
      }
    }
  }

  return lines.join('\n');
}

function tokenizeMarkup(input: string): MarkupToken[] {
  const tokens: MarkupToken[] = [];
  let index = 0;

  while (index < input.length) {
    if (input[index] !== '<') {
      const next = input.indexOf('<', index);
      const end = next === -1 ? input.length : next;
      tokens.push({ kind: 'text', value: input.slice(index, end) });
      index = end;
      continue;
    }

    if (input.startsWith('<!--', index)) {
      const end = input.indexOf('-->', index + 4);
      const stop = end === -1 ? input.length : end + 3;
      tokens.push({ kind: 'comment', value: input.slice(index, stop) });
      index = stop;
      continue;
    }

    if (input.startsWith('<![CDATA[', index)) {
      const end = input.indexOf(']]>', index + 9);
      const stop = end === -1 ? input.length : end + 3;
      tokens.push({ kind: 'cdata', value: input.slice(index, stop) });
      index = stop;
      continue;
    }

    if (input.startsWith('<!', index) || input.startsWith('<?', index)) {
      const end = input.indexOf('>', index + 2);
      const stop = end === -1 ? input.length : end + 1;
      tokens.push({ kind: 'declaration', value: input.slice(index, stop) });
      index = stop;
      continue;
    }

    const stop = tagEnd(input, index);
    const value = input.slice(index, stop);
    const name = (/^<\/?\s*([^\s/>]+)/.exec(value)?.[1] ?? '').toLowerCase();
    const closing = value.startsWith('</');
    const selfClosing = value.endsWith('/>') || VOID_ELEMENTS.has(name);

    tokens.push({ kind: closing ? 'close' : selfClosing ? 'self' : 'open', value, name });
    index = stop;

    if (!closing && !selfClosing && RAW_TEXT_ELEMENTS.has(name)) {
      const closeAt = closingTag(input, index, name);
      if (closeAt !== -1) {
        if (closeAt > index) {
          tokens.push({ kind: 'text', value: input.slice(index, closeAt), raw: true });
        }
        index = closeAt;
      }
    }
  }

  return tokens;
}

/** Position just after the '>' that closes a tag, ignoring quoted '>'. */
function tagEnd(input: string, start: number): number {
  let index = start + 1;
  let quote = '';

  while (index < input.length) {
    const char = input[index];
    if (quote) {
      if (char === quote) {
        quote = '';
      }
    } else if (char === '"' || char === "'") {
      quote = char;
    } else if (char === '>') {
      return index + 1;
    }
    index += 1;
  }

  return input.length;
}

/** Position of the "</name" that closes a raw text element, or -1. */
function closingTag(input: string, start: number, name: string): number {
  const needle = `</${name}`;
  const haystack = input.toLowerCase();
  let from = start;

  while (from < input.length) {
    const at = haystack.indexOf(needle, from);
    if (at === -1) {
      return -1;
    }
    const next = input[at + needle.length];
    if (next === undefined || next === '>' || next === '/' || /\s/.test(next)) {
      return at;
    }
    from = at + needle.length;
  }

  return -1;
}
