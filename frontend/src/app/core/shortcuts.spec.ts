import { Shortcuts } from './shortcuts';

describe('Shortcuts', () => {
  afterEach(() => delete document.documentElement.dataset['platform']);

  it('shows the Apple key on macOS', () => {
    document.documentElement.dataset['platform'] = 'darwin';
    const shortcuts = new Shortcuts();

    expect(shortcuts.command).toBe('⌘');
    expect(shortcuts.key('P')).toBe('⌘P');
    expect(shortcuts.key(shortcuts.enter)).toBe('⌘↵');
  });

  it('shows Control on Windows and Linux', () => {
    for (const platform of ['windows', 'linux']) {
      document.documentElement.dataset['platform'] = platform;
      const shortcuts = new Shortcuts();

      expect(shortcuts.command).toBe('Ctrl');
      expect(shortcuts.key('P')).toBe('Ctrl+P');
      expect(shortcuts.key(shortcuts.enter)).toBe('Ctrl+↵');
    }
  });
});
