import {
  Body,
  BodyType,
  Parameter,
} from '@bindings/github.com/awake-client/awake/src/domain/models';

/**
 * A way a local override no longer matches the shared version it was copied
 * from: the shared request changed after the override was made.
 */
export interface DriftIssue {
  /** missing: the shared version has it, the override does not send it.
   *  extra: only the override has it.
   *  type: both have it, with a different kind of value. */
  kind: 'missing' | 'extra' | 'type' | 'invalid';
  /** Where it is: a JSON path such as `user.address.city`, or a name. */
  path: string;
  message: string;
}

/** Issues listed before the rest is summed up as "and N more". */
export const MAXIMUM_DRIFT_ISSUES = 50;

/** A variable written as a bare JSON value, such as `"id": {{movieId}}`. */
const VARIABLE_VALUE = '\u0000awake-variable\u0000';

type JsonKind = 'object' | 'array' | 'string' | 'number' | 'boolean' | 'null' | 'variable';

/**
 * Checks the local query parameters against the shared ones. The override
 * replaces the whole list, so a shared parameter missing from it is no longer
 * sent, and one only the override has is not part of the shared request.
 */
export function parameterDrift(
  shared: readonly Parameter[],
  local: readonly Parameter[],
): DriftIssue[] {
  const sharedNames = namesOf(shared);
  const localNames = namesOf(local);
  const issues: DriftIssue[] = [];
  for (const name of sharedNames) {
    if (!localNames.has(name)) {
      issues.push({ kind: 'missing', path: name, message: 'not in the local override' });
    }
  }
  for (const name of localNames) {
    if (!sharedNames.has(name)) {
      issues.push({ kind: 'extra', path: name, message: 'no longer in the shared version' });
    }
  }
  return issues;
}

/**
 * Checks a local body against the shared one. A JSON body is validated
 * against the structure of the shared JSON, used as its schema: the same
 * keys, and values of the same kind. A form body compares its field names.
 */
export function bodyDrift(shared: Body, local: Body): DriftIssue[] {
  if (normalizedType(shared.Type) !== normalizedType(local.Type)) {
    return [
      {
        kind: 'type',
        path: '',
        message: `the shared body is now ${bodyTypeLabel(shared.Type)}, not ${bodyTypeLabel(local.Type)}`,
      },
    ];
  }
  if (shared.Type === BodyType.BodyJSON) {
    return jsonDrift(shared.Content, local.Content);
  }
  if (shared.Type === BodyType.BodyFormURLEncoded || shared.Type === BodyType.BodyMultipart) {
    return parameterDrift(shared.Fields ?? [], local.Fields ?? []);
  }
  return [];
}

/** Validates the local JSON against the structure of the shared JSON. */
export function jsonDrift(sharedContent: string, localContent: string): DriftIssue[] {
  const shared = parseJsonWithVariables(sharedContent);
  if (shared === undefined) {
    // Without a readable shared body there is no schema to check against.
    return [];
  }
  const local = parseJsonWithVariables(localContent);
  if (local === undefined) {
    return [{ kind: 'invalid', path: '', message: 'the local body is not valid JSON' }];
  }
  const issues: DriftIssue[] = [];
  compareJson(shared, local, '', issues);
  return issues;
}

/**
 * Parses JSON that may hold variables as bare values, which the request
 * resolves before sending. Returns undefined when it is not JSON; empty
 * content reads as undefined too.
 */
export function parseJsonWithVariables(content: string): unknown {
  if (content.trim() === '') {
    return undefined;
  }
  try {
    return JSON.parse(quoteBareVariables(content));
  } catch {
    return undefined;
  }
}

/** Replaces each {{name}} written outside a string with a marker string. */
function quoteBareVariables(content: string): string {
  let result = '';
  let inString = false;
  for (let position = 0; position < content.length; position++) {
    const character = content[position];
    if (inString) {
      result += character;
      if (character === '\\') {
        result += content[++position] ?? '';
      } else if (character === '"') {
        inString = false;
      }
      continue;
    }
    if (character === '"') {
      inString = true;
      result += character;
      continue;
    }
    if (character === '{' && content[position + 1] === '{') {
      const end = content.indexOf('}}', position + 2);
      if (end >= 0) {
        result += JSON.stringify(VARIABLE_VALUE);
        position = end + 1;
        continue;
      }
    }
    result += character;
  }
  return result;
}

function compareJson(shared: unknown, local: unknown, path: string, issues: DriftIssue[]): void {
  if (issues.length >= MAXIMUM_DRIFT_ISSUES) {
    return;
  }
  const sharedKind = kindOf(shared);
  const localKind = kindOf(local);
  // A variable stands for any value: only its place is checked.
  if (sharedKind === 'variable' || localKind === 'variable') {
    return;
  }
  if (sharedKind !== localKind) {
    issues.push({
      kind: 'type',
      path: displayPath(path),
      message: `${article(sharedKind)} in the shared body, ${article(localKind)} here`,
    });
    return;
  }

  if (sharedKind === 'object') {
    const sharedObject = shared as Record<string, unknown>;
    const localObject = local as Record<string, unknown>;
    for (const key of Object.keys(sharedObject)) {
      const childPath = joinPath(path, key);
      if (!(key in localObject)) {
        issues.push({ kind: 'missing', path: displayPath(childPath), message: 'missing here' });
      } else {
        compareJson(sharedObject[key], localObject[key], childPath, issues);
      }
    }
    for (const key of Object.keys(localObject)) {
      if (!(key in sharedObject)) {
        issues.push({
          kind: 'extra',
          path: displayPath(joinPath(path, key)),
          message: 'no longer in the shared body',
        });
      }
    }
    return;
  }

  if (sharedKind === 'array') {
    // The first shared item describes every item; an empty array says nothing.
    const [schema] = shared as unknown[];
    if (schema === undefined) {
      return;
    }
    (local as unknown[]).forEach((item, index) =>
      compareJson(schema, item, `${path}[${index}]`, issues),
    );
  }
}

function kindOf(value: unknown): JsonKind {
  if (value === VARIABLE_VALUE) {
    return 'variable';
  }
  if (value === null) {
    return 'null';
  }
  if (Array.isArray(value)) {
    return 'array';
  }
  return typeof value as JsonKind;
}

function article(kind: JsonKind): string {
  switch (kind) {
    case 'object':
    case 'array':
      return `an ${kind}`;
    case 'null':
      return 'null';
    default:
      return `a ${kind}`;
  }
}

function joinPath(path: string, key: string): string {
  const segment = /^[A-Za-z_$][\w$]*$/.test(key) ? key : JSON.stringify(key);
  return path === '' ? segment : `${path}.${segment}`;
}

function displayPath(path: string): string {
  return path === '' ? '(root)' : path;
}

function namesOf(parameters: readonly Parameter[]): Set<string> {
  return new Set(
    parameters.map((parameter) => parameter.Name.trim()).filter((name) => name !== ''),
  );
}

/** An unset type sends no body, like "none". */
function normalizedType(type: BodyType): BodyType {
  return type === BodyType.$zero ? BodyType.BodyNone : type;
}

function bodyTypeLabel(type: BodyType): string {
  switch (normalizedType(type)) {
    case BodyType.BodyJSON:
      return 'JSON';
    case BodyType.BodyText:
      return 'text';
    case BodyType.BodyXML:
      return 'XML';
    case BodyType.BodyBinary:
      return 'a file';
    case BodyType.BodyNone:
      return 'empty';
    case BodyType.BodyFormURLEncoded:
      return 'a URL encoded form';
    case BodyType.BodyMultipart:
      return 'a multipart form';
    default:
      return String(type);
  }
}
