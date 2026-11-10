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

  it('brings every hidden collection back at once', () => {
    const store = new HiddenCollectionsStore();
    store.useWorkspace(WORKSPACE);

    store.hide('collections/billing');
    store.hide('local/notes');
    expect(store.hiddenPaths()).toEqual(['collections/billing', 'local/notes']);

    store.unhideAll();
    expect(store.hiddenPaths()).toEqual([]);
  });

  it('recognizes the entries inside a hidden collection', () => {
    const store = new HiddenCollectionsStore();
    store.useWorkspace(WORKSPACE);
    store.hide('collections/billing');

    expect(store.isInHidden('collections/billing')).toBe(true);
    expect(store.isInHidden('collections/billing/invoices/list.yaml')).toBe(true);
    expect(store.isInHidden('collections/billing-old')).toBe(false);
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
