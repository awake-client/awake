/** An environment offered by the picker. */
export interface EnvironmentChoice {
  FileName: string;
  Name: string;
  HasLocalOverrides: boolean;
  /** True when the environment lives in a git ignored local file only. */
  Private: boolean;
  CollectionOnly: boolean;
  Color: string;
  /** Scopes defining it: '' for the global one, then the collection path. */
  Scopes: string[];
}

export type EnvironmentSort = 'ascending' | 'descending';

/**
 * Options of the picker: the environments matching the search. Pinned ones
 * come first, and each group is sorted by display name.
 */
export function environmentOptions(
  environments: readonly EnvironmentChoice[],
  pinned: readonly string[],
  query: string,
  sort: EnvironmentSort,
): EnvironmentChoice[] {
  const term = query.trim().toLocaleLowerCase();
  const pinnedFiles = new Set(pinned);

  const matches = environments.filter((environment) => matchesQuery(environment, term));
  matches.sort((first, second) => {
    const firstPinned = pinnedFiles.has(first.FileName);
    const secondPinned = pinnedFiles.has(second.FileName);
    if (firstPinned !== secondPinned) {
      return firstPinned ? -1 : 1;
    }
    const byName =
      first.Name.localeCompare(second.Name, undefined, { sensitivity: 'base' }) ||
      first.FileName.localeCompare(second.FileName);
    return sort === 'descending' ? -byName : byName;
  });

  return matches;
}

/** A search term matches the display name, the file name or a scope. */
function matchesQuery(environment: EnvironmentChoice, term: string): boolean {
  if (!term) {
    return true;
  }
  const haystack = [environment.Name, environment.FileName, ...environment.Scopes]
    .join(' ')
    .toLocaleLowerCase();
  return haystack.includes(term);
}
