import { parseFormContent } from './form-body';

describe('parseFormContent', () => {
  it('reads an encoded query', () => {
    expect(parseFormContent('title=Hello+world&tags=foo%2Cbar')).toEqual([
      { Name: 'title', Value: 'Hello world', Enabled: true },
      { Name: 'tags', Value: 'foo,bar', Enabled: true },
    ]);
  });

  it('reads one field per line, ~ disabling it', () => {
    expect(parseFormContent('  title: test\n  ~draft: yes\n  url: https://x.dev/?a=b\n')).toEqual([
      { Name: 'title', Value: 'test', Enabled: true },
      { Name: 'draft', Value: 'yes', Enabled: false },
      { Name: 'url', Value: 'https://x.dev/?a=b', Enabled: true },
    ]);
  });
});
