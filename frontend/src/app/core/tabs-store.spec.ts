import { TestBed } from '@angular/core/testing';
import { Router } from '@angular/router';
import { Subject } from 'rxjs';
import { RequestTab, TabsStore } from './tabs-store';

function tab(path: string): RequestTab {
  return { path, name: path, method: 'GET', protocol: 'http', preview: false, dirty: false };
}

describe('TabsStore.closeMany', () => {
  let store: TabsStore;
  let navigate: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    navigate = vi.fn().mockResolvedValue(true);
    TestBed.configureTestingModule({
      providers: [{ provide: Router, useValue: { navigate, events: new Subject() } }],
    });
    store = TestBed.inject(TabsStore);
    store.tabs.set(['a', 'b', 'c', 'd', 'e'].map((path) => tab(`collections/x/${path}.yaml`)));
  });

  it('keeps the active tab when it stays open', () => {
    store.active.set('collections/x/c.yaml');
    store.closeMany(['collections/x/a.yaml', 'collections/x/b.yaml']);
    expect(store.tabs().map((entry) => entry.path)).toEqual([
      'collections/x/c.yaml',
      'collections/x/d.yaml',
      'collections/x/e.yaml',
    ]);
    expect(navigate).not.toHaveBeenCalled();
  });

  it('shows the nearest tab to the right, then to the left, when the active one closes', () => {
    store.active.set('collections/x/c.yaml');
    store.closeMany(['collections/x/c.yaml', 'collections/x/d.yaml']);
    expect(navigate).toHaveBeenLastCalledWith(['/workspace/request', 'collections', 'x', 'e.yaml']);

    store.active.set('collections/x/e.yaml');
    store.closeMany(['collections/x/e.yaml']);
    expect(navigate).toHaveBeenLastCalledWith(['/workspace/request', 'collections', 'x', 'b.yaml']);
  });

  it('goes back to the overview when every tab closes', () => {
    store.active.set('collections/x/a.yaml');
    store.closeMany(store.tabs().map((entry) => entry.path));
    expect(store.tabs()).toEqual([]);
    expect(navigate).toHaveBeenLastCalledWith(['/workspace']);
  });
});
