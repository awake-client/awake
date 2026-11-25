import { parseViewMessage, resolvedTheme } from './plugin-frame-protocol';

describe('parseViewMessage', () => {
  it('reads ready and invoke messages', () => {
    expect(parseViewMessage({ type: 'awake:ready' })).toEqual({ type: 'awake:ready' });
    expect(
      parseViewMessage({ type: 'awake:invoke', id: 3, handler: 'decode', payload: { a: 1 } }),
    ).toEqual({
      type: 'awake:invoke',
      id: 3,
      handler: 'decode',
      payload: { a: 1 },
    });
  });

  it('defaults a missing payload to null', () => {
    expect(parseViewMessage({ type: 'awake:invoke', id: 1, handler: 'run' })).toEqual({
      type: 'awake:invoke',
      id: 1,
      handler: 'run',
      payload: null,
    });
  });

  it('rejects anything else', () => {
    const rejected: unknown[] = [
      null,
      'awake:ready',
      { type: 'awake:navigate', url: 'https://example.com' },
      { type: 'awake:invoke', id: '1', handler: 'run' },
      { type: 'awake:invoke', id: 1.5, handler: 'run' },
      { type: 'awake:invoke', id: 1, handler: '../escape' },
      { type: 'awake:invoke', id: 1, handler: 'a'.repeat(65) },
      { type: 'awake:invoke', id: 1 },
    ];
    for (const message of rejected) {
      expect(parseViewMessage(message)).toBeNull();
    }
  });
});

describe('resolvedTheme', () => {
  it('follows the system only when asked to', () => {
    expect(resolvedTheme('system', true)).toBe('dark');
    expect(resolvedTheme('system', false)).toBe('light');
    expect(resolvedTheme('light', true)).toBe('light');
  });
});
