import {
  Body,
  BodyType,
  Parameter,
} from '@bindings/github.com/clementherve/awake/src/domain/models';

/** A body that sends nothing. */
export function emptyBody(): Body {
  return { Type: BodyType.BodyNone, Content: '', Fields: null };
}

/** Whether a body type is made of fields rather than text. */
export function isFormBody(type: BodyType): boolean {
  return type === BodyType.BodyFormURLEncoded || type === BodyType.BodyMultipart;
}

/**
 * Reads a form written as text into fields, as the backend does: an encoded
 * query such as a=1&b=2, or one field per line written name: value or
 * name=value, a leading ~ disabling it.
 */
export function parseFormContent(content: string): Parameter[] {
  const trimmed = content.trim();
  if (trimmed === '') {
    return [];
  }

  if (!trimmed.includes('\n') && !trimmed.includes(': ')) {
    return trimmed
      .split('&')
      .filter((pair) => pair !== '')
      .map((pair) => {
        const [name, ...rest] = pair.split('=');
        return {
          Name: decodeFormValue(name),
          Value: decodeFormValue(rest.join('=')),
          Enabled: true,
        };
      });
  }

  const fields: Parameter[] = [];
  for (const rawLine of trimmed.split('\n')) {
    let line = rawLine.trim();
    if (line === '' || line.startsWith('#')) {
      continue;
    }
    const enabled = !line.startsWith('~');
    line = line.replace(/^~/, '').trim();
    const separator = line.search(/[:=]/);
    if (separator <= 0) {
      continue;
    }
    fields.push({
      Name: line.slice(0, separator).trim(),
      Value: line.slice(separator + 1).trim(),
      Enabled: enabled,
    });
  }
  return fields;
}

function decodeFormValue(value: string): string {
  try {
    return decodeURIComponent(value.replace(/\+/g, ' '));
  } catch {
    return value;
  }
}
