/** A JSON Web Token split into its parts and decoded. */
export interface DecodedJwt {
  /** The three encoded segments, as they appear in the token. */
  encoded: { header: string; payload: string; signature: string };
  header: Record<string, unknown>;
  payload: Record<string, unknown>;
}

/** Outcome of decoding a token: the decoded parts or why it is not a JWT. */
export type JwtDecoding = { ok: true; jwt: DecodedJwt } | { ok: false; error: string };

/** A registered claim worth explaining next to its raw value. */
export interface ClaimDescription {
  name: string;
  label: string;
  value: string;
  /** Human readable form, such as a local date for timestamps. */
  detail?: string;
}

const TIMESTAMP_CLAIMS: Record<string, string> = {
  exp: 'Expires',
  iat: 'Issued at',
  nbf: 'Not before',
  auth_time: 'Authenticated at',
};

const TEXT_CLAIMS: Record<string, string> = {
  iss: 'Issuer',
  sub: 'Subject',
  aud: 'Audience',
  azp: 'Authorized party',
  jti: 'Token ID',
  scope: 'Scope',
  scp: 'Scope',
  client_id: 'Client ID',
};

/**
 * Decodes a compact JWS (`header.payload.signature`). The signature is not
 * verified: the viewer only shows what the token claims. A leading
 * `Bearer ` prefix and surrounding whitespace are ignored.
 */
export function decodeJwt(token: string): JwtDecoding {
  const compact = token.trim().replace(/^bearer\s+/i, '');
  if (compact === '') {
    return { ok: false, error: 'Paste a token to decode it.' };
  }

  const segments = compact.split('.');
  if (segments.length === 5) {
    return { ok: false, error: 'This is an encrypted token (JWE); its content cannot be read.' };
  }
  if (segments.length !== 3) {
    return { ok: false, error: 'Not a JWT: expected three segments separated by dots.' };
  }

  const [header, payload, signature] = segments;
  const decodedHeader = decodeSegment(header);
  if (!decodedHeader) {
    return { ok: false, error: 'The header is not valid base64url-encoded JSON.' };
  }
  const decodedPayload = decodeSegment(payload);
  if (!decodedPayload) {
    return { ok: false, error: 'The payload is not valid base64url-encoded JSON.' };
  }

  return {
    ok: true,
    jwt: {
      encoded: { header, payload, signature },
      header: decodedHeader,
      payload: decodedPayload,
    },
  };
}

/** Lists the well-known claims of a payload with readable values. */
export function describeClaims(payload: Record<string, unknown>): ClaimDescription[] {
  const claims: ClaimDescription[] = [];
  for (const [name, value] of Object.entries(payload)) {
    if (name in TIMESTAMP_CLAIMS && typeof value === 'number') {
      claims.push({
        name,
        label: TIMESTAMP_CLAIMS[name],
        value: String(value),
        detail: new Date(value * 1000).toLocaleString(),
      });
    } else if (name in TEXT_CLAIMS) {
      claims.push({
        name,
        label: TEXT_CLAIMS[name],
        value: Array.isArray(value) ? value.join(', ') : String(value),
      });
    }
  }
  return claims;
}

/**
 * Validity of a payload at a given time, from its `exp` and `nbf` claims:
 * `expired`, `pending` (not yet valid), `valid`, or `unknown` without `exp`.
 */
export function jwtValidity(
  payload: Record<string, unknown>,
  now: number = Date.now(),
): 'expired' | 'pending' | 'valid' | 'unknown' {
  const seconds = now / 1000;
  if (typeof payload['nbf'] === 'number' && seconds < payload['nbf']) {
    return 'pending';
  }
  if (typeof payload['exp'] !== 'number') {
    return 'unknown';
  }
  return seconds >= payload['exp'] ? 'expired' : 'valid';
}

function decodeSegment(segment: string): Record<string, unknown> | null {
  try {
    const value: unknown = JSON.parse(base64UrlDecode(segment));
    return value !== null && typeof value === 'object' && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

function base64UrlDecode(segment: string): string {
  if (!/^[A-Za-z0-9_-]*$/.test(segment)) {
    throw new Error('invalid base64url');
  }
  const base64 = segment.replace(/-/g, '+').replace(/_/g, '/');
  const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4);
  const binary = atob(padded);
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  return new TextDecoder('utf-8', { fatal: true }).decode(bytes);
}
