import { EnvironmentStore } from './environment-store';

const WORKSPACE = '/workspace';

describe('EnvironmentStore', () => {
  beforeEach(() => localStorage.clear());

  it('keeps the environment chosen in each collection', () => {
    const store = new EnvironmentStore();
    store.useWorkspace(WORKSPACE);
    store.select('development');

    store.useScope('collections/billing');
    store.select('prodA');
    store.useScope('collections/support');
    store.select('dev4');

    store.useScope('collections/billing');
    expect(store.selected()).toBe('prodA');
    store.useScope('collections/support');
    expect(store.selected()).toBe('dev4');
    store.useScope('');
    expect(store.selected()).toBe('development');
  });

  it('starts a collection never chosen in with the global choice', () => {
    const store = new EnvironmentStore();
    store.useWorkspace(WORKSPACE);
    store.select('development');

    store.useScope('collections/billing');
    expect(store.selected()).toBe('development');
  });

  it('starts a folder never chosen in with the choice of its collection', () => {
    const store = new EnvironmentStore();
    store.useWorkspace(WORKSPACE);
    store.useScope('collections/billing');
    store.select('prodA');

    store.useScope('collections/billing/invoices');
    expect(store.selected()).toBe('prodA');

    // A folder keeps its own choice once made.
    store.select('sandbox');
    store.useScope('collections/billing');
    store.useScope('collections/billing/invoices');
    expect(store.selected()).toBe('sandbox');
  });

  it('keeps an explicit "no environment" chooser per collection', () => {
    const store = new EnvironmentStore();
    store.useWorkspace(WORKSPACE);
    store.select('development');

    store.useScope('collections/billing');
    store.select('');
    store.useScope('collections/support');
    expect(store.selected()).toBe('development');

    store.useScope('collections/billing');
    expect(store.selected()).toBe('');
  });

  it('restores the choices of a workspace after a restart', () => {
    const first = new EnvironmentStore();
    first.useWorkspace(WORKSPACE);
    first.useScope('collections/billing');
    first.select('prodA');

    const second = new EnvironmentStore();
    second.useWorkspace(WORKSPACE);
    second.useScope('collections/billing');

    expect(second.selected()).toBe('prodA');
  });

  it('reads the choice stored by an older version', () => {
    localStorage.setItem('awake.environments', JSON.stringify({ [WORKSPACE]: 'staging' }));

    const store = new EnvironmentStore();
    store.useWorkspace(WORKSPACE);

    expect(store.selected()).toBe('staging');
    store.useScope('collections/billing');
    expect(store.selected()).toBe('staging');
  });

  it('remembers pinned environments per workspace', () => {
    const store = new EnvironmentStore();
    store.useWorkspace(WORKSPACE);
    store.togglePin('production');
    store.togglePin('staging');

    expect(store.pinned()).toEqual(['production', 'staging']);

    const second = new EnvironmentStore();
    second.useWorkspace(WORKSPACE);
    expect(second.pinned()).toEqual(['production', 'staging']);

    second.togglePin('production');
    expect(second.pinned()).toEqual(['staging']);
  });

  it('keeps the pins of each workspace separate', () => {
    const store = new EnvironmentStore();
    store.useWorkspace(WORKSPACE);
    store.togglePin('production');

    store.useWorkspace('/other');

    expect(store.pinned()).toEqual([]);
  });
});
