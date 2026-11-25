import { matchesContentType, mediaTypeOf } from './plugin-contributions';

describe('mediaTypeOf', () => {
  it('drops the parameters and the case', () => {
    expect(mediaTypeOf('Application/JSON; charset=utf-8')).toBe('application/json');
    expect(mediaTypeOf('')).toBe('');
  });
});

describe('matchesContentType', () => {
  it('matches exact media types', () => {
    expect(matchesContentType('application/x-protobuf', ['application/x-protobuf'])).toBe(true);
    expect(matchesContentType('application/json', ['application/x-protobuf'])).toBe(false);
  });

  it('matches wildcards', () => {
    expect(matchesContentType('image/png', ['image/*'])).toBe(true);
    expect(matchesContentType('imagery/png', ['image/*'])).toBe(false);
    expect(matchesContentType('text/csv', ['*'])).toBe(true);
    expect(matchesContentType('text/csv', ['*/*'])).toBe(true);
  });

  it('never matches a response without a content type', () => {
    expect(matchesContentType('', ['*'])).toBe(false);
  });
});
