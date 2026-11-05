import {
  Component,
  ElementRef,
  computed,
  effect,
  inject,
  input,
  signal,
  viewChild,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import * as HTTPClientService from '@bindings/github.com/clementherve/awake/src/features/httpclient/service';
import {
  Stream,
  StreamEvent,
} from '@bindings/github.com/clementherve/awake/src/features/httpclient/models';
import { Protocol } from '@bindings/github.com/clementherve/awake/src/domain/models';
import { WorkspaceStore } from '@core/workspace-store';

const POLL_INTERVAL_MILLISECONDS = 250;

/**
 * Displays a live server-sent events or WebSocket session. Events are polled
 * from the Go stream registry and rendered as they arrive.
 */
@Component({
  selector: 'app-stream-panel',
  imports: [FormsModule],
  templateUrl: './stream-panel.html',
  styleUrl: './stream-panel.css',
})
export class StreamPanel {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly eventLog =
    viewChild<ElementRef<HTMLDivElement>>('eventLog');

  readonly protocol = input.required<Protocol>();
  readonly requestPath = input.required<string>();
  readonly environmentFileName = input.required<string>();

  protected readonly stream = signal<Stream | null>(null);
  protected readonly events = signal<StreamEvent[]>([]);
  protected readonly isConnecting = signal(false);
  protected readonly isPolling = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly message = signal('');
  protected readonly activeTab = signal<'events' | 'headers'>('events');
  protected readonly isWebSocket = computed(
    () => this.protocol() === Protocol.ProtocolWebSocket,
  );
  readonly isLive = computed(() => {
    const stream = this.stream();
    return stream !== null && !stream.Closed;
  });
  protected readonly eventCount = computed(() => this.events().length);
  protected readonly headerEntries = computed(() => {
    const stream = this.stream();
    if (!stream) {
      return [];
    }

    const headers: Record<string, string[] | null | undefined> =
      stream.Headers ?? {};
    return Object.entries(headers).map(([name, values]) => ({
      name,
      value: (values ?? []).join(', '),
    }));
  });

  private nextIndex = 0;
  private pollTimer: number | null = null;

  constructor() {
    effect(() => {
      this.events();
      const log = this.eventLog()?.nativeElement;
      if (log) {
        log.scrollTop = log.scrollHeight;
      }
    });
  }

  ngOnDestroy(): void {
    void this.release();
  }

  /** Connects when idle and disconnects when the session is live. */
  async toggle(): Promise<void> {
    if (this.isLive()) {
      await this.disconnect();
      return;
    }

    await this.connect();
  }

  async connect(): Promise<void> {
    if (this.isConnecting() || this.isLive()) {
      return;
    }

    await this.release();

    this.isConnecting.set(true);
    this.errorMessage.set(null);

    try {
      const stream = await HTTPClientService.OpenStream(
        this.workspaceStore.rootPath() ?? '',
        this.requestPath(),
        this.environmentFileName(),
      );

      this.stream.set(stream);
      this.nextIndex = 0;
      this.activeTab.set('events');
      this.startPolling();
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isConnecting.set(false);
    }
  }

  async disconnect(): Promise<void> {
    const stream = this.stream();
    if (!stream || stream.Closed) {
      return;
    }

    try {
      await HTTPClientService.CloseStream(stream.ID);
    } catch (error) {
      this.errorMessage.set(describeError(error));
      this.stopPolling();
    }
  }

  /** Stops the session and drops every buffered event. */
  async release(): Promise<void> {
    this.stopPolling();

    const stream = this.stream();
    this.stream.set(null);
    this.events.set([]);
    this.nextIndex = 0;

    if (!stream) {
      return;
    }

    try {
      await HTTPClientService.ReleaseStream(stream.ID);
    } catch {
      // The stream may already have expired.
    }
  }

  protected async sendMessage(): Promise<void> {
    const stream = this.stream();
    const content = this.message();
    if (!stream || stream.Closed || !this.isWebSocket() || content === '') {
      return;
    }

    try {
      await HTTPClientService.SendStreamMessage(stream.ID, content, 'utf-8');
      this.message.set('');
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  protected handleComposerKeydown(event: KeyboardEvent): void {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      void this.sendMessage();
    }
  }

  protected selectTab(tab: 'events' | 'headers'): void {
    this.activeTab.set(tab);
  }

  protected displayData(event: StreamEvent): string {
    if (event.Encoding === 'base64') {
      return '(binary message)';
    }
    return event.Data;
  }

  protected directionLabel(event: StreamEvent): string {
    switch (event.Direction) {
      case 'in':
        return '↓';
      case 'out':
        return '↑';
      default:
        return '•';
    }
  }

  protected eventLabel(event: StreamEvent): string {
    if (event.Kind === 'event') {
      return event.Name || 'message';
    }
    return event.Kind;
  }

  protected formatTime(milliseconds: number): string {
    return new Date(milliseconds).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  }

  protected protocolLabel(): string {
    return this.isWebSocket() ? 'WebSocket' : 'Server-sent events';
  }

  private startPolling(): void {
    this.stopPolling();
    this.pollTimer = window.setInterval(() => {
      void this.poll();
    }, POLL_INTERVAL_MILLISECONDS);
    void this.poll();
  }

  private stopPolling(): void {
    if (this.pollTimer !== null) {
      window.clearInterval(this.pollTimer);
      this.pollTimer = null;
    }
  }

  private async poll(): Promise<void> {
    const stream = this.stream();
    if (!stream || this.isPolling()) {
      return;
    }

    this.isPolling.set(true);

    try {
      const page = await HTTPClientService.ReadStreamEvents(
        stream.ID,
        this.nextIndex,
      );

      if ((page.Events?.length ?? 0) > 0) {
        this.events.update((events) => [...events, ...(page.Events ?? [])]);
      }
      this.nextIndex = page.NextIndex;

      if (page.Closed) {
        this.stopPolling();
        this.stream.update((current) =>
          current
            ? { ...current, Closed: true, Error: page.Error }
            : current,
        );
        this.errorMessage.set(page.Error || null);
      }
    } catch (error) {
      this.stopPolling();
      this.errorMessage.set(describeError(error));
    } finally {
      this.isPolling.set(false);
    }
  }
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
