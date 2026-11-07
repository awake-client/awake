import { slugify } from './slug';

describe('slugify', () => {
  it.each([
    ['Production', 'production'],
    ['Pré-prod EU', 'pre-prod-eu'],
    ['  Staging (v2) / Paris  ', 'staging-v2-paris'],
    ['Développement local', 'developpement-local'],
    ['***', ''],
  ])('turns %s into %s', (name, expected) => {
    expect(slugify(name)).toBe(expected);
  });
});
