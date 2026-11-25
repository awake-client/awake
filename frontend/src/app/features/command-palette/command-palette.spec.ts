import { matchesTerms } from './command-palette';

describe('matchesTerms', () => {
  it('needs every term, in any order and case', () => {
    expect(matchesTerms('AWS: refresh token AWS SigV4', ['token', 'aws'])).toBe(true);
    expect(matchesTerms('AWS: refresh token', ['token', 'gcp'])).toBe(false);
  });

  it('matches everything without terms', () => {
    expect(matchesTerms('anything', [])).toBe(true);
  });
});
