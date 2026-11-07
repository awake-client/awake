/** A :name segment of a URL, after a slash so a scheme or a port is not one. */
const PATH_PARAMETER_PATTERN = /\/:([A-Za-z_][A-Za-z0-9_]*)/g;

/** The :name segments of a URL, in order and once each, as the backend reads them. */
export function pathParameterNames(url: string): string[] {
  return [...new Set([...url.matchAll(PATH_PARAMETER_PATTERN)].map((match) => match[1]))];
}
