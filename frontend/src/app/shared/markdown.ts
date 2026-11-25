import { InlineSegment, parseInlineMarkdown } from './inline-markdown';

/** How a table column aligns its cells. */
export type ColumnAlignment = 'left' | 'center' | 'right' | null;

/** A block of a Markdown document, ready to render through bindings. */
export type MarkdownBlock =
  | { kind: 'heading'; level: number; segments: InlineSegment[] }
  | { kind: 'paragraph'; segments: InlineSegment[] }
  | { kind: 'list'; ordered: boolean; start: number; items: InlineSegment[][] }
  | { kind: 'code'; language: string; text: string }
  | { kind: 'quote'; paragraphs: InlineSegment[][] }
  | { kind: 'rule' }
  | {
      kind: 'table';
      alignments: ColumnAlignment[];
      header: InlineSegment[][];
      rows: InlineSegment[][][];
    };

const HEADING_PATTERN = /^(#{1,6})\s+(.*?)\s*#*\s*$/;
const FENCE_PATTERN = /^\s*(```+|~~~+)\s*([\w+-]*)/;
const RULE_PATTERN = /^\s*([-*_])(\s*\1){2,}\s*$/;
const BULLET_PATTERN = /^\s*[-*+]\s+(.*)$/;
const ORDERED_PATTERN = /^\s*(\d{1,9})[.)]\s+(.*)$/;
const QUOTE_PATTERN = /^\s*>\s?(.*)$/;
const TABLE_DELIMITER_PATTERN = /^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$/;
const CONTINUATION_INDENT_PATTERN = /^\s{2,}\S/;

/**
 * Parses the Markdown of a documentation into blocks: headings, paragraphs,
 * lists, fenced code, quotes, rules and tables. Inline formats come from
 * parseInlineMarkdown. Raw HTML stays text, so nothing is ever injected.
 */
export function parseMarkdown(source: string): MarkdownBlock[] {
  const lines = source.replace(/\r\n?/g, '\n').split('\n');
  const blocks: MarkdownBlock[] = [];
  let position = 0;

  while (position < lines.length) {
    const line = lines[position];

    if (!line.trim()) {
      position++;
      continue;
    }

    const fence = FENCE_PATTERN.exec(line);
    if (fence) {
      const marker = fence[1];
      const content: string[] = [];
      position++;
      while (position < lines.length && !lines[position].trim().startsWith(marker)) {
        content.push(lines[position]);
        position++;
      }
      position++;
      blocks.push({ kind: 'code', language: fence[2], text: content.join('\n') });
      continue;
    }

    const heading = HEADING_PATTERN.exec(line);
    if (heading) {
      blocks.push({
        kind: 'heading',
        level: heading[1].length,
        segments: parseInlineMarkdown(heading[2]),
      });
      position++;
      continue;
    }

    if (RULE_PATTERN.test(line)) {
      blocks.push({ kind: 'rule' });
      position++;
      continue;
    }

    if (QUOTE_PATTERN.test(line)) {
      const quoted: string[] = [];
      while (position < lines.length && QUOTE_PATTERN.test(lines[position])) {
        quoted.push(QUOTE_PATTERN.exec(lines[position])![1]);
        position++;
      }
      blocks.push({ kind: 'quote', paragraphs: splitParagraphs(quoted) });
      continue;
    }

    if (BULLET_PATTERN.test(line) || ORDERED_PATTERN.test(line)) {
      const ordered = !BULLET_PATTERN.test(line);
      const itemPattern = ordered ? ORDERED_PATTERN : BULLET_PATTERN;
      const start = ordered ? Number(ORDERED_PATTERN.exec(line)![1]) : 1;
      const items: string[] = [];
      while (position < lines.length) {
        const current = lines[position];
        const item = itemPattern.exec(current);
        if (item) {
          items.push(item[ordered ? 2 : 1]);
        } else if (items.length && CONTINUATION_INDENT_PATTERN.test(current)) {
          items[items.length - 1] += ` ${current.trim()}`;
        } else {
          break;
        }
        position++;
      }
      blocks.push({
        kind: 'list',
        ordered,
        start,
        items: items.map((item) => parseInlineMarkdown(item)),
      });
      continue;
    }

    const next = lines[position + 1];
    if (line.includes('|') && next !== undefined && TABLE_DELIMITER_PATTERN.test(next)) {
      const header = splitTableRow(line);
      const alignments = splitTableRow(next).map(readAlignment);
      const rows: InlineSegment[][][] = [];
      position += 2;
      while (position < lines.length && lines[position].includes('|')) {
        const cells = splitTableRow(lines[position]);
        rows.push(header.map((_, column) => parseInlineMarkdown(cells[column] ?? '')));
        position++;
      }
      blocks.push({
        kind: 'table',
        alignments: header.map((_, column) => alignments[column] ?? null),
        header: header.map((cell) => parseInlineMarkdown(cell)),
        rows,
      });
      continue;
    }

    const paragraph: string[] = [];
    while (position < lines.length && continuesParagraph(lines[position])) {
      paragraph.push(lines[position].trim());
      position++;
    }
    blocks.push({ kind: 'paragraph', segments: parseInlineMarkdown(paragraph.join(' ')) });
  }

  return blocks;
}

/** Tells whether a line extends the paragraph above it rather than opening a block. */
function continuesParagraph(line: string): boolean {
  return (
    line.trim() !== '' &&
    !FENCE_PATTERN.test(line) &&
    !HEADING_PATTERN.test(line) &&
    !RULE_PATTERN.test(line) &&
    !QUOTE_PATTERN.test(line) &&
    !BULLET_PATTERN.test(line) &&
    !ORDERED_PATTERN.test(line)
  );
}

function splitParagraphs(lines: string[]): InlineSegment[][] {
  const paragraphs: string[][] = [[]];
  for (const line of lines) {
    if (line.trim()) {
      paragraphs[paragraphs.length - 1].push(line.trim());
    } else if (paragraphs[paragraphs.length - 1].length) {
      paragraphs.push([]);
    }
  }
  return paragraphs
    .filter((paragraph) => paragraph.length)
    .map((paragraph) => parseInlineMarkdown(paragraph.join(' ')));
}

function splitTableRow(line: string): string[] {
  const trimmed = line.trim().replace(/^\|/, '').replace(/\|$/, '');
  return trimmed.split(/(?<!\\)\|/).map((cell) => cell.trim().replace(/\\\|/g, '|'));
}

function readAlignment(delimiter: string): ColumnAlignment {
  const left = delimiter.startsWith(':');
  const right = delimiter.endsWith(':');
  if (left && right) {
    return 'center';
  }
  if (right) {
    return 'right';
  }
  return left ? 'left' : null;
}
