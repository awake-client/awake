import { describe, expect, it } from 'vitest';

import { diagnoseScript } from './script-language';

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
});
