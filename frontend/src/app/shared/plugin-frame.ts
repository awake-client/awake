import {
  Component,
  DestroyRef,
  ElementRef,
  computed,
  effect,
  inject,
  input,
  signal,
  viewChild,
} from '@angular/core';
import { DomSanitizer } from '@angular/platform-browser';
import * as PluginsService from '@bindings/github.com/awake-client/awake/src/features/plugins/service';
import { ThemeStore } from '@core/theme-store';
import { WorkspaceStore } from '@core/workspace-store';
import { HostMessage, parseViewMessage, resolvedTheme } from './plugin-frame-protocol';

/** Prefix of the pages the backend serves for plugin views. */
const VIEW_PATH_PREFIX = '/plugin-views/';

/** Origin a sandboxed frame reports; postMessage needs '*' to reach it. */
const SANDBOXED_TARGET_ORIGIN = '*';

/**
 * Renders the page of a plugin view in a frame sandboxed without same-origin
 * access, so the page cannot reach the application, its storage or its
 * bindings. The page receives its context through postMessage and may only
 * call the handlers its plugin registered, which run in the backend under the
 * permissions the user approved.
 */
@Component({
  selector: 'app-plugin-frame',
  template: `
    <iframe
      #frame
      class="plugin-frame"
      sandbox="allow-scripts"
      referrerpolicy="no-referrer"
      [title]="title()"
      [src]="url()"
    ></iframe>
    @if (failure(); as message) {
      <p class="plugin-frame-error" role="alert">{{ message }}</p>
    }
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      min-height: 0;
      flex: 1;
    }
    .plugin-frame {
      flex: 1;
      width: 100%;
      min-height: 160px;
      border: none;
      background: var(--color-surface);
    }
    .plugin-frame-error {
      margin: 0;
      padding: 6px 12px;
      font-size: var(--text-sm);
      color: var(--color-danger);
    }
  `,
})
export class PluginFrame {
  /** URL of the page, as the backend lists it under /plugin-views/. */
  readonly source = input.required<string>();
  readonly pluginId = input.required<string>();
  readonly viewId = input.required<string>();
  readonly kind = input.required<'response' | 'request'>();
  /** What the view shows: the response, or the request being edited. */
  readonly data = input<unknown>(null);
  readonly title = input('Plugin view');

  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly themeStore = inject(ThemeStore);
  private readonly sanitizer = inject(DomSanitizer);
  private readonly frame = viewChild.required<ElementRef<HTMLIFrameElement>>('frame');
  private readonly ready = signal(false);
  private readonly prefersDark = signal(matchMedia('(prefers-color-scheme: dark)').matches);

  protected readonly failure = signal<string | null>(null);
  /**
   * The source as a URL Angular lets a frame load. Only paths the backend
   * serves for enabled plugins, under a sandbox policy, are trusted; anything
   * else loads a blank page.
   */
  protected readonly url = computed(() => {
    const source = this.source();
    const path =
      source.startsWith(VIEW_PATH_PREFIX) && !source.includes('..') ? source : 'about:blank';
    return this.sanitizer.bypassSecurityTrustResourceUrl(path);
  });
  private readonly theme = computed(() =>
    resolvedTheme(this.themeStore.preference(), this.prefersDark()),
  );

  constructor() {
    const listener = (event: MessageEvent) => this.receive(event);
    window.addEventListener('message', listener);
    const media = matchMedia('(prefers-color-scheme: dark)');
    const themeListener = (event: MediaQueryListEvent) => this.prefersDark.set(event.matches);
    media.addEventListener('change', themeListener);
    inject(DestroyRef).onDestroy(() => {
      window.removeEventListener('message', listener);
      media.removeEventListener('change', themeListener);
    });

    // A new page starts over: it says when it is ready again.
    effect(() => {
      this.source();
      this.ready.set(false);
    });
    // The context follows the data and the theme once the page listens.
    effect(() => {
      const data = this.data();
      const theme = this.theme();
      if (this.ready()) {
        this.post({ type: 'awake:context', theme, kind: this.kind(), data: cloneable(data) });
      }
    });
  }

  private receive(event: MessageEvent): void {
    // Only the frame of this component speaks to it; any other window, or a
    // page the frame navigated to, is ignored by the protocol checks.
    if (event.source !== this.frame().nativeElement.contentWindow) {
      return;
    }
    const message = parseViewMessage(event.data);
    if (!message) {
      return;
    }
    if (message.type === 'awake:ready') {
      this.ready.set(true);
      this.post({
        type: 'awake:context',
        theme: this.theme(),
        kind: this.kind(),
        data: cloneable(this.data()),
      });
      return;
    }
    void this.invoke(message.id, message.handler, message.payload);
  }

  private async invoke(id: number, handler: string, payload: unknown): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      this.post({ type: 'awake:error', id, message: 'No workspace is open.' });
      return;
    }
    try {
      const encoded = await PluginsService.InvokeView(
        rootPath,
        this.pluginId(),
        this.viewId(),
        handler,
        JSON.stringify(payload ?? null),
      );
      this.failure.set(null);
      this.post({ type: 'awake:result', id, result: encoded ? JSON.parse(encoded) : null });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      this.failure.set(message);
      this.post({ type: 'awake:error', id, message });
    }
  }

  private post(message: HostMessage): void {
    this.frame().nativeElement.contentWindow?.postMessage(message, SANDBOXED_TARGET_ORIGIN);
  }
}

/** Plain data only crosses postMessage; anything else would throw. */
function cloneable(data: unknown): unknown {
  return data === undefined ? null : JSON.parse(JSON.stringify(data));
}
