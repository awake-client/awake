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
});
