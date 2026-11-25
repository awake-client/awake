import { formatBytes, parseInlineMarkdown } from './inline-markdown';

describe('parseInlineMarkdown', () => {
  it('keeps plain text as a single segment', () => {
    expect(parseInlineMarkdown('Fixed a crash.')).toEqual([
      { kind: 'text', text: 'Fixed a crash.' },
    ]);
  });

  it('reads code spans, bold text and links', () => {
    expect(
      parseInlineMarkdown('Run `awake mcp` **now**, see [docs](https://example.test/a).'),
    ).toEqual([
      { kind: 'text', text: 'Run ' },
      { kind: 'code', text: 'awake mcp' },
      { kind: 'text', text: ' ' },
      { kind: 'strong', text: 'now' },
      { kind: 'text', text: ', see ' },
      { kind: 'link', text: 'docs', href: 'https://example.test/a' },
      { kind: 'text', text: '.' },
    ]);
  });

  it('reads italic text without splitting snake case words', () => {
    expect(parseInlineMarkdown('*once* per _user_ in user_id')).toEqual([
      { kind: 'emphasis', text: 'once' },
      { kind: 'text', text: ' per ' },
      { kind: 'emphasis', text: 'user' },
      { kind: 'text', text: ' in user_id' },
    ]);
  });

  it('keeps lone asterisks as text', () => {
    expect(parseInlineMarkdown('2 * 3 * 4')).toEqual([{ kind: 'text', text: '2 * 3 * 4' }]);
  });

  it('links bare URLs without their trailing punctuation', () => {
    expect(parseInlineMarkdown('in https://example.test/pull/1.')).toEqual([
      { kind: 'text', text: 'in ' },
      { kind: 'link', text: 'https://example.test/pull/1', href: 'https://example.test/pull/1' },
      { kind: 'text', text: '.' },
    ]);
  });

  it('never links other schemes', () => {
    expect(parseInlineMarkdown('[x](javascript:alert(1))')).toEqual([
      { kind: 'text', text: '[x](javascript:alert(1))' },
    ]);
  });

  it('keeps HTML as text', () => {
    expect(parseInlineMarkdown('<img src=x>')).toEqual([{ kind: 'text', text: '<img src=x>' }]);
  });
});

describe('formatBytes', () => {
  it('picks a readable unit', () => {
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(1536)).toBe('1.5 KB');
    expect(formatBytes(12.4 * 1024 * 1024)).toBe('12.4 MB');
    expect(formatBytes(150 * 1024 * 1024)).toBe('150 MB');
  });
});
