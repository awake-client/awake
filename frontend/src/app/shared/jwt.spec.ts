import { decodeJwt, describeClaims, jwtValidity } from './jwt';

function encode(value: unknown): string {
  const bytes = new TextEncoder().encode(JSON.stringify(value));
  const binary = Array.from(bytes, (byte) => String.fromCharCode(byte)).join('');
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

const header = { alg: 'HS256', typ: 'JWT' };
const payload = { sub: '1234567890', name: 'Zoé Ünïcode', iat: 1516239022, exp: 1516242622 };
const token = `${encode(header)}.${encode(payload)}.c2lnbmF0dXJl`;

describe('decodeJwt', () => {
  it('decodes the header and the payload, including UTF-8 text', () => {
    const decoding = decodeJwt(token);

    expect(decoding.ok).toBe(true);
    if (decoding.ok) {
      expect(decoding.jwt.header).toEqual(header);
      expect(decoding.jwt.payload).toEqual(payload);
      expect(decoding.jwt.encoded.signature).toBe('c2lnbmF0dXJl');
    }
  });

  it('ignores a Bearer prefix and surrounding whitespace', () => {
    expect(decodeJwt(`  Bearer ${token}\n`).ok).toBe(true);
  });

  it('rejects empty input, opaque tokens and encrypted tokens', () => {
    expect(decodeJwt('').ok).toBe(false);
    expect(decodeJwt('opaque-access-token').ok).toBe(false);
    expect(decodeJwt('a.b.c.d.e')).toEqual({
      ok: false,
      error: 'This is an encrypted token (JWE); its content cannot be read.',
    });
  });

  it('rejects segments that are not base64url JSON objects', () => {
    expect(decodeJwt(`${encode(header)}.not+base64.sig`).ok).toBe(false);
    expect(decodeJwt(`${encode(header)}.${encode([1, 2])}.sig`).ok).toBe(false);
  });
});

describe('describeClaims', () => {
  it('labels registered claims and formats timestamps', () => {
    const claims = describeClaims({ ...payload, aud: ['api', 'web'] });

    expect(claims.map((claim) => claim.name)).toEqual(['sub', 'iat', 'exp', 'aud']);
    expect(claims.find((claim) => claim.name === 'aud')?.value).toBe('api, web');
    expect(claims.find((claim) => claim.name === 'exp')?.detail).toBe(
      new Date(1516242622 * 1000).toLocaleString(),
    );
  });
});

describe('jwtValidity', () => {
  it('compares exp and nbf with the current time', () => {
    expect(jwtValidity({ exp: 100 }, 99_000)).toBe('valid');
    expect(jwtValidity({ exp: 100 }, 100_000)).toBe('expired');
    expect(jwtValidity({ nbf: 50, exp: 100 }, 10_000)).toBe('pending');
    expect(jwtValidity({}, 0)).toBe('unknown');
  });
});
