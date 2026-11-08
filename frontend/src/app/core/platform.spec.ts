const state = vi.hoisted(() => ({
  fullscreen: false,
  listeners: new Map<string, () => void>(),
}));

vi.mock('@wailsio/runtime', () => ({
  Call: {
    ByID: () => Promise.resolve('darwin'),
  },
  Window: {
    IsFullscreen: () => Promise.resolve(state.fullscreen),
  },
  Events: {
    On: (name: string, callback: () => void) => {
      state.listeners.set(name, callback);
      return () => state.listeners.delete(name);
    },
  },
}));

import { detectPlatform } from './platform';

describe('detectPlatform', () => {
  beforeEach(() => {
    state.fullscreen = false;
    state.listeners.clear();
    delete document.documentElement.dataset['platform'];
    delete document.documentElement.dataset['fullscreen'];
  });

  it('exposes the host platform', async () => {
    await detectPlatform();

    expect(document.documentElement.dataset['platform']).toBe('darwin');
    expect(document.documentElement.dataset['fullscreen']).toBe('false');
  });

  it('follows the fullscreen events of the window', async () => {
    state.fullscreen = true;
    await detectPlatform();

    expect(document.documentElement.dataset['fullscreen']).toBe('true');

    state.listeners.get('common:WindowUnFullscreen')?.();
    expect(document.documentElement.dataset['fullscreen']).toBe('false');

    state.listeners.get('common:WindowFullscreen')?.();
    expect(document.documentElement.dataset['fullscreen']).toBe('true');
  });
});
