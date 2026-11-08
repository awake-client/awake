import { snapUrlLines, URL_FIELD_ONE_LINE } from './url-field';

describe('snapUrlLines', () => {
  it('rounds a height to whole lines', () => {
    // One line is 20px of text plus 9px of padding above and below.
    expect(URL_FIELD_ONE_LINE).toBe(38);
    expect(snapUrlLines(30)).toBe(38);
    expect(snapUrlLines(50)).toBe(58);
    expect(snapUrlLines(97)).toBe(98);
  });

  it('never goes below one line', () => {
    expect(snapUrlLines(0)).toBe(38);
  });

  it('keeps a height saved before line snapping from cutting a line', () => {
    // The field used to accept any pixel height, such as 47.
    expect(snapUrlLines(47)).toBe(38);
  });
});
