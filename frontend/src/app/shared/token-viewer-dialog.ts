import { Component, ElementRef, computed, effect, inject, signal, viewChild } from '@angular/core';
import { TokenViewer, ViewedToken } from '@core/token-viewer';
import { describeExpiry } from './expiry';
import { Icon } from './icon';
import { decodeJwt, describeClaims, jwtValidity } from './jwt';

type CopyTarget = 'token' | 'header' | 'payload';

/**
 * Decodes a JSON Web Token the way jwt.io does: the encoded token colored by
 * part, its header and payload as JSON, and its registered claims explained.
 * The token can be edited or replaced by any pasted JWT. Signatures are not
 * verified.
 */
@Component({
  selector: 'app-token-viewer-dialog',
  imports: [Icon],
  templateUrl: './token-viewer-dialog.html',
  styleUrl: './token-viewer-dialog.css',
})
export class TokenViewerDialog {
  protected readonly viewer = inject(TokenViewer);
  private readonly dialog = viewChild.required<ElementRef<HTMLDialogElement>>('dialog');

  /** Tokens offered as tabs; a pasted token replaces the selected value. */
  protected readonly tokens = signal<ViewedToken[]>([]);
  protected readonly selectedIndex = signal(0);
  protected readonly text = signal('');
  protected readonly copied = signal<CopyTarget | null>(null);
  private copiedTimer: ReturnType<typeof setTimeout> | undefined;

  protected readonly decoding = computed(() => decodeJwt(this.text()));
  protected readonly jwt = computed(() => {
    const decoding = this.decoding();
    return decoding.ok ? decoding.jwt : null;
  });
  protected readonly headerJson = computed(() => format(this.jwt()?.header));
  protected readonly payloadJson = computed(() => format(this.jwt()?.payload));
  protected readonly claims = computed(() => describeClaims(this.jwt()?.payload ?? {}));
  protected readonly validity = computed(() => {
    const jwt = this.jwt();
    return jwt ? jwtValidity(jwt.payload, this.now()) : null;
  });
  /** Current time, ticking every second while the dialog is open. */
  private readonly now = signal(Date.now());
  private timer: ReturnType<typeof setInterval> | undefined;
  /** Expiry of the token as reported by the token endpoint. */
  protected readonly endpointExpiry = computed(() => {
    const expiresAt = this.viewer.pending()?.expiresAtMilliseconds;
    return expiresAt === undefined ? null : describeExpiry(expiresAt, this.now());
  });
  /** Expiry of the decoded token, from its `exp` claim. */
  protected readonly jwtExpiry = computed(() => {
    const expiration = this.jwt()?.payload['exp'];
    return typeof expiration === 'number' ? describeExpiry(expiration * 1000, this.now()) : null;
  });

  constructor() {
    effect(() => {
      const request = this.viewer.pending();
      const dialog = this.dialog().nativeElement;
      if (request) {
        this.tokens.set(request.tokens);
        this.select(0);
        if (!dialog.open) {
          dialog.showModal();
        }
        this.startClock();
      } else {
        this.stopClock();
        if (dialog.open) {
          dialog.close();
        }
      }
    });
  }

  private startClock(): void {
    this.now.set(Date.now());
    clearInterval(this.timer);
    this.timer = setInterval(() => this.now.set(Date.now()), 1000);
  }

  private stopClock(): void {
    clearInterval(this.timer);
    this.timer = undefined;
  }

  protected select(index: number): void {
    this.selectedIndex.set(index);
    this.text.set(this.tokens()[index]?.value ?? '');
    this.copied.set(null);
  }

  protected edit(event: Event): void {
    this.text.set((event.target as HTMLTextAreaElement).value);
  }

  protected async copy(target: CopyTarget): Promise<void> {
    const value =
      target === 'token'
        ? this.text().trim()
        : target === 'header'
          ? this.headerJson()
          : this.payloadJson();
    if (!value) {
      return;
    }

    await navigator.clipboard.writeText(value);
    this.copied.set(target);
    clearTimeout(this.copiedTimer);
    this.copiedTimer = setTimeout(() => this.copied.set(null), 1500);
  }

  protected closeOnBackdrop(event: MouseEvent): void {
    if (event.target === this.dialog().nativeElement) {
      this.viewer.close();
    }
  }
}

function format(value: Record<string, unknown> | undefined): string {
  return value ? JSON.stringify(value, null, 2) : '';
}
