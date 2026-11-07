import { formatVersion } from './version-label';

describe('formatVersion', () => {
  it.each([
    ['0.0.4', 'v0.0.4'],
    ['v0.0.4', 'v0.0.4'],
    ['1.2.3', 'v1.2.3'],
    ['dev', 'dev'],
    ['', 'dev'],
    [null, 'dev'],
    [undefined, 'dev'],
  ])('turns %s into %s', (version, expected) => {
    expect(formatVersion(version)).toBe(expected);
  });
});
