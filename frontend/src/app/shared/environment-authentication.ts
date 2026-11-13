import {
  Authentication,
  AuthenticationType,
  OAuth2GrantType,
} from '@bindings/github.com/awake-client/awake/src/domain/models';

/** One accepted value of a reserved variable. */
export interface ReservedOption {
  value: string;
  label: string;
}

/** An environment variable the application reads instead of interpolating. */
export interface ReservedVariable {
  name: string;
  /** What setting the variable does, shown next to it. */
  description: string;
  options: ReservedOption[];
}

export const AUTH_TYPE_VARIABLE = 'authType';
export const GRANT_TYPE_VARIABLE = 'grantType';

const AUTHENTICATION_OPTIONS: ReservedOption[] = [
  { value: AuthenticationType.AuthenticationNone, label: 'None' },
  { value: AuthenticationType.AuthenticationBasic, label: 'Basic' },
  { value: AuthenticationType.AuthenticationBearer, label: 'Bearer token' },
  { value: AuthenticationType.AuthenticationAPIKey, label: 'API key' },
  { value: AuthenticationType.AuthenticationOAuth2, label: 'OAuth 2' },
];

const GRANT_OPTIONS: ReservedOption[] = [
  { value: OAuth2GrantType.OAuth2GrantAuthorizationCode, label: 'Authorization code (PKCE)' },
  { value: OAuth2GrantType.OAuth2GrantClientCredentials, label: 'Client credentials' },
  { value: OAuth2GrantType.OAuth2GrantPassword, label: 'Password' },
];

/**
 * Variables that choose how requests authenticate in an environment. They
 * mirror `domain.ApplyEnvironmentAuthentication` in the backend.
 */
export const RESERVED_VARIABLES: Record<string, ReservedVariable> = {
  [AUTH_TYPE_VARIABLE]: {
    name: AUTH_TYPE_VARIABLE,
    description: 'Authentication type used by every request in this environment',
    options: AUTHENTICATION_OPTIONS,
  },
  [GRANT_TYPE_VARIABLE]: {
    name: GRANT_TYPE_VARIABLE,
    description: 'OAuth 2 grant type used by every request in this environment',
    options: GRANT_OPTIONS,
  },
};

/** The reserved variable with this name, if any. */
export function reservedVariable(name: string): ReservedVariable | null {
  return RESERVED_VARIABLES[name.trim()] ?? null;
}

/**
 * Normalizes a reserved value to its option, accepting the same spellings as
 * the backend, or returns null when it is not accepted.
 */
export function parseReservedValue(name: string, value: string): string | null {
  const normalized = value.trim().toLowerCase();
  if (name.trim() === AUTH_TYPE_VARIABLE) {
    switch (normalized) {
      case 'none':
      case 'noauth':
        return AuthenticationType.AuthenticationNone;
      case 'basic':
        return AuthenticationType.AuthenticationBasic;
      case 'bearer':
        return AuthenticationType.AuthenticationBearer;
      case 'apikey':
      case 'api-key':
      case 'api_key':
        return AuthenticationType.AuthenticationAPIKey;
      case 'oauth2':
        return AuthenticationType.AuthenticationOAuth2;
    }
  }
  if (name.trim() === GRANT_TYPE_VARIABLE) {
    switch (normalized) {
      case 'authorizationcode':
      case 'authorization_code':
      case 'authorization-code':
        return OAuth2GrantType.OAuth2GrantAuthorizationCode;
      case 'clientcredentials':
      case 'client_credentials':
      case 'client_credential':
        return OAuth2GrantType.OAuth2GrantClientCredentials;
      case 'password':
      case 'resource_owner_password':
      case 'resource_owner_password_credentials':
        return OAuth2GrantType.OAuth2GrantPassword;
    }
  }
  return null;
}

/** The authentication actually sent, and what decided it. */
export interface EnvironmentAuthentication {
  authentication: Authentication | null;
  /** Reserved variables that changed the type or the grant. */
  overriddenBy: string[];
  /** A reserved variable holding a value the backend rejects. */
  invalid: { name: string; value: string } | null;
}

/**
 * Applies the reserved variables of the active environment to an
 * authentication, as the backend does when the request is sent.
 */
export function applyEnvironmentAuthentication(
  authentication: Authentication | null,
  values: ReadonlyMap<string, string>,
): EnvironmentAuthentication {
  const result: EnvironmentAuthentication = { authentication, overriddenBy: [], invalid: null };
  let effective = authentication ? { ...authentication } : null;

  for (const name of [AUTH_TYPE_VARIABLE, GRANT_TYPE_VARIABLE]) {
    const raw = values.get(name)?.trim() ?? '';
    if (raw === '') {
      continue;
    }
    const parsed = parseReservedValue(name, raw);
    if (parsed === null) {
      result.invalid ??= { name, value: raw };
      continue;
    }
    effective ??= { Type: AuthenticationType.AuthenticationNone } as Authentication;
    if (name === AUTH_TYPE_VARIABLE && effective.Type !== parsed) {
      effective.Type = parsed as AuthenticationType;
      result.overriddenBy.push(name);
    }
    if (name === GRANT_TYPE_VARIABLE && effective.GrantType !== parsed) {
      effective.GrantType = parsed as OAuth2GrantType;
      result.overriddenBy.push(name);
    }
  }

  result.authentication = effective;
  return result;
}
