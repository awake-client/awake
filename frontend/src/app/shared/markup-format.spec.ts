import { looksLikeMarkup, prettyMarkup } from './markup-format';

describe('prettyMarkup', () => {
  it('indents nested XML and keeps text-only elements on one line', () => {
    const input =
      '<?xml version="1.0"?><people><person id="1"><name>Chris</name><craft>ISS</craft></person><empty/></people>';

    expect(prettyMarkup(input)).toBe(
      [
        '<?xml version="1.0"?>',
        '<people>',
        '  <person id="1">',
        '    <name>Chris</name>',
        '    <craft>ISS</craft>',
        '  </person>',
        '  <empty/>',
        '</people>',
      ].join('\n'),
    );
  });

  it('indents HTML and leaves void elements unindented', () => {
    const input =
      '<!DOCTYPE html><html><head><title>Hi</title></head><body><img src="a.png"><p>Hello <b>world</b></p></body></html>';

    expect(prettyMarkup(input)).toBe(
      [
        '<!DOCTYPE html>',
        '<html>',
        '  <head>',
        '    <title>Hi</title>',
        '  </head>',
        '  <body>',
        '    <img src="a.png">',
        '    <p>',
        '      Hello',
        '      <b>world</b>',
        '    </p>',
        '  </body>',
        '</html>',
      ].join('\n'),
    );
  });

  it('does not split on > inside attribute values', () => {
    expect(prettyMarkup('<a title="1 > 0">x</a>')).toBe('<a title="1 > 0">x</a>');
  });

  it('preserves the content of raw elements', () => {
    expect(prettyMarkup('<div><pre>line 1\n  line 2</pre></div>')).toBe(
      ['<div>', '  <pre>line 1', '  line 2</pre>', '</div>'].join('\n'),
    );
    expect(prettyMarkup('<script>if (a < b) { x(); }</script>')).toBe(
      '<script>if (a < b) { x(); }</script>',
    );
  });

  it('formats a truncated body without failing', () => {
    expect(prettyMarkup('<root><item>1</item>')).toBe('<root>\n  <item>1</item>');
    expect(prettyMarkup('')).toBe('');
  });
});

describe('looksLikeMarkup', () => {
  it.each([
    ['<?xml version="1.0"?>', true],
    ['<!doctype html>', true],
    ['<root>', true],
    ['<p class="x">', true],
    ['{"a": 1}', false],
    ['hello <b>world</b>', false],
    ['', false],
  ])('detects %s as markup: %s', (body, expected) => {
    expect(looksLikeMarkup(body)).toBe(expected);
  });
});
