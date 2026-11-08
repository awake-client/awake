import { TestBed } from '@angular/core/testing';
import { LayoutStore, URL_HEIGHT } from './layout-store';

describe('LayoutStore URL height', () => {
  beforeEach(() => localStorage.clear());

  it('fits the URL field to its content by default', () => {
    expect(TestBed.inject(LayoutStore).urlHeight()).toBeNull();
  });

  it('keeps a chosen height within its bounds', () => {
    const store = TestBed.inject(LayoutStore);

    store.setUrlHeight(10);
    expect(store.urlHeight()).toBe(URL_HEIGHT.minimum);
    store.setUrlHeight(10_000);
    expect(store.urlHeight()).toBe(URL_HEIGHT.maximum);
  });

  it('remembers the chosen height and forgets it on reset', () => {
    const store = TestBed.inject(LayoutStore);

    store.setUrlHeight(120);
    expect(JSON.parse(localStorage.getItem('awake.layout') ?? '{}').urlHeight).toBe(120);

    store.resetUrlHeight();
    expect(store.urlHeight()).toBeNull();
    expect(JSON.parse(localStorage.getItem('awake.layout') ?? '{}').urlHeight).toBeNull();
  });
});
