import { snapToLines } from './variable-input';

describe('snapToLines', () => {
  it('rounds a height to whole lines', () => {
    // One line is 20px of text plus 9px of padding above and below.
    expect(snapToLines(30)).toBe(38);
    expect(snapToLines(50)).toBe(58);
    expect(snapToLines(97)).toBe(98);
  });

  it('never goes below one line', () => {
    expect(snapToLines(0)).toBe(38);
  });
});
