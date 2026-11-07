import { ScopedSummary } from '@bindings/github.com/clementherve/awake/src/features/environments/models';
import { EnvironmentChoice } from './environment-picker';

/** Routes showing a request or a collection, whose environments are offered. */
const REQUEST_ROUTE = '/workspace/request/';
const COLLECTION_ROUTE = '/workspace/collection/';
const COLLECTION_ROOTS = ['collections', 'local'];

/**
 * Collection scope of the page on screen: the collection of a request, or the
 * collection whose settings are open. Other pages return an empty string.
 */
export function collectionScopeOf(url: string): string {
  const path = url.split(/[?#]/)[0];
  const prefix = path.startsWith(REQUEST_ROUTE)
    ? REQUEST_ROUTE
    : path.startsWith(COLLECTION_ROUTE)
      ? COLLECTION_ROUTE
      : '';
  if (!prefix) {
    return '';
  }

  const segments = decodeURIComponent(path.slice(prefix.length)).split('/');
  if (segments.length < 2) {
    return '';
  }
  const [root, name] = segments;
  if (!COLLECTION_ROOTS.includes(root) || !name) {
    return '';
  }
  return `${root}/${name}`;
}

/**
 * Environments offered in a context: the global ones, then those of its
 * collection. An environment of the same name in both scopes is offered once,
 * since the collection refines the global one.
 */
export function visibleEnvironments(scoped: ScopedSummary[], scope: string): EnvironmentChoice[] {
  const options = new Map<string, EnvironmentChoice>();
  for (const summary of scoped) {
    if (summary.Scope !== '' && summary.Scope !== scope) {
      continue;
    }

    const existing = options.get(summary.FileName);
    const scopes = [...(existing?.Scopes ?? []), summary.Scope].sort();
    if (!existing || (existing.CollectionOnly && summary.Scope === '')) {
      options.set(summary.FileName, {
        FileName: summary.FileName,
        Name: summary.Name,
        HasLocalOverrides: summary.HasLocalOverrides || !!existing?.HasLocalOverrides,
        CollectionOnly: summary.Scope !== '',
        Color: summary.Color || existing?.Color || '',
        Scopes: scopes,
      });
    } else {
      existing.Scopes = scopes;
      if (summary.HasLocalOverrides) {
        existing.HasLocalOverrides = true;
      }
    }
  }
  return [...options.values()];
}
