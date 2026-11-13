import {
  Component,
  DestroyRef,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';
import {
  Authentication,
  AuthenticationType,
  OAuth2GrantType,
} from '@bindings/github.com/awake-client/awake/src/domain/models';
import { Status } from '@bindings/github.com/awake-client/awake/src/features/oauth2/models';
import * as OAuth2Service from '@bindings/github.com/awake-client/awake/src/features/oauth2/service';
import { EnvironmentStore } from '@core/environment-store';
import { TokenViewer, ViewedToken } from '@core/token-viewer';
import { VariableStore } from '@core/variable-store';
import { WorkspaceStore } from '@core/workspace-store';
import { applyEnvironmentAuthentication } from '@shared/environment-authentication';
import { describeExpiry } from '@shared/expiry';
import { Icon } from '@shared/icon';

/**
 * Gets, shows and clears the OAuth 2 token of a request, collection or
 * folder. The token belongs to the entry that defines the authentication, so
 * a collection and the requests inheriting from it share it.
 */
@Component({
  selector: 'app-oauth2-actions',
  imports: [Icon],
  template: `
    <div class="actions">
      <button type="button" class="btn btn-sm" [disabled]="busy()" (click)="authorize()">
        {{ busy() ? 'Authorizing…' : usesBrowser() ? 'Authorize in browser' : 'Get token' }}
      </button>
      <button
        type="button"
        class="btn btn-sm btn-ghost"
        [disabled]="busy() || !status()?.HasToken"
        (click)="view()"
      >
        <app-icon name="eye" [size]="13" />
        View token
      </button>
      <button
        type="button"
        class="btn btn-sm btn-ghost"
        [disabled]="busy() || !status()?.HasToken"
        (click)="clear()"
      >
        Clear token
      </button>
      @if (expiry(); as expiry) {
        <span class="expiry" [attr.data-state]="expiry.state">
          <span class="dot" aria-hidden="true"></span>
          <strong>{{ expiry.label }}</strong>
          @if (expiry.at) {
            <span class="at">{{ expiry.state === 'expired' ? 'at' : 'on' }} {{ expiry.at }}</span>
          }
        </span>
      } @else {
        <span class="note">No token stored.</span>
      }
    </div>
    @if (error(); as message) {
      <p class="error">{{ message }}</p>
    }
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      gap: 8px;
    }
    .actions {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      gap: 8px;
    }
    .note {
      font-size: var(--text-sm);
      color: var(--color-text-subtle);
    }
    .expiry {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      height: 26px;
      padding: 0 10px;
      border-radius: 999px;
      font-size: var(--text-sm);
      color: var(--state-color);
      background: color-mix(in srgb, var(--state-color) 12%, transparent);
      --state-color: var(--color-success);
    }
    .expiry[data-state='expiring'] {
      --state-color: var(--color-warning);
    }
    .expiry[data-state='expired'] {
      --state-color: var(--color-danger);
    }
    .expiry[data-state='none'] {
      --state-color: var(--color-text-muted);
    }
    .expiry strong {
      font-weight: 600;
    }
    .expiry .at {
      color: var(--color-text-muted);
    }
    .dot {
      width: 7px;
      height: 7px;
      border-radius: 50%;
      background: var(--state-color);
    }
    .error {
      margin: 0;
      max-width: 64ch;
      font-size: var(--text-base);
      color: var(--color-danger);
    }
  `,
})
export class OAuth2Actions {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly environmentStore = inject(EnvironmentStore);
  private readonly tokenViewer = inject(TokenViewer);
  private readonly variableStore = inject(VariableStore);

  /** Request file, or directory of a collection or folder. */
  readonly entryPath = input.required<string>();
  /** The authentication in effect, inherited or not; used for labels. */
  readonly authentication = input<Authentication | null>(null);
  /** Changes when the entry is saved, so the status is read again. */
  readonly revision = input(0);
  /**
   * Called before contacting the token endpoint, typically to save pending
   * edits. Returning false cancels.
   */
  readonly beforeAuthorize = input<() => Promise<boolean>>(async () => true);

  /** The authentication sent, once the environment chose its type and grant. */
  private readonly inUse = computed(() => {
    const values = new Map<string, string>();
    for (const [name, variable] of this.variableStore.variables()) {
      values.set(name, variable.value);
    }
    return applyEnvironmentAuthentication(this.authentication(), values).authentication;
  });

  protected readonly status = signal<Status | null>(null);
  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);
  /** Current time, ticking every second so the remaining time stays live. */
  private readonly now = signal(Date.now());
  protected readonly expiry = computed(() => {
    const status = this.status();
    return status?.HasToken ? describeExpiry(status.ExpiresAtMilliseconds, this.now()) : null;
  });

  constructor() {
    const timer = setInterval(() => this.now.set(Date.now()), 1000);
    inject(DestroyRef).onDestroy(() => clearInterval(timer));

    effect(() => {
      const authentication = this.inUse();
      this.entryPath();
      this.revision();
      this.environmentStore.selected();
      this.workspaceStore.rootPath();
      authentication?.Type;
      authentication?.GrantType;
      untracked(() => void this.refresh());
    });
  }

  protected usesBrowser(): boolean {
    const grantType = this.inUse()?.GrantType;
    return !grantType || grantType === OAuth2GrantType.OAuth2GrantAuthorizationCode;
  }

  protected async authorize(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath || this.busy() || !(await this.beforeAuthorize()())) {
      return;
    }

    this.busy.set(true);
    this.error.set(null);
    try {
      this.status.set(
        await OAuth2Service.Authorize(rootPath, this.entryPath(), this.environmentStore.selected()),
      );
    } catch (error) {
      this.error.set(describeError(error));
    } finally {
      this.busy.set(false);
    }
  }

  /** Opens the access token, and the ID and refresh tokens if any, in the viewer. */
  protected async view(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      return;
    }

    this.error.set(null);
    try {
      const details = await OAuth2Service.Token(
        rootPath,
        this.entryPath(),
        this.environmentStore.selected(),
      );
      const tokens: ViewedToken[] = [{ label: 'Access token', value: details.AccessToken }];
      if (details.IDToken) {
        tokens.push({ label: 'ID token', value: details.IDToken });
      }
      if (details.RefreshToken) {
        tokens.push({ label: 'Refresh token', value: details.RefreshToken });
      }
      const information = [`Type: ${details.TokenType || 'Bearer'}`];
      if (details.Scopes?.length) {
        information.push(`Scopes: ${details.Scopes.join(' ')}`);
      }
      this.tokenViewer.open({
        tokens,
        details: information,
        expiresAtMilliseconds: details.ExpiresAtMilliseconds,
      });
    } catch (error) {
      this.error.set(describeError(error));
      void this.refresh();
    }
  }

  protected async clear(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      return;
    }

    this.busy.set(true);
    this.error.set(null);
    try {
      await OAuth2Service.Clear(rootPath, this.entryPath(), this.environmentStore.selected());
      this.status.set(null);
    } catch (error) {
      this.error.set(describeError(error));
    } finally {
      this.busy.set(false);
    }
  }

  private async refresh(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const entryPath = this.entryPath();
    const authentication = this.inUse();
    if (
      !rootPath ||
      !entryPath ||
      authentication?.Type !== AuthenticationType.AuthenticationOAuth2 ||
      !authentication.GrantType
    ) {
      this.status.set(null);
      return;
    }

    try {
      this.status.set(
        await OAuth2Service.Status(rootPath, entryPath, this.environmentStore.selected()),
      );
    } catch {
      this.status.set(null);
    }
  }
}

function describeError(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
