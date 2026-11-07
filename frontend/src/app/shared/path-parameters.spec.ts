import { pathParameterNames, replaceUrlPlaceholders, urlPlaceholders } from './path-parameters';

describe('urlPlaceholders', () => {
  it('reads path segments and whole query values', () => {
    const placeholders = urlPlaceholders(
      '{{base_url}}/api/articles/:articleId?query=:todoId&time=12:30&partial=a:b#top',
    );
    expect(placeholders.map(({ name, inQuery }) => ({ name, inQuery }))).toEqual([
      { name: 'articleId', inQuery: false },
      { name: 'todoId', inQuery: true },
    ]);
  });

  it('ignores schemes and ports', () => {
    expect(urlPlaceholders('https://api.example.com:8080/users')).toEqual([]);
  });

  it('lists each name once', () => {
    expect(pathParameterNames('/a/:x?b=:y&c=:x')).toEqual(['x', 'y']);
  });

  it('replaces the colon and the name only', () => {
    expect(
      replaceUrlPlaceholders('/a/:x?b=:y', (placeholder) => placeholder.name.toUpperCase()),
    ).toBe('/a/X?b=Y');
  });
});
