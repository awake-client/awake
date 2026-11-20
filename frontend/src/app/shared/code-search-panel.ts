import {
  SearchQuery,
  closeSearchPanel,
  findNext,
  findPrevious,
  getSearchQuery,
  setSearchQuery,
} from '@codemirror/search';
import { EditorSelection, EditorState } from '@codemirror/state';
import { EditorView, Panel, ViewUpdate } from '@codemirror/view';

/** Matches counted before the panel shows "1000+". */
const MAXIMUM_COUNTED_MATCHES = 1000;

const SVG_NAMESPACE = 'http://www.w3.org/2000/svg';
const CHEVRON_UP = 'm6 15 6-6 6 6';
const CHEVRON_DOWN = 'm6 9 6 6 6-6';
const CROSS = 'M6 6l12 12M18 6 6 18';

/** Where the selection stands among the matches of the query. */
export interface MatchPosition {
  /** One-based index of the selected match, 0 when none is selected. */
  current: number;
  total: number;
  /** True when counting stopped at {@link MAXIMUM_COUNTED_MATCHES}. */
  capped: boolean;
}

/** Counts the matches of query and finds the one the selection covers. */
export function matchPosition(state: EditorState, query: SearchQuery): MatchPosition {
  if (!query.valid) {
    return { current: 0, total: 0, capped: false };
  }
  const selection = state.selection.main;
  const cursor = query.getCursor(state);
  let total = 0;
  let current = 0;
  for (let next = cursor.next(); !next.done; next = cursor.next()) {
    total++;
    if (next.value.from === selection.from && next.value.to === selection.to) {
      current = total;
    }
    if (total >= MAXIMUM_COUNTED_MATCHES) {
      return { current, total, capped: true };
    }
  }
  return { current, total, capped: false };
}

/** Formats the match position as "3 of 12", "No results" or nothing. */
export function describeMatches(position: MatchPosition, hasQuery: boolean): string {
  if (!hasQuery) {
    return '';
  }
  if (position.total === 0) {
    return 'No results';
  }
  const total = position.capped ? `${position.total}+` : `${position.total}`;
  return position.current > 0 ? `${position.current} of ${total}` : `${total} found`;
}

/**
 * A compact search bar: one field, the match count, previous, next and close.
 * Enter goes to the next match, Shift+Enter to the previous one and Escape
 * closes the bar.
 */
export function createSearchPanel(view: EditorView): Panel {
  const field = document.createElement('input');
  field.type = 'text';
  field.placeholder = 'Search';
  field.spellcheck = false;
  field.autocomplete = 'off';
  field.className = 'awake-search-field';
  field.setAttribute('aria-label', 'Search');
  // openSearchPanel focuses and selects the element carrying this attribute.
  field.setAttribute('main-field', 'true');
  field.value = getSearchQuery(view.state).search;

  const count = document.createElement('span');
  count.className = 'awake-search-count';
  count.setAttribute('aria-live', 'polite');

  const previous = iconButton('Previous match (Shift+Enter)', CHEVRON_UP, () => findPrevious(view));
  const next = iconButton('Next match (Enter)', CHEVRON_DOWN, () => findNext(view));
  const close = iconButton('Close (Escape)', CROSS, () => closeSearchPanel(view));

  const commit = () => {
    const query = new SearchQuery({ search: field.value });
    if (!query.eq(getSearchQuery(view.state))) {
      view.dispatch({ effects: setSearchQuery.of(query) });
    }
  };
  field.addEventListener('input', () => {
    commit();
    revealMatchFrom(view, view.state.selection.main.from);
  });
  field.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      (event.shiftKey ? findPrevious : findNext)(view);
    } else if (event.key === 'Escape') {
      event.preventDefault();
      closeSearchPanel(view);
    }
  });

  const dom = document.createElement('div');
  dom.className = 'awake-search';
  dom.append(field, count, previous, next, close);

  const refresh = (state: EditorState) => {
    const query = getSearchQuery(state);
    if (query.search !== field.value && document.activeElement !== field) {
      field.value = query.search;
    }
    const hasQuery = query.search !== '';
    const position = matchPosition(state, query);
    count.textContent = describeMatches(position, hasQuery);
    count.classList.toggle('empty', hasQuery && position.total === 0);
    previous.disabled = next.disabled = position.total === 0;
  };
  refresh(view.state);

  return {
    dom,
    top: true,
    // openSearchPanel only focuses a bar that is already open.
    mount() {
      field.focus();
      field.select();
    },
    update(update: ViewUpdate) {
      if (update.docChanged || update.selectionSet || queryChanged(update)) {
        refresh(update.state);
      }
    },
  };
}

/**
 * Selects the first match at or after position, wrapping to the start, while
 * the query is typed. Unlike findNext, it leaves the text of the field alone.
 */
function revealMatchFrom(view: EditorView, position: number): void {
  const query = getSearchQuery(view.state);
  if (!query.valid) {
    return;
  }
  const match = firstMatch(query, view.state, position) ?? firstMatch(query, view.state, 0);
  if (match) {
    view.dispatch({
      selection: EditorSelection.single(match.from, match.to),
      effects: EditorView.scrollIntoView(match.from, { y: 'center' }),
      userEvent: 'select.search',
    });
  }
}

function firstMatch(
  query: SearchQuery,
  state: EditorState,
  from: number,
): { from: number; to: number } | null {
  const next = query.getCursor(state, from).next();
  return next.done ? null : next.value;
}

function queryChanged(update: ViewUpdate): boolean {
  return update.transactions.some((transaction) =>
    transaction.effects.some((effect) => effect.is(setSearchQuery)),
  );
}

function iconButton(label: string, path: string, action: () => void): HTMLButtonElement {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'awake-search-button';
  button.title = label;
  button.setAttribute('aria-label', label);
  // Keep the focus in the field, so Enter keeps stepping through matches.
  button.addEventListener('mousedown', (event) => event.preventDefault());
  button.addEventListener('click', action);

  const icon = document.createElementNS(SVG_NAMESPACE, 'svg');
  icon.setAttribute('viewBox', '0 0 24 24');
  icon.setAttribute('width', '14');
  icon.setAttribute('height', '14');
  icon.setAttribute('fill', 'none');
  icon.setAttribute('stroke', 'currentColor');
  icon.setAttribute('stroke-width', '2');
  icon.setAttribute('stroke-linecap', 'round');
  icon.setAttribute('stroke-linejoin', 'round');
  icon.setAttribute('aria-hidden', 'true');
  const shape = document.createElementNS(SVG_NAMESPACE, 'path');
  shape.setAttribute('d', path);
  icon.append(shape);
  button.append(icon);
  return button;
}
