import { parseMarkdown } from './markdown';

describe('parseMarkdown', () => {
  it('reads headings and joins the lines of a paragraph', () => {
    expect(parseMarkdown('## Create a user\n\nSends the\n**profile**.')).toEqual([
      { kind: 'heading', level: 2, segments: [{ kind: 'text', text: 'Create a user' }] },
      {
        kind: 'paragraph',
        segments: [
          { kind: 'text', text: 'Sends the ' },
          { kind: 'strong', text: 'profile' },
          { kind: 'text', text: '.' },
        ],
      },
    ]);
  });

  it('reads bullet and ordered lists with continuation lines', () => {
    expect(parseMarkdown('- one\n  more\n* two\n\n3. three\n4. four')).toEqual([
      {
        kind: 'list',
        ordered: false,
        start: 1,
        items: [[{ kind: 'text', text: 'one more' }], [{ kind: 'text', text: 'two' }]],
      },
      {
        kind: 'list',
        ordered: true,
        start: 3,
        items: [[{ kind: 'text', text: 'three' }], [{ kind: 'text', text: 'four' }]],
      },
    ]);
  });

  it('keeps fenced code verbatim', () => {
    expect(parseMarkdown('```json\n{ "a": **1** }\n\n```')).toEqual([
      { kind: 'code', language: 'json', text: '{ "a": **1** }\n' },
    ]);
  });

  it('reads quotes and rules', () => {
    expect(parseMarkdown('> Needs a token\n>\n> from the admin\n\n---')).toEqual([
      {
        kind: 'quote',
        paragraphs: [
          [{ kind: 'text', text: 'Needs a token' }],
          [{ kind: 'text', text: 'from the admin' }],
        ],
      },
      { kind: 'rule' },
    ]);
  });

  it('reads tables with their alignment and pads short rows', () => {
    expect(parseMarkdown('| Code | Meaning |\n| :--- | ---: |\n| 200 | `ok` |\n| 404 |')).toEqual([
      {
        kind: 'table',
        alignments: ['left', 'right'],
        header: [[{ kind: 'text', text: 'Code' }], [{ kind: 'text', text: 'Meaning' }]],
        rows: [
          [[{ kind: 'text', text: '200' }], [{ kind: 'code', text: 'ok' }]],
          [[{ kind: 'text', text: '404' }], []],
        ],
      },
    ]);
  });

  it('keeps HTML as text', () => {
    expect(parseMarkdown('<script>alert(1)</script>')).toEqual([
      { kind: 'paragraph', segments: [{ kind: 'text', text: '<script>alert(1)</script>' }] },
    ]);
  });

  it('returns nothing for blank documentation', () => {
    expect(parseMarkdown('  \n\n')).toEqual([]);
  });
});
