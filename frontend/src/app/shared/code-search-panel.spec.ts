import { SearchQuery } from '@codemirror/search';
import { EditorSelection, EditorState } from '@codemirror/state';
import { describeMatches, matchPosition } from './code-search-panel';

function stateWith(doc: string, from = 0, to = from): EditorState {
  return EditorState.create({ doc, selection: EditorSelection.single(from, to) });
}

describe('matchPosition', () => {
  it('counts the matches and finds the selected one', () => {
    const state = stateWith('id: 1, id: 2, id: 3', 7, 9);

    expect(matchPosition(state, new SearchQuery({ search: 'id' }))).toEqual({
      current: 2,
      total: 3,
      capped: false,
    });
  });

  it('ignores case by default', () => {
    expect(
      matchPosition(stateWith('Movie movie'), new SearchQuery({ search: 'movie' })).total,
    ).toBe(2);
  });

  it('reports nothing for an empty query', () => {
    expect(matchPosition(stateWith('text'), new SearchQuery({ search: '' })).total).toBe(0);
  });
});

describe('describeMatches', () => {
  it('reads like a search bar', () => {
    expect(describeMatches({ current: 2, total: 3, capped: false }, true)).toBe('2 of 3');
    expect(describeMatches({ current: 0, total: 3, capped: false }, true)).toBe('3 found');
    expect(describeMatches({ current: 0, total: 0, capped: false }, true)).toBe('No results');
    expect(describeMatches({ current: 1, total: 1000, capped: true }, true)).toBe('1 of 1000+');
    expect(describeMatches({ current: 0, total: 0, capped: false }, false)).toBe('');
  });
});
