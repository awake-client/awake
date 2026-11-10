import { ScopedSummary } from '@bindings/github.com/awake-client/awake/src/features/environments/models';
import { collectionScopeOf, visibleEnvironments } from './environment-scope';

function summary(
  fileName: string,
  scope: string,
  local = false,
  privateEnv = false,
): ScopedSummary {
  return {
    FileName: fileName,
    Name: fileName,
    Scope: scope,
    HasLocalOverrides: local,
    Private: privateEnv,
    Color: '',
  };
}

describe('collectionScopeOf', () => {
  it('reads the collection of an open request', () => {
    expect(
      collectionScopeOf('/workspace/request/collections/billing/invoices/list%20invoices.yaml'),
    ).toBe('collections/billing/invoices');
  });

  it('reads the folder of a request inside a folder', () => {
    expect(
      collectionScopeOf('/workspace/request/collections/billing/requests/list%20invoices.yaml'),
    ).toBe('collections/billing/requests');
  });

  it('reads the collection whose settings are open', () => {
    expect(collectionScopeOf('/workspace/collection/local/scratch?tab=variables')).toBe(
      'local/scratch',
    );
  });

  it('reads the folder whose settings are open', () => {
    expect(collectionScopeOf('/workspace/collection/collections/platform/users')).toBe(
      'collections/platform/users',
    );
  });

  it('ignores pages that show no collection', () => {
    expect(collectionScopeOf('/workspace')).toBe('');
    expect(collectionScopeOf('/workspace/git')).toBe('');
    expect(collectionScopeOf('/workspace/environments')).toBe('');
  });
});

describe('visibleEnvironments', () => {
  const scoped = [
    summary('development', ''),
    summary('staging', ''),
    summary('development', 'collections/billing', true),
    summary('production', 'collections/billing'),
    summary('sandbox', 'collections/support'),
  ];

  it('offers the global environments and those of the open collection only', () => {
    const names = visibleEnvironments(scoped, 'collections/billing').map(
      (option) => option.FileName,
    );

    expect(names).toEqual(['development', 'staging', 'production']);
  });

  it('offers a folder environment with those of its collection and the global ones', () => {
    const scoped = [
      summary('development', ''),
      summary('production', 'collections/billing'),
      summary('users', 'collections/billing/invoices'),
      summary('sandbox', 'collections/support'),
    ];

    const names = visibleEnvironments(scoped, 'collections/billing/invoices').map(
      (option) => option.FileName,
    );

    expect(names).toEqual(['development', 'production', 'users']);
  });

  it('offers the global environments outside a collection', () => {
    const names = visibleEnvironments(scoped, '').map((option) => option.FileName);

    expect(names).toEqual(['development', 'staging']);
  });

  it('offers an environment defined in both scopes once, as global', () => {
    const development = visibleEnvironments(scoped, 'collections/billing').find(
      (option) => option.FileName === 'development',
    );

    expect(development?.CollectionOnly).toBe(false);
    expect(development?.HasLocalOverrides).toBe(true);
  });

  it('marks an environment defined by the collection alone', () => {
    const production = visibleEnvironments(scoped, 'collections/billing').find(
      (option) => option.FileName === 'production',
    );

    expect(production?.CollectionOnly).toBe(true);
  });

  it('offers a private environment living in a local file only', () => {
    const options = visibleEnvironments([summary('scratch', '', false, true)], '');

    expect(options.map((option) => option.FileName)).toEqual(['scratch']);
    expect(options[0].Private).toBe(true);
    expect(options[0].HasLocalOverrides).toBe(false);
  });

  it('lists every scope defining an environment', () => {
    const options = visibleEnvironments(
      [summary('development', ''), summary('development', 'collections/billing')],
      'collections/billing',
    );
    expect(options[0].Scopes).toEqual(['', 'collections/billing']);
  });
});
