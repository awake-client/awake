import { Parameter } from '@bindings/github.com/clementherve/awake/src/domain/models';
import { composeUrlQuery, mergeUrlParameters, parseUrlQuery } from './url-query';

function parameter(Name: string, Value: string, Enabled = true): Parameter {
  return { Name, Value, Enabled };
}

describe('parseUrlQuery', () => {
  it('reads the query entries as parameters', () => {
    const parsed = parseUrlQuery('https://api.example.com/users?page=1&q=a%20b');
    expect(parsed.url).toBe('https://api.example.com/users');
    expect(parsed.parameters).toEqual([parameter('page', '1'), parameter('q', 'a b')]);
  });

  it('keeps whole :name placeholder values in the URL', () => {
    const parsed = parseUrlQuery('/articles/:id?author=:authorId&page=1');
    expect(parsed.url).toBe('/articles/:id?author=:authorId');
    expect(parsed.parameters).toEqual([parameter('page', '1')]);
  });

  it('leaves the fragment at the end', () => {
    expect(parseUrlQuery('/a?page=1#top')).toEqual({
      url: '/a#top',
      parameters: [parameter('page', '1')],
    });
  });

  it('keeps a URL without a query untouched', () => {
    expect(parseUrlQuery('/a/:id')).toEqual({ url: '/a/:id', parameters: [] });
  });
});

describe('composeUrlQuery', () => {
  it('writes enabled parameters after the URL query', () => {
    expect(
      composeUrlQuery('/a/:id?author=:authorId#top', [
        parameter('page', '1'),
        parameter('draft', 'true', false),
      ]),
    ).toBe('/a/:id?author=:authorId&page=1#top');
  });

  it('drops empty names and returns the URL as is without parameters', () => {
    expect(composeUrlQuery('/a', [parameter('', '1')])).toBe('/a');
  });

  it('escapes what would end an entry and unescapes it back', () => {
    const original = [parameter('q', 'a&b=c d+e%f'), parameter('flag', ':id')];
    const url = composeUrlQuery('/a', original);

    expect(url).toBe('/a?q=a%26b=c d%2Be%25f&flag=%3Aid');
    expect(parseUrlQuery(url).parameters).toEqual(original);
  });

  it('round-trips a URL with placeholders and a fragment', () => {
    const url = '/a/:id?author=:authorId#top';
    const parameters = [parameter('page', '1'), parameter('q', 'a b')];
    expect(parseUrlQuery(composeUrlQuery(url, parameters))).toEqual({
      url,
      parameters,
    });
  });
});

describe('mergeUrlParameters', () => {
  it('drops the stored copies of what the URL already holds', () => {
    expect(
      mergeUrlParameters(
        [parameter('page', '1')],
        [parameter('page', '1'), parameter('page', '2'), parameter('draft', 'true', false)],
      ),
    ).toEqual([parameter('page', '1'), parameter('page', '2'), parameter('draft', 'true', false)]);
  });

  it('keeps a query written in both places once', () => {
    const parsed = parseUrlQuery(
      'https://conso.boris.sh/api/daily_consumption?prm={{prm}}&start=2026-06-27&end=2026-07-02',
    );
    const merged = mergeUrlParameters(parsed.parameters, [
      parameter('prm', '{{prm}}'),
      parameter('start', '2026-06-27'),
      parameter('end', '2026-07-02'),
    ]);

    expect(composeUrlQuery(parsed.url, merged)).toBe(
      'https://conso.boris.sh/api/daily_consumption?prm={{prm}}&start=2026-06-27&end=2026-07-02',
    );
  });
});
