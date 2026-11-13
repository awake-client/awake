import { describeExpiry, formatDuration } from './expiry';

describe('formatDuration', () => {
  it('keeps the two largest non-zero units', () => {
    expect(formatDuration(0)).toBe('0 s');
    expect(formatDuration(42_000)).toBe('42 s');
    expect(formatDuration(272_000)).toBe('4 min 32 s');
    expect(formatDuration(7_500_000)).toBe('2 h 5 min');
    expect(formatDuration(3_600_000)).toBe('1 h');
    expect(formatDuration(90_000_000)).toBe('1 d 1 h');
  });
});

describe('describeExpiry', () => {
  const now = 1_000_000_000_000;

  it('reports the remaining time and flags tokens about to expire', () => {
    expect(describeExpiry(now + 272_000, now)).toMatchObject({
      state: 'valid',
      label: 'Expires in 4 min 32 s',
    });
    expect(describeExpiry(now + 30_000, now).state).toBe('expiring');
  });

  it('reports expired tokens and tokens without expiry', () => {
    expect(describeExpiry(now - 180_000, now)).toMatchObject({
      state: 'expired',
      label: 'Expired 3 min ago',
    });
    expect(describeExpiry(0, now)).toEqual({ state: 'none', label: 'No expiry given', at: '' });
  });
});
