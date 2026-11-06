import {
  AuthenticationType,
  BodyType,
  Protocol,
} from '@bindings/github.com/clementherve/awake/src/domain/models';
import { parseRequestInput, tokenize } from './request-input';

describe('parseRequestInput', () => {
  it('keeps a plain URL as a GET request', () => {
    const parsed = parseRequestInput('https://api.example.com/v1/users');

    expect(parsed.kind).toBe('url');
    expect(parsed.method).toBe('GET');
    expect(parsed.url).toBe('https://api.example.com/v1/users');
    expect(parsed.suggestedName).toBe('Get users');
  });

  it('reads a method followed by a URL', () => {
    const parsed = parseRequestInput('delete {{baseUrl}}/invoices/42');

    expect(parsed.kind).toBe('method-url');
    expect(parsed.method).toBe('DELETE');
    expect(parsed.url).toBe('{{baseUrl}}/invoices/42');
    expect(parsed.suggestedName).toBe('Delete invoices 42');
  });

  it('switches WebSocket URLs to the WebSocket protocol', () => {
    const parsed = parseRequestInput('wss://stream.example.com/live');

    expect(parsed.protocol).toBe(Protocol.ProtocolWebSocket);
  });

  it('imports a multi-line cURL command', () => {
    const parsed = parseRequestInput(`curl -X POST 'https://api.example.com/users' \\
      -H 'Content-Type: application/json' \\
      -H "Authorization: Bearer abc" \\
      --data-raw '{"name":"Ada"}' --compressed -sL`);

    expect(parsed.kind).toBe('curl');
    expect(parsed.method).toBe('POST');
    expect(parsed.url).toBe('https://api.example.com/users');
    expect(parsed.headers.map((header) => header.Name)).toEqual(['Content-Type', 'Authorization']);
    expect(parsed.body).toEqual({
      Type: BodyType.BodyJSON,
      Content: '{"name":"Ada"}',
    });
    expect(parsed.warnings).toEqual([]);
  });

  it('defaults to POST with a form body when data is given', () => {
    const parsed = parseRequestInput('curl https://example.com/login -d user=ada -d pass=x');

    expect(parsed.method).toBe('POST');
    expect(parsed.body).toEqual({
      Type: BodyType.BodyFormURLEncoded,
      Content: 'user=ada&pass=x',
    });
  });

  it('moves data into the query string with -G', () => {
    const parsed = parseRequestInput('curl -G https://example.com/search -d q=awake');

    expect(parsed.method).toBe('GET');
    expect(parsed.url).toBe('https://example.com/search?q=awake');
    expect(parsed.body).toBeNull();
  });

  it('understands glued short options, --json and basic credentials', () => {
    const parsed = parseRequestInput(
      'curl -XPUT -u ada:secret --json \'{"a":1}\' https://example.com/items/1',
    );

    expect(parsed.method).toBe('PUT');
    expect(parsed.authentication?.Type).toBe(AuthenticationType.AuthenticationBasic);
    expect(parsed.authentication?.Username).toBe('ada');
    expect(parsed.authentication?.Password).toBe('secret');
    expect(parsed.body?.Type).toBe(BodyType.BodyJSON);
    expect(parsed.headers.map((header) => header.Name)).toEqual(['Content-Type', 'Accept']);
  });

  it('detects server-sent events from the Accept header', () => {
    const parsed = parseRequestInput(
      'curl -N -H "Accept: text/event-stream" https://example.com/events',
    );

    expect(parsed.protocol).toBe(Protocol.ProtocolSSE);
  });

  it('reports what it cannot import', () => {
    const parsed = parseRequestInput('curl -F file=@a.png https://example.com/upload');

    expect(parsed.warnings.length).toBe(1);
    expect(parsed.url).toBe('https://example.com/upload');
  });
});

describe('tokenize', () => {
  it('handles quotes, escapes and ANSI-C strings', () => {
    expect(tokenize(`curl 'a b' "c \\"d\\"" e\\ f $'g\\nh'`)).toEqual([
      'curl',
      'a b',
      'c "d"',
      'e f',
      'g\nh',
    ]);
  });
});
