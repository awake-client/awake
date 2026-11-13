import {
  Authentication,
  AuthenticationType,
  OAuth2GrantType,
} from '@bindings/github.com/awake-client/awake/src/domain/models';
import {
  applyEnvironmentAuthentication,
  parseReservedValue,
  reservedVariable,
} from './environment-authentication';

const bearer = { Type: AuthenticationType.AuthenticationBearer, Token: 'kept' } as Authentication;

describe('parseReservedValue', () => {
  it('accepts the spellings of the backend', () => {
    expect(parseReservedValue('authType', ' noauth ')).toBe('none');
    expect(parseReservedValue('authType', 'API_KEY')).toBe('apiKey');
    expect(parseReservedValue('grantType', 'client_credentials')).toBe('clientCredentials');
    expect(parseReservedValue('grantType', 'implicit')).toBeNull();
    expect(parseReservedValue('baseUrl', 'oauth2')).toBeNull();
  });
});

describe('reservedVariable', () => {
  it('knows the authentication variables only', () => {
    expect(reservedVariable(' authType ')?.options.length).toBe(5);
    expect(reservedVariable('grantType')?.options.map((option) => option.value)).toEqual([
      'authorizationCode',
      'clientCredentials',
      'password',
    ]);
    expect(reservedVariable('token')).toBeNull();
  });
});

describe('applyEnvironmentAuthentication', () => {
  it('keeps the authentication without reserved variables', () => {
    const result = applyEnvironmentAuthentication(bearer, new Map([['token', 'x']]));
    expect(result.authentication).toEqual(bearer);
    expect(result.overriddenBy).toEqual([]);
  });

  it('switches the type and the grant and reports which variables did', () => {
    const result = applyEnvironmentAuthentication(
      bearer,
      new Map([
        ['authType', 'oauth2'],
        ['grantType', 'password'],
      ]),
    );
    expect(result.authentication).toMatchObject({
      Type: AuthenticationType.AuthenticationOAuth2,
      GrantType: OAuth2GrantType.OAuth2GrantPassword,
      Token: 'kept',
    });
    expect(result.overriddenBy).toEqual(['authType', 'grantType']);
  });

  it('does not report a variable repeating the configured value', () => {
    const result = applyEnvironmentAuthentication(bearer, new Map([['authType', 'bearer']]));
    expect(result.overriddenBy).toEqual([]);
  });

  it('flags a value the backend would reject', () => {
    const result = applyEnvironmentAuthentication(bearer, new Map([['authType', 'token']]));
    expect(result.invalid).toEqual({ name: 'authType', value: 'token' });
    expect(result.authentication).toEqual(bearer);
  });
});
