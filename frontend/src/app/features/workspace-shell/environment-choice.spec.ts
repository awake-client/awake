import { environmentOptions, NO_ENVIRONMENT, type EnvironmentChoice } from './environment-choice';

function choice(name: string, fileName: string, scopes: string[] = []): EnvironmentChoice {
  return {
    FileName: fileName,
    Name: name,
    HasLocalOverrides: false,
    Private: false,
    CollectionOnly: scopes.length > 0,
    Color: '',
    Scopes: scopes,
  };
}

describe('environmentOptions', () => {
  const environments = [
    choice('Production', 'production', ['collections/billing']),
    choice('Development', 'development'),
    choice('Staging', 'staging'),
    choice('Sandbox', 'sandbox', ['collections/support']),
  ];

  it('offers "No environment" first', () => {
    const options = environmentOptions(environments, [], '', 'ascending');

    expect(options[0]).toBe(NO_ENVIRONMENT);
  });

  it('sorts environments by display name in both directions', () => {
    const ascending = environmentOptions(environments, [], '', 'ascending').map(
      (option) => option.Name,
    );
    const descending = environmentOptions(environments, [], '', 'descending').map(
      (option) => option.Name,
    );

    expect(ascending).toEqual([
      'No environment',
      'Development',
      'Production',
      'Sandbox',
      'Staging',
    ]);
    expect(descending).toEqual([
      'No environment',
      'Staging',
      'Sandbox',
      'Production',
      'Development',
    ]);
  });

  it('offers pinned environments first, then the others', () => {
    const names = environmentOptions(environments, ['staging'], '', 'ascending').map(
      (option) => option.Name,
    );

    expect(names).toEqual(['No environment', 'Staging', 'Development', 'Production', 'Sandbox']);
  });

  it('filters by display name, ignoring case', () => {
    const names = environmentOptions(environments, [], 'PROD', 'ascending').map(
      (option) => option.Name,
    );

    expect(names).toEqual(['Production']);
  });

  it('filters by file name and scope', () => {
    expect(
      environmentOptions(environments, [], 'sandbox', 'ascending').map((option) => option.Name),
    ).toEqual(['Sandbox']);
    expect(
      environmentOptions(environments, [], 'support', 'ascending').map((option) => option.Name),
    ).toEqual(['Sandbox']);
  });

  it('hides "No environment" when it does not match the search', () => {
    const names = environmentOptions(environments, [], 'staging', 'ascending').map(
      (option) => option.Name,
    );

    expect(names).toEqual(['Staging']);
  });

  it('keeps pinned environments of a search together', () => {
    const names = environmentOptions(environments, ['production'], 'd', 'ascending').map(
      (option) => option.Name,
    );

    // Production is pinned, so it comes before the other matches.
    expect(names).toEqual(['Production', 'Development', 'Sandbox']);
  });
});
