import {
  Authentication,
  AuthenticationType,
  Body,
  BodyType,
  Header,
  OAuth2GrantType,
  Protocol,
} from '@bindings/github.com/clementherve/awake/src/domain/models';
import { isFormBody, parseFormContent } from '@shared/form-body';

/** What the smart request field recognised in the text typed or pasted. */
export type RequestInputKind = 'empty' | 'url' | 'method-url' | 'curl';

/** A request draft extracted from free-form input. */
export interface ParsedRequestInput {
  kind: RequestInputKind;
  method: string;
  url: string;
  protocol: Protocol;
  headers: Header[];
  body: Body | null;
  authentication: Authentication | null;
  /** Parts of the input that could not be carried over. */
  warnings: string[];
  /** A readable name derived from the method and the URL path. */
  suggestedName: string;
}

const HTTP_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'];

const CURL_PREFIX = /^\s*(?:\$\s*)?curl(?:\.exe)?(?:\s|$)/i;
const METHOD_URL = new RegExp(`^(${HTTP_METHODS.join('|')})\\s+(\\S+)\\s*$`, 'i');

/**
 * Recognises a cURL command, a `METHOD url` pair or a plain URL and turns it
 * into a request draft. Unknown input is kept as the URL so nothing typed is
 * ever lost.
 */
export function parseRequestInput(text: string): ParsedRequestInput {
  const trimmed = text.trim();
  if (trimmed === '') {
    return draft('empty', 'GET', '');
  }

  if (CURL_PREFIX.test(trimmed)) {
    return parseCurl(trimmed);
  }

  const methodUrl = METHOD_URL.exec(trimmed);
  if (methodUrl) {
    return draft('method-url', methodUrl[1].toUpperCase(), methodUrl[2]);
  }

  return draft('url', 'GET', trimmed);
}

/** Tells whether a text looks like a cURL command. */
export function isCurlCommand(text: string): boolean {
  return CURL_PREFIX.test(text);
}

/** Options whose value is the next argument and that we can safely ignore. */
const IGNORED_VALUE_OPTIONS = new Set([
  '-o',
  '--output',
  '-m',
  '--max-time',
  '--connect-timeout',
  '-w',
  '--write-out',
  '--retry',
  '--retry-delay',
  '--retry-max-time',
  '-x',
  '--proxy',
  '--cacert',
  '--capath',
  '--cert',
  '-E',
  '--key',
  '--resolve',
  '-c',
  '--cookie-jar',
  '--limit-rate',
  '-r',
  '--range',
  '--max-redirs',
  '--interface',
  '-Y',
  '-y',
]);

/** Short options taking a value, which cURL also accepts glued (`-XPOST`). */
const SHORT_VALUE_OPTIONS = new Set('XHdubAeFTomxwrEcyY'.split(''));

function parseCurl(command: string): ParsedRequestInput {
  const tokens = tokenize(command).slice(1);
  const warnings: string[] = [];
  const headers: Header[] = [];
  const data: string[] = [];

  let method = '';
  let url = '';
  let user = '';
  let json = false;
  let forceGet = false;
  let head = false;

  for (let index = 0; index < tokens.length; index += 1) {
    let option = tokens[index];
    let inlineValue: string | undefined;

    if (option.startsWith('--') && option.includes('=')) {
      inlineValue = option.slice(option.indexOf('=') + 1);
      option = option.slice(0, option.indexOf('='));
    } else if (
      /^-[A-Za-z]/.test(option) &&
      option.length > 2 &&
      SHORT_VALUE_OPTIONS.has(option[1])
    ) {
      inlineValue = option.slice(2);
      option = option.slice(0, 2);
    }

    const value = (): string => {
      if (inlineValue !== undefined) {
        return inlineValue;
      }
      index += 1;
      return tokens[index] ?? '';
    };

    switch (option) {
      case '-X':
      case '--request':
        method = value().toUpperCase();
        break;
      case '-H':
      case '--header': {
        const header = parseHeader(value());
        if (header) {
          headers.push(header);
        }
        break;
      }
      case '-d':
      case '--data':
      case '--data-raw':
      case '--data-ascii':
      case '--data-binary':
      case '--data-urlencode': {
        const content = value();
        if (content.startsWith('@')) {
          warnings.push(`Data read from the file ${content.slice(1)} was not imported.`);
        } else {
          data.push(content);
        }
        break;
      }
      case '--json':
        json = true;
        data.push(value());
        break;
      case '-u':
      case '--user':
        user = value();
        break;
      case '-A':
      case '--user-agent':
        headers.push(header('User-Agent', value()));
        break;
      case '-b':
      case '--cookie':
        headers.push(header('Cookie', value()));
        break;
      case '-e':
      case '--referer':
        headers.push(header('Referer', value()));
        break;
      case '--url':
        url = value();
        break;
      case '-G':
      case '--get':
        forceGet = true;
        break;
      case '-I':
      case '--head':
        head = true;
        break;
      case '-F':
      case '--form':
        value();
        warnings.push('Multipart form fields (-F) are not supported yet.');
        break;
      case '-T':
      case '--upload-file':
        value();
        warnings.push('File uploads (-T) are not supported yet.');
        break;
      default:
        if (IGNORED_VALUE_OPTIONS.has(option)) {
          value();
        } else if (!option.startsWith('-') && url === '') {
          url = option;
        }
      // Remaining flags (-s, -L, -k, --compressed…) change how cURL runs,
      // not what the request is.
    }
  }

  let body: Body | null = null;
  if (forceGet && data.length > 0) {
    const separator = url.includes('?') ? '&' : '?';
    url = `${url}${separator}${data.join('&')}`;
  } else if (data.length > 0) {
    if (json) {
      addHeaderIfMissing(headers, 'Content-Type', 'application/json');
      addHeaderIfMissing(headers, 'Accept', 'application/json');
    }
    const content = data.join('&');
    const type = inferBodyType(content, headers, json);
    body = isFormBody(type)
      ? { Type: type, Content: '', Fields: parseFormContent(content) }
      : { Type: type, Content: content, Fields: null };
  }

  if (!method) {
    method = head ? 'HEAD' : body ? 'POST' : 'GET';
  }

  const result = draft('curl', method, url);
  result.headers = headers;
  result.body = body;
  result.warnings = warnings;
  if (user) {
    result.authentication = basicAuthentication(user);
  }
  if (
    headers.some(
      (entry) => entry.Name.toLowerCase() === 'accept' && entry.Value.includes('text/event-stream'),
    )
  ) {
    result.protocol = Protocol.ProtocolSSE;
  }
  if (url === '') {
    result.warnings.push('The command has no URL.');
  }
  return result;
}

/**
 * Splits a shell command into arguments, honouring single quotes, double
 * quotes, `$'…'` strings, backslash escapes and line continuations.
 */
export function tokenize(command: string): string[] {
  const tokens: string[] = [];
  let current = '';
  let hasToken = false;
  let index = 0;

  const push = () => {
    if (hasToken) {
      tokens.push(current);
    }
    current = '';
    hasToken = false;
  };

  while (index < command.length) {
    const character = command[index];

    if (character === '\\' && /\r?\n/.test(command.slice(index + 1, index + 3))) {
      index += command[index + 1] === '\r' ? 3 : 2;
      continue;
    }
    if (/\s/.test(character)) {
      push();
      index += 1;
      continue;
    }

    hasToken = true;

    if (character === "'") {
      const end = command.indexOf("'", index + 1);
      const stop = end < 0 ? command.length : end;
      current += command.slice(index + 1, stop);
      index = stop + 1;
      continue;
    }

    if (character === '$' && command[index + 1] === "'") {
      index += 2;
      while (index < command.length && command[index] !== "'") {
        if (command[index] === '\\' && index + 1 < command.length) {
          current += unescapeAnsi(command[index + 1]);
          index += 2;
        } else {
          current += command[index];
          index += 1;
        }
      }
      index += 1;
      continue;
    }

    if (character === '"') {
      index += 1;
      while (index < command.length && command[index] !== '"') {
        if (command[index] === '\\' && '"\\$`\n'.includes(command[index + 1])) {
          if (command[index + 1] !== '\n') {
            current += command[index + 1];
          }
          index += 2;
        } else {
          current += command[index];
          index += 1;
        }
      }
      index += 1;
      continue;
    }

    if (character === '\\' && index + 1 < command.length) {
      current += command[index + 1];
      index += 2;
      continue;
    }

    current += character;
    index += 1;
  }

  push();
  return tokens;
}

function unescapeAnsi(character: string): string {
  switch (character) {
    case 'n':
      return '\n';
    case 't':
      return '\t';
    case 'r':
      return '\r';
    default:
      return character;
  }
}

function draft(kind: RequestInputKind, method: string, url: string): ParsedRequestInput {
  const lowerUrl = url.toLowerCase();
  const isWebSocket = lowerUrl.startsWith('ws://') || lowerUrl.startsWith('wss://');
  return {
    kind,
    method: isWebSocket ? 'GET' : method,
    url,
    protocol: isWebSocket ? Protocol.ProtocolWebSocket : Protocol.ProtocolHTTP,
    headers: [],
    body: null,
    authentication: null,
    warnings: [],
    suggestedName: suggestName(method, url),
  };
}

function parseHeader(raw: string): Header | null {
  const separator = raw.indexOf(':');
  if (separator <= 0) {
    return null;
  }
  const name = raw.slice(0, separator).trim();
  const value = raw.slice(separator + 1).trim();
  return name ? header(name, value) : null;
}

function header(name: string, value: string): Header {
  return { Name: name, Value: value, Enabled: true };
}

function addHeaderIfMissing(headers: Header[], name: string, value: string): void {
  if (!headers.some((entry) => entry.Name.toLowerCase() === name.toLowerCase())) {
    headers.push(header(name, value));
  }
}

function inferBodyType(content: string, headers: Header[], json: boolean): BodyType {
  const contentType =
    headers.find((entry) => entry.Name.toLowerCase() === 'content-type')?.Value.toLowerCase() ?? '';

  if (json || contentType.includes('json')) {
    return BodyType.BodyJSON;
  }
  if (contentType.includes('xml')) {
    return BodyType.BodyXML;
  }
  if (contentType.includes('x-www-form-urlencoded')) {
    return BodyType.BodyFormURLEncoded;
  }
  if (contentType !== '') {
    return BodyType.BodyText;
  }

  const trimmed = content.trim();
  if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
    return BodyType.BodyJSON;
  }
  if (trimmed.startsWith('<')) {
    return BodyType.BodyXML;
  }
  // cURL sends -d data as a form unless told otherwise.
  return BodyType.BodyFormURLEncoded;
}

function basicAuthentication(user: string): Authentication {
  const separator = user.indexOf(':');
  return {
    Type: AuthenticationType.AuthenticationBasic,
    Username: separator < 0 ? user : user.slice(0, separator),
    Password: separator < 0 ? '' : user.slice(separator + 1),
    Token: '',
    Key: '',
    Value: '',
    Location: 'header',
    GrantType: OAuth2GrantType.OAuth2GrantAuthorizationCode,
    AuthorizationURL: '',
    TokenURL: '',
    ClientID: '',
    ClientSecret: '',
    Scopes: [],
    RedirectURL: '',
    Audience: '',
  };
}

/**
 * Builds a name such as "Get users" or "Delete invoice 42" from the last
 * meaningful segments of the URL path.
 */
function suggestName(method: string, url: string): string {
  if (url === '') {
    return '';
  }

  let path = url;
  try {
    const parsed = new URL(url.replace(/\{\{[^}]*\}\}/g, 'variable'));
    path = parsed.pathname === '/' ? parsed.hostname : parsed.pathname;
  } catch {
    path = url.replace(/^[a-z]+:\/\/[^/]+/i, '').split('?')[0];
  }

  const segments = path
    .split('/')
    .map((segment) => decodeSafely(segment))
    .filter((segment) => segment !== '' && !segment.startsWith('{{'));
  const words: string[] = [];
  for (let index = segments.length - 1; index >= 0 && words.length < 2; index -= 1) {
    const segment = segments[index];
    words.unshift(segment.replace(/[-_.]+/g, ' '));
    if (!/^[0-9a-f-]+$/i.test(segment)) {
      break;
    }
  }

  const verb = method.charAt(0).toUpperCase() + method.slice(1).toLowerCase();
  return words.length === 0 ? verb : `${verb} ${words.join(' ')}`;
}

function decodeSafely(segment: string): string {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}
