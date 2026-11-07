/**
 * A :name in a URL, filled from the path parameters of the request: a path
 * segment written /:name, or a whole query value written name=:name. The
 * colon must follow a slash or an equals sign, so a scheme, a port or a time
 * such as 12:30 is never read as one. Mirrors domain.URLPlaceholders.
 */
export interface UrlPlaceholder {
  name: string;
  inQuery: boolean;
  /** Bounds of the colon and the name in the URL. */
  start: number;
  end: number;
}

const PATH_PLACEHOLDER = /\/(:[A-Za-z_][A-Za-z0-9_]*)/g;
const QUERY_PLACEHOLDER = /=(:[A-Za-z_][A-Za-z0-9_]*)/g;

export function urlPlaceholders(url: string): UrlPlaceholder[] {
  const queryStart = url.indexOf('?');
  const pathPart = queryStart >= 0 ? url.slice(0, queryStart) : url;
  const placeholders: UrlPlaceholder[] = [];

  for (const match of pathPart.matchAll(PATH_PLACEHOLDER)) {
    const start = match.index + 1;
    placeholders.push({
      name: match[1].slice(1),
      inQuery: false,
      start,
      end: start + match[1].length,
    });
  }
  if (queryStart < 0) {
    return placeholders;
  }

  const query = url.slice(queryStart);
  for (const match of query.matchAll(QUERY_PLACEHOLDER)) {
    const end = match.index + match[0].length;
    if (end < query.length && query[end] !== '&' && query[end] !== '#') {
      continue;
    }
    const start = queryStart + match.index + 1;
    placeholders.push({
      name: match[1].slice(1),
      inQuery: true,
      start,
      end: start + match[1].length,
    });
  }
  return placeholders;
}

/** The placeholder names of a URL, in order and once each. */
export function pathParameterNames(url: string): string[] {
  return [...new Set(urlPlaceholders(url).map((placeholder) => placeholder.name))];
}

/** Replaces each placeholder, colon included, with what resolve returns. */
export function replaceUrlPlaceholders(
  url: string,
  resolve: (placeholder: UrlPlaceholder) => string,
): string {
  let result = '';
  let cursor = 0;
  for (const placeholder of urlPlaceholders(url)) {
    result += url.slice(cursor, placeholder.start) + resolve(placeholder);
    cursor = placeholder.end;
  }
  return result + url.slice(cursor);
}
