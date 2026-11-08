import { Parameter } from '@bindings/github.com/clementherve/awake/src/domain/models';

/** A query value written as a whole :name placeholder, kept in the URL. */
const PLACEHOLDER_VALUE = /^:[A-Za-z_][A-Za-z0-9_]*$/;

/** Characters that would end an entry or its value, escaped in the URL text. */
const NAME_ESCAPED = /[%&#=+]/g;
/** A value ends at &, # or a plus; an equals sign is safe after the name's. */
const VALUE_ESCAPED = /[%&#+]/g;

export interface ParsedUrlQuery {
  /** The URL without the entries that belong in the parameters list. */
  url: string;
  /** The query entries to edit as parameters, in URL order. */
  parameters: Parameter[];
}

/**
 * Splits the query of a URL into the entries to edit as parameters. Entries
 * written as a whole :name value are left in the URL: they are path
 * parameters, not query parameters. The fragment stays where it is.
 */
export function parseUrlQuery(raw: string): ParsedUrlQuery {
  const fragmentStart = raw.indexOf('#');
  const fragment = fragmentStart >= 0 ? raw.slice(fragmentStart) : '';
  const withoutFragment = fragmentStart >= 0 ? raw.slice(0, fragmentStart) : raw;
  const queryStart = withoutFragment.indexOf('?');
  const path = queryStart >= 0 ? withoutFragment.slice(0, queryStart) : withoutFragment;

  const kept: string[] = [];
  const parameters: Parameter[] = [];
  if (queryStart >= 0) {
    for (const entry of withoutFragment.slice(queryStart + 1).split('&')) {
      if (entry === '') {
        continue;
      }
      const equals = entry.indexOf('=');
      const rawName = equals >= 0 ? entry.slice(0, equals) : entry;
      const rawValue = equals >= 0 ? entry.slice(equals + 1) : '';
      if (PLACEHOLDER_VALUE.test(rawValue)) {
        kept.push(entry);
        continue;
      }
      parameters.push({
        Name: decodeQueryText(rawName),
        Value: decodeQueryText(rawValue),
        Enabled: true,
      });
    }
  }

  return {
    url: path + (kept.length > 0 ? '?' + kept.join('&') : '') + fragment,
    parameters,
  };
}

/**
 * Writes the enabled parameters of a request into the query of its URL, so
 * the field shows the URL that is sent. The URL keeps its own query, such as
 * the :name placeholders of path parameters, and its fragment.
 */
export function composeUrlQuery(url: string, parameters: readonly Parameter[]): string {
  const fragmentStart = url.indexOf('#');
  const fragment = fragmentStart >= 0 ? url.slice(fragmentStart) : '';
  const withoutFragment = fragmentStart >= 0 ? url.slice(0, fragmentStart) : url;
  const queryStart = withoutFragment.indexOf('?');
  const path = queryStart >= 0 ? withoutFragment.slice(0, queryStart) : withoutFragment;
  const kept = queryStart >= 0 ? withoutFragment.slice(queryStart + 1) : '';

  const entries = parameters
    .filter((parameter) => parameter.Enabled && parameter.Name.trim() !== '')
    .map(
      (parameter) =>
        `${escapeQueryText(parameter.Name, NAME_ESCAPED)}=${escapeQueryText(parameter.Value, VALUE_ESCAPED, true)}`,
    );

  const query = [kept, ...entries].filter((part) => part !== '').join('&');
  return path + (query !== '' ? '?' + query : '') + fragment;
}

/**
 * Adds the parameters read from a URL to the ones stored with the request,
 * dropping the stored copies of what the URL already holds. The stored
 * entries the URL does not write, such as disabled ones, are kept.
 */
export function mergeUrlParameters(
  fromUrl: readonly Parameter[],
  stored: readonly Parameter[],
): Parameter[] {
  const present = new Set(fromUrl.map(parameterKey));
  return [
    ...fromUrl.map((parameter) => ({ ...parameter })),
    ...stored.filter((parameter) => !present.has(parameterKey(parameter))),
  ];
}

function parameterKey(parameter: Parameter): string {
  return `${parameter.Name}\u0000${parameter.Value}`;
}

function escapeQueryText(text: string, escaped: RegExp, placeholderSafe = false): string {
  const encoded = text.replace(
    escaped,
    (character) => `%${character.charCodeAt(0).toString(16).toUpperCase()}`,
  );
  // A value that would read back as a path parameter escapes its colon.
  return placeholderSafe && PLACEHOLDER_VALUE.test(text) ? `%3A${encoded.slice(1)}` : encoded;
}

function decodeQueryText(text: string): string {
  try {
    // A plus stands for a space in a query, as it does for the Go client.
    return decodeURIComponent(text.replace(/\+/g, ' '));
  } catch {
    return text;
  }
}
