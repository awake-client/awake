import { BodyType, Parameter } from '@bindings/github.com/awake-client/awake/src/domain/models';
import { bodyDrift, jsonDrift, parameterDrift, parseJsonWithVariables } from './override-drift';

function parameter(name: string, value = '', enabled = true): Parameter {
  return { Name: name, Value: value, Enabled: enabled };
}

function json(content: string) {
  return { Type: BodyType.BodyJSON, Content: content, Fields: null };
}

describe('parameterDrift', () => {
  it('accepts the same names with other values', () => {
    expect(parameterDrift([parameter('page', '1')], [parameter('page', '2', false)])).toEqual([]);
  });

  it('reports shared parameters the override does not send, and local only ones', () => {
    expect(
      parameterDrift(
        [parameter('language'), parameter('page')],
        [parameter('page'), parameter('debug'), parameter(' ')],
      ),
    ).toEqual([
      { kind: 'missing', path: 'language', message: 'not in the local override' },
      { kind: 'extra', path: 'debug', message: 'no longer in the shared version' },
    ]);
  });
});

describe('jsonDrift', () => {
  it('accepts the same structure with other values', () => {
    expect(
      jsonDrift('{"name": "a", "tags": ["x"], "age": 3}', '{"age": 4, "name": "b", "tags": []}'),
    ).toEqual([]);
  });

  it('reports missing, extra and retyped keys with their path', () => {
    const issues = jsonDrift(
      '{"user": {"name": "a", "address": {"city": "x"}}, "count": 1}',
      '{"user": {"name": 2, "address": {}, "nickname": "b"}, "count": 1}',
    );

    expect(issues).toEqual([
      { kind: 'type', path: 'user.name', message: 'a string in the shared body, a number here' },
      { kind: 'missing', path: 'user.address.city', message: 'missing here' },
      { kind: 'extra', path: 'user.nickname', message: 'no longer in the shared body' },
    ]);
  });

  it('checks every array item against the first shared one', () => {
    expect(jsonDrift('[{"id": 1}]', '[{"id": 2}, {"id": "3"}]')).toEqual([
      { kind: 'type', path: '[1].id', message: 'a number in the shared body, a string here' },
    ]);
  });

  it('lets a variable stand for any value', () => {
    expect(jsonDrift('{"id": {{movieId}}, "name": "x"}', '{"id": 550, "name": {{name}}}')).toEqual(
      [],
    );
  });

  it('reports a local body that is not JSON, and skips an unreadable shared one', () => {
    expect(jsonDrift('{"a": 1}', '{"a": ')[0].kind).toBe('invalid');
    expect(jsonDrift('not json', '{"a": 1}')).toEqual([]);
  });

  it('quotes keys that are not identifiers', () => {
    expect(jsonDrift('{"content-type": "x"}', '{}')[0].path).toBe('"content-type"');
  });
});

describe('bodyDrift', () => {
  it('reports a changed body type', () => {
    expect(bodyDrift(json('{}'), { Type: BodyType.BodyText, Content: '', Fields: null })).toEqual([
      { kind: 'type', path: '', message: 'the shared body is now JSON, not text' },
    ]);
  });

  it('treats an unset type as no body', () => {
    expect(
      bodyDrift(
        { Type: BodyType.$zero, Content: '', Fields: null },
        { Type: BodyType.BodyNone, Content: '', Fields: null },
      ),
    ).toEqual([]);
  });

  it('compares form field names', () => {
    const form = (names: string[]) => ({
      Type: BodyType.BodyFormURLEncoded,
      Content: '',
      Fields: names.map((name) => parameter(name)),
    });

    expect(bodyDrift(form(['a', 'b']), form(['a'])).map((issue) => issue.path)).toEqual(['b']);
  });
});

describe('parseJsonWithVariables', () => {
  it('keeps variables inside strings as they are', () => {
    expect(parseJsonWithVariables('{"url": "{{base}}/x", "id": {{id}}}')).toEqual({
      url: '{{base}}/x',
      id: '\u0000awake-variable\u0000',
    });
  });
});
