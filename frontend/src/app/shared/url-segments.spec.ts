import { urlSegments } from './variable-input';

describe('urlSegments', () => {
  it('marks path parameters and the parts of the query', () => {
    const segments = urlSegments('/api/articles/:articleId/publish?page=2&sort=desc', {
      inQuery: false,
      inValue: false,
    });
    expect(segments).toEqual([
      { text: '/api/articles/', state: 'text' },
      { text: ':articleId', state: 'path-parameter' },
      { text: '/publish', state: 'text' },
      { text: '?', state: 'query-separator' },
      { text: 'page', state: 'query-name' },
      { text: '=', state: 'query-separator' },
      { text: '2', state: 'query-value' },
      { text: '&', state: 'query-separator' },
      { text: 'sort', state: 'query-name' },
      { text: '=', state: 'query-separator' },
      { text: 'desc', state: 'query-value' },
    ]);
  });

  it('leaves schemes and ports alone', () => {
    expect(
      urlSegments('https://api.example.com:8080/users', { inQuery: false, inValue: false }),
    ).toEqual([{ text: 'https://api.example.com:8080/users', state: 'text' }]);
  });

  it('carries the position across a variable', () => {
    const position = { inQuery: false, inValue: false };
    urlSegments('/search?token=', position);
    expect(urlSegments('&page=1', position)[0]).toEqual({ text: '&', state: 'query-separator' });
    expect(position).toEqual({ inQuery: true, inValue: true });
  });

  it('marks a whole query value written :name', () => {
    const segments = urlSegments('/todos?query=:todoId&time=12:30', {
      inQuery: false,
      inValue: false,
    });
    expect(segments).toContainEqual({ text: ':todoId', state: 'path-parameter' });
    expect(segments).toContainEqual({ text: '12:30', state: 'query-value' });
  });
});
