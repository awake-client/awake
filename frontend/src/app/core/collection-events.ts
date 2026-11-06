/** Window event asking the creation dialog to open. */
export const CREATE_ENTRY_EVENT = 'awake:create-entry';

/** Window event sent after collections changed on disk. */
export const COLLECTIONS_CHANGED_EVENT = 'awake:collections-changed';

/** Kind of entry that can be created in the collection tree. */
export type CreationKind = 'request' | 'folder' | 'collection' | 'environment';

/** Detail of the {@link CREATE_ENTRY_EVENT} event. */
export interface CreationRequest {
  kind: CreationKind;
  /** Directory receiving the new entry; defaults to the current selection. */
  folder?: string;
  /** For an environment: a collection path, or empty for a global one. */
  scope?: string;
}

/** Detail of the {@link COLLECTIONS_CHANGED_EVENT} event. */
export interface CollectionsChange {
  /** Workspace relative path of the entry just created, to reveal it. */
  created?: string;
  kind?: CreationKind;
  /** Workspace relative path of the entry just deleted, with its children. */
  removed?: string;
  /** An entry, and everything inside it, moved from one path to another. */
  moved?: { from: string; to: string };
}

/** Opens the creation dialog from anywhere. */
export function requestEntryCreation(request: CreationRequest): void {
  window.dispatchEvent(new CustomEvent<CreationRequest>(CREATE_ENTRY_EVENT, { detail: request }));
}

/** Tells every view that the collection tree changed. */
export function notifyCollectionsChanged(change: CollectionsChange = {}): void {
  window.dispatchEvent(
    new CustomEvent<CollectionsChange>(COLLECTIONS_CHANGED_EVENT, {
      detail: change,
    }),
  );
}
