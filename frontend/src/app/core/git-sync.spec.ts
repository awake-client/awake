import { parsePeriod } from './git-sync';

describe('parsePeriod', () => {
  it('reads Go durations', () => {
    expect(parsePeriod('90s')).toBe(90_000);
    expect(parsePeriod('5m')).toBe(300_000);
    expect(parsePeriod('1h30m')).toBe(5_400_000);
  });

  it('falls back to five minutes for empty or invalid values', () => {
    expect(parsePeriod('')).toBe(300_000);
    expect(parsePeriod('soon')).toBe(300_000);
    expect(parsePeriod('5 minutes')).toBe(300_000);
  });
});
