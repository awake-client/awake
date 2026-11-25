/** A run of text inside a line of Markdown, with the formatting it carries. */
export type InlineSegment =
  | { kind: 'text'; text: string }
  | { kind: 'code'; text: string }
  | { kind: 'strong'; text: string }
  | { kind: 'emphasis'; text: string }
  | { kind: 'link'; text: string; href: string };

const INLINE_PATTERN =
  /`([^`]+)`|\*\*([^*]+)\*\*|__([^_]+)__|\*([^*\s](?:[^*]*[^*\s])?)\*|(?<![\w])_([^_\s](?:[^_]*[^_\s])?)_(?![\w])|\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)|(https?:\/\/[^\s)<>]+[^\s)<>.,;:!?'"])/g;

/**
 * Splits a line of Markdown into its inline formats: code spans, bold and
 * italic text, and links. Everything else stays plain text, so the result
 * renders through bindings, never as HTML. Only http and https links are kept.
 */
export function parseInlineMarkdown(source: string): InlineSegment[] {
  const segments: InlineSegment[] = [];
  let cursor = 0;
  const pushText = (text: string) => {
    if (text) {
      segments.push({ kind: 'text', text });
    }
  };

  for (const match of source.matchAll(INLINE_PATTERN)) {
    const start = match.index ?? 0;
    pushText(source.slice(cursor, start));
    const [whole, code, strong, underlined, starred, underscored, label, href, bareUrl] = match;
    if (code !== undefined) {
      segments.push({ kind: 'code', text: code });
    } else if (strong !== undefined || underlined !== undefined) {
      segments.push({ kind: 'strong', text: strong ?? underlined });
    } else if (starred !== undefined || underscored !== undefined) {
      segments.push({ kind: 'emphasis', text: starred ?? underscored });
    } else if (label !== undefined) {
      segments.push({ kind: 'link', text: label, href });
    } else {
      segments.push({ kind: 'link', text: bareUrl, href: bareUrl });
    }
    cursor = start + whole.length;
  }
  pushText(source.slice(cursor));
  return segments;
}

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB'];
const BYTES_PER_UNIT = 1024;

/** Formats a byte count such as 12.4 MB. */
export function formatBytes(bytes: number): string {
  let value = Math.max(0, bytes);
  let unit = 0;
  while (value >= BYTES_PER_UNIT && unit < BYTE_UNITS.length - 1) {
    value /= BYTES_PER_UNIT;
    unit++;
  }
  const digits = unit === 0 || value >= 100 ? 0 : 1;
  return `${value.toFixed(digits)} ${BYTE_UNITS[unit]}`;
}
