import { pathParameterNames } from './path-parameters';

describe('pathParameterNames', () => {
  it('reads the :name segments once each, in order', () => {
    expect(pathParameterNames('{{base_url}}/api/articles/:articleId/tags/:tag/:articleId')).toEqual(
      ['articleId', 'tag'],
    );
  });

  it('ignores schemes and ports', () => {
    expect(pathParameterNames('https://api.example.com:8080/users')).toEqual([]);
  });
});
