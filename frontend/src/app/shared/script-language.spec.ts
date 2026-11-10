import { CompletionContext, CompletionResult, CompletionSource } from '@codemirror/autocomplete';
import { EditorState } from '@codemirror/state';
import { describe, expect, it } from 'vitest';

import { ScriptKind, diagnoseScript, scriptLanguage } from './script-language';

const REQUESTS = ['collections/auth/login.yaml', 'collections/billing/list-invoices.yaml'];

/** The first completion result the script language offers at the | cursor. */
function complete(
  source: string,
  kind: ScriptKind = 'pre-request',
  requests: readonly string[] = [],
): CompletionResult | null {
  const cursor = source.indexOf('|');
  const state = EditorState.create({
    doc: source.replace('|', ''),
    extensions: [scriptLanguage(kind, requests)],
  });
  const context = new CompletionContext(state, cursor, true);
  const sources = state.languageDataAt<CompletionSource>('autocomplete', cursor);
  for (const source of sources) {
    const result = source(context);
    if (result && !(result instanceof Promise)) {
      return result;
    }
  }
  return null;
}

describe('script completions', () => {
  it('completes the members of awake in a pre-request script', () => {
    const labels = complete('awake.|')?.options.map((option) => option.label);
    expect(labels).toContain('runRequest');
    expect(labels).toContain('environment');
  });

  it('completes the request paths of awake.runRequest', () => {
    const result = complete('awake.runRequest("coll|', 'pre-request', REQUESTS);
    expect(result?.from).toBe('awake.runRequest("'.length);
    expect(result?.options.map((option) => option.label)).toEqual(REQUESTS);
  });

  it('completes a path written with backticks', () => {
    const result = complete('awake.runRequest(`coll|', 'pre-request', REQUESTS);
    expect(result?.options.map((option) => option.label)).toEqual(REQUESTS);
  });

  it('leaves the arguments of other calls alone', () => {
    expect(complete('awake.log("coll|', 'pre-request', REQUESTS)).toBeNull();
  });

  it('offers no path when the workspace has no other request', () => {
    expect(complete('awake.runRequest("coll|')).toBeNull();
  });

  it('offers no path in a post-response script', () => {
    expect(complete('awake.runRequest("coll|', 'post-response', REQUESTS)).toBeNull();
  });
});

describe('diagnoseScript', () => {
  it('accepts a valid post-response script', () => {
    const source = `
      const body = response.json();
      awake.test('ok', response.status === 200);
      awake.environment.set('token', body.access_token);
      request.headers['X-Id'] = variables.id;
    `;
    expect(diagnoseScript(source, 'post-response')).toEqual([]);
  });

  it('reports syntax errors', () => {
    const diagnostics = diagnoseScript('awake.log(1;', 'post-response');
    expect(diagnostics.some((diagnostic) => diagnostic.severity === 'error')).toBe(true);
  });

  it('suggests the closest member for a typo', () => {
    const [diagnostic] = diagnoseScript(
      'awake.test("ok", response.statsu === 200);',
      'post-response',
    );
    expect(diagnostic.severity).toBe('warning');
    expect(diagnostic.message).toContain('Did you mean "status"?');
  });

  it('checks nested namespaces', () => {
    const [diagnostic] = diagnoseScript('awake.environment.sett("a", "b");', 'post-response');
    expect(diagnostic.message).toContain('Did you mean "set"?');
  });

  it('flags response in a pre-request script', () => {
    const [diagnostic] = diagnoseScript('awake.log(response.status);', 'pre-request');
    expect(diagnostic.message).toContain('post-response');
  });

  it('leaves names the script declares alone', () => {
    expect(diagnoseScript('const response = {}; response.anything = 1;', 'pre-request')).toEqual(
      [],
    );
  });

  it('follows calls into named environments', () => {
    const valid = `
      const production = awake.environments.get('Production');
      awake.environments.get("Production").set('cookies', response.cookieHeader);
      console.log(response.cookies.session, production.name);
    `;
    expect(diagnoseScript(valid, 'post-response')).toEqual([]);

    const [diagnostic] = diagnoseScript(
      'awake.environments.get("Production").sett("a", "b");',
      'post-response',
    );
    expect(diagnostic.message).toContain('Did you mean "set"?');
  });

  it('knows the console', () => {
    const [diagnostic] = diagnoseScript('console.wrn("x");', 'pre-request');
    expect(diagnostic.message).toContain('Did you mean "warn"?');
  });

  it('accepts a request run from a pre-request script', () => {
    const source = `
      const login = awake.runRequest('collections/auth/login.yaml', { timeout: 2000 });
      request.headers['Authorization'] = 'Bearer ' + login.json().access_token;
    `;
    expect(diagnoseScript(source, 'pre-request')).toEqual([]);
  });

  it('only offers runRequest to the pre-request script', () => {
    const [diagnostic] = diagnoseScript(
      `awake.runRequest('collections/auth/login.yaml');`,
      'post-response',
    );
    expect(diagnostic.message).toContain('runRequest');
  });
});
