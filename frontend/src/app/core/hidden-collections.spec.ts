import { HiddenCollectionsStore } from './hidden-collections';

const WORKSPACE = '/workspace';

describe('HiddenCollectionsStore', () => {
  beforeEach(() => localStorage.clear());

  it('starts without hidden collections', () => {
    const store = new HiddenCollectionsStore();
    store.useWorkspace(WORKSPACE);

    expect(store.hiddenPaths()).toEqual([]);
  });

  it('hides and unhides a collection of the open workspace', () => {
    const store = new HiddenCollectionsStore();
    store.useWorkspace(WORKSPACE);

    store.hide('collections/billing');
    expect(store.isHidden('collections/billing')).toBe(true);
    expect(store.hiddenPaths()).toEqual(['collections/billing']);

    store.unhide('collections/billing');
    expect(store.isHidden('collections/billing')).toBe(false);
  });

  it('keeps the hidden collections after a restart', () => {
    const first = new HiddenCollectionsStore();
    first.useWorkspace(WORKSPACE);
    first.hide('collections/billing');

    const second = new HiddenCollectionsStore();
    second.useWorkspace(WORKSPACE);

    expect(second.isHidden('collections/billing')).toBe(true);
  });

  it('keeps the hidden collections of each workspace apart', () => {
    const store = new HiddenCollectionsStore();
    store.useWorkspace('/workspace-a');
    store.hide('collections/billing');

    store.useWorkspace('/workspace-b');
    expect(store.isHidden('collections/billing')).toBe(false);

    store.useWorkspace('/workspace-a');
    expect(store.isHidden('collections/billing')).toBe(true);
  });
});
