import { TestBed } from '@angular/core/testing';
import { LayoutStore, URL_HEIGHT } from './layout-store';

describe('LayoutStore URL height', () => {
  beforeEach(() => localStorage.clear());

  it('fits the URL field to its content by default', () => {
    expect(TestBed.inject(LayoutStore).urlHeight()).toBeNull();
  });

  it('keeps a chosen height on a whole line', () => {
    const store = TestBed.inject(LayoutStore);

    // A height in the middle of a line, such as the one an earlier version
    // could save, would cut the line in half.
    store.setUrlHeight(47);
    expect(store.urlHeight()).toBe(URL_HEIGHT.minimum);
    store.setUrlHeight(50);
    expect(store.urlHeight()).toBe(URL_HEIGHT.minimum + 20);
    store.setUrlHeight(10_000);
    expect(store.urlHeight()).toBe(URL_HEIGHT.maximum);
  });

  it('reads a height saved before line snapping on a whole line', () => {
    localStorage.setItem('awake.layout', JSON.stringify({ urlHeight: 47 }));

    expect(TestBed.inject(LayoutStore).urlHeight()).toBe(URL_HEIGHT.minimum);
  });

  it('remembers the chosen height and forgets it on reset', () => {
    const store = TestBed.inject(LayoutStore);

    store.setUrlHeight(120);
    expect(JSON.parse(localStorage.getItem('awake.layout') ?? '{}').urlHeight).toBe(118);

    store.resetUrlHeight();
    expect(store.urlHeight()).toBeNull();
    expect(JSON.parse(localStorage.getItem('awake.layout') ?? '{}').urlHeight).toBeNull();
  });
});
