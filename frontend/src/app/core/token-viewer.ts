import { Injectable, signal } from '@angular/core';

/** One token offered by the viewer, such as an access or an ID token. */
export interface ViewedToken {
  label: string;
  value: string;
}

/** What the viewer shows when it opens. */
export interface TokenViewerRequest {
  tokens: ViewedToken[];
  /** Extra context shown under the title, such as the token expiry. */
  details?: string[];
  /** Expiry reported by the token endpoint, in milliseconds since the epoch. */
  expiresAtMilliseconds?: number;
}

/**
 * Opens the JWT viewer. The dialog is rendered once by the workspace shell;
 * callers only pass the tokens to inspect.
 */
@Injectable({ providedIn: 'root' })
export class TokenViewer {
  private readonly current = signal<TokenViewerRequest | null>(null);

  readonly pending = this.current.asReadonly();

  open(request: TokenViewerRequest): void {
    this.current.set(request);
  }

  close(): void {
    this.current.set(null);
  }
}
