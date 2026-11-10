import { ScopedSummary } from '@bindings/github.com/awake-client/awake/src/features/environments/models';
import type { EnvironmentChoice } from './environment-choice';

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
  const [root, name, folder] = segments;
  if (!COLLECTION_ROOTS.includes(root) || !name) {
    return '';
  }
  const collection = `${root}/${name}`;
  if (folder && !folder.endsWith('.yaml')) {
    const folderScope = `${collection}/${folder}`;
    if (folderScope.split('/').length >= 3) {
      return folderScope;
    }
  }
  return collection;
}

/**
 * Environment scopes visible from a page, from the weakest to the strongest:
 * the global one, then the collection, then its folder. A folder scope keeps
 * the environments of its collection, which its requests still inherit.
 */
export function visibleScopesOf(scope: string): string[] {
  if (!scope) {
    return [''];
  }
  const segments = scope.split('/');
  if (segments.length > 2) {
    return ['', segments.slice(0, 2).join('/'), scope];
  }
  return ['', scope];
}

/**
 * Environments offered in a context: the global ones, then those of its
 * collection and of its folder. An environment of the same name in several
 * scopes is offered once, since the closest scope refines the others.
 */
export function visibleEnvironments(scoped: ScopedSummary[], scope: string): EnvironmentChoice[] {
  const visibleScopes = new Set(visibleScopesOf(scope));
  const options = new Map<string, EnvironmentChoice>();
  for (const summary of scoped) {
    if (summary.Scope !== '' && !visibleScopes.has(summary.Scope)) {
      continue;
    }

    const existing = options.get(summary.FileName);
    const scopes = [...(existing?.Scopes ?? []), summary.Scope].sort();
    if (!existing || (existing.CollectionOnly && summary.Scope === '')) {
      options.set(summary.FileName, {
        FileName: summary.FileName,
        Name: summary.Name,
        HasLocalOverrides: summary.HasLocalOverrides || !!existing?.HasLocalOverrides,
        Private: summary.Private || !!existing?.Private,
        CollectionOnly: summary.Scope !== '',
        Color: summary.Color || existing?.Color || '',
        Scopes: scopes,
      });
    } else {
      existing.Scopes = scopes;
      if (summary.HasLocalOverrides) {
        existing.HasLocalOverrides = true;
      }
      if (summary.Private) {
        existing.Private = true;
      }
    }
  }
  return [...options.values()];
}
