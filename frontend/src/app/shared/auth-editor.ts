import { Component, input, linkedSignal, output } from '@angular/core';
import { RouterLink } from '@angular/router';
import {
  Authentication,
  AuthenticationType,
  OAuth2GrantType,
} from '@bindings/github.com/clementherve/awake/src/domain/models';
import { Icon, IconName } from './icon';
import { SecretInput } from './secret-input';
import { VariableInput } from './variable-input';

/** Fields of an authentication edited as plain text. */
export type AuthenticationField =
  | 'Username'
  | 'Password'
  | 'Token'
  | 'Key'
  | 'Value'
  | 'Location'
  | 'AuthorizationURL'
  | 'TokenURL'
  | 'ClientID'
  | 'ClientSecret'
  | 'RedirectURL'
  | 'Audience';

interface TypeOption {
  type: AuthenticationType;
  label: string;
  icon: IconName;
}

const TYPES: TypeOption[] = [
  { type: AuthenticationType.AuthenticationInherit, label: 'Inherit', icon: 'layers' },
  { type: AuthenticationType.AuthenticationNone, label: 'None', icon: 'close' },
  { type: AuthenticationType.AuthenticationBasic, label: 'Basic', icon: 'lock' },
  { type: AuthenticationType.AuthenticationBearer, label: 'Bearer', icon: 'key' },
  { type: AuthenticationType.AuthenticationAPIKey, label: 'API key', icon: 'code' },
  { type: AuthenticationType.AuthenticationOAuth2, label: 'OAuth 2', icon: 'globe' },
];

/** Human readable name of an authentication type. */
export function describeAuthentication(authentication: Authentication | null | undefined): string {
  switch (authentication?.Type) {
    case AuthenticationType.AuthenticationBasic:
      return 'Basic authentication';
    case AuthenticationType.AuthenticationBearer:
      return 'Bearer token';
    case AuthenticationType.AuthenticationAPIKey:
      return `API key in the ${authentication.Location === 'query' ? 'query' : 'headers'}`;
    case AuthenticationType.AuthenticationOAuth2:
      switch (authentication.GrantType) {
        case OAuth2GrantType.OAuth2GrantClientCredentials:
          return 'OAuth 2, client credentials';
        case OAuth2GrantType.OAuth2GrantPassword:
          return 'OAuth 2, password';
        default:
          return 'OAuth 2, authorization code';
      }
    default:
      return 'No authentication';
  }
}

/**
 * Edits an authentication, for a request or a collection. Values accept
 * `{{variables}}`, so credentials can come from environments and secrets.
 * Extra actions, such as authorizing OAuth 2, are projected with the
 * `oauth2Actions` attribute.
 */
@Component({
  selector: 'app-auth-editor',
  imports: [Icon, SecretInput, VariableInput, RouterLink],
  templateUrl: './auth-editor.html',
  styleUrl: './auth-editor.css',
})
export class AuthEditor {
  readonly authentication = input.required<Authentication>();
  /** Requests can inherit from their collection; collections cannot. */
  readonly allowInherit = input(true);
  /** What an inheriting request receives, and where to edit it. */
  readonly inherited = input<Authentication | null>(null);
  readonly inheritedFrom = input('');
  readonly collectionPath = input('');
  readonly authenticationChange = output<Authentication>();

  /**
   * Local copy updated on every keystroke, so two quick edits in different
   * fields never start from a stale value. It resets when the input changes.
   */
  protected readonly value = linkedSignal(() => this.authentication());

  protected readonly AuthenticationType = AuthenticationType;
  protected readonly OAuth2GrantType = OAuth2GrantType;
  protected readonly describe = describeAuthentication;

  /** Only the authorization code flow sends the user to a browser. */
  protected usesBrowser(): boolean {
    const grantType = this.value().GrantType;
    return !grantType || grantType === OAuth2GrantType.OAuth2GrantAuthorizationCode;
  }

  protected collectionLink(): string[] {
    return ['/workspace/collection', ...this.collectionPath().split('/')];
  }

  protected types(): TypeOption[] {
    return this.allowInherit()
      ? TYPES
      : TYPES.filter((option) => option.type !== AuthenticationType.AuthenticationInherit);
  }

  protected selectType(type: AuthenticationType): void {
    const authentication = this.value();
    this.change({
      ...authentication,
      Type: type,
      GrantType:
        type === AuthenticationType.AuthenticationOAuth2
          ? authentication.GrantType || OAuth2GrantType.OAuth2GrantAuthorizationCode
          : authentication.GrantType,
      Location: authentication.Location || 'header',
    });
  }

  protected update(field: AuthenticationField, value: string): void {
    this.change({ ...this.value(), [field]: value });
  }

  protected selectGrantType(grantType: OAuth2GrantType): void {
    this.change({ ...this.value(), GrantType: grantType });
  }

  protected scopesText(): string {
    return (this.value().Scopes ?? []).join(' ');
  }

  protected updateScopes(value: string): void {
    const scopes = value
      .split(/[\s,]+/)
      .map((scope) => scope.trim())
      .filter((scope) => scope !== '');
    this.change({ ...this.value(), Scopes: scopes });
  }

  private change(authentication: Authentication): void {
    this.value.set(authentication);
    this.authenticationChange.emit(authentication);
  }

  protected inputValue(event: Event): string {
    return (event.target as HTMLInputElement).value;
  }
}
