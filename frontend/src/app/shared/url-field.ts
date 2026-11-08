/**
 * Geometry of the request URL field. Its height is a whole number of lines,
 * so the field never truncates a line in half. Shared by the field, which
 * measures and draws it, and by the layout store, which remembers its height.
 */
export const URL_FIELD = {
  /** Height of one line of text and of the padding above and below it. */
  lineHeight: 20,
  padding: 9,
  /** Enough to read a long URL without taking over the editor. */
  maximumLines: 12,
};

/** Height of the field when it shows one line. */
export const URL_FIELD_ONE_LINE = URL_FIELD.lineHeight + 2 * URL_FIELD.padding;

/** Height of the field at its maximum number of lines. */
export const URL_FIELD_MAXIMUM =
  URL_FIELD_ONE_LINE + (URL_FIELD.maximumLines - 1) * URL_FIELD.lineHeight;

/** Rounds a height to the nearest number of whole lines of the field. */
export function snapUrlLines(height: number): number {
  const lines = Math.max(1, Math.round((height - URL_FIELD_ONE_LINE) / URL_FIELD.lineHeight) + 1);
  return Math.min(URL_FIELD_ONE_LINE + (lines - 1) * URL_FIELD.lineHeight, URL_FIELD_MAXIMUM);
}
