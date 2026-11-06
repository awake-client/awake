import { DatePipe } from '@angular/common';
import { Component, computed, effect, inject, input, signal } from '@angular/core';
import * as HistoryService from '@bindings/github.com/clementherve/awake/src/features/history/service';
import { HistoryEntry } from '@bindings/github.com/clementherve/awake/src/domain/models';
import { Confirmation } from '@core/confirmation';
import { WorkspaceStore } from '@core/workspace-store';
import { CodeEditor, CodeLanguage } from '@shared/code-editor';
import { Icon } from '@shared/icon';

const PAGE_SIZE = 30;

type DetailTab = 'request' | 'response';

/**
 * Past executions of one request. Each entry keeps the resolved request that
 * was sent, the response received and the time it took, so a call can be
 * inspected or compared later.
 */
@Component({
  selector: 'app-request-history',
  imports: [DatePipe, Icon, CodeEditor],
  templateUrl: './request-history.html',
  styleUrl: './request-history.css',
})
export class RequestHistory {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly confirmation = inject(Confirmation);

  readonly requestPath = input.required<string>();
  /** Changes each time the request is sent, to refresh the list. */
  readonly refreshKey = input<unknown>(null);

  protected readonly entries = signal<HistoryEntry[]>([]);
  protected readonly selected = signal<HistoryEntry | null>(null);
  protected readonly detailTab = signal<DetailTab>('response');
  protected readonly isLoading = signal(false);
  protected readonly hasMore = signal(false);
  protected readonly errorMessage = signal<string | null>(null);

  /** Longest duration of the page, to scale the timing bars. */
  protected readonly slowest = computed(() =>
    Math.max(1, ...this.entries().map((entry) => entry.DurationMilliseconds)),
  );

  constructor() {
    effect(() => {
      this.refreshKey();
      void this.load(this.requestPath());
    });
  }

  protected async load(requestPath: string): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath || !requestPath) {
      return;
    }
    this.isLoading.set(true);
    this.errorMessage.set(null);
    try {
      const entries =
        (await HistoryService.ListForRequest(rootPath, requestPath, PAGE_SIZE, 0)) ?? [];
      this.entries.set(entries);
      this.hasMore.set(entries.length === PAGE_SIZE);
      const selected = this.selected();
      if (!selected || !entries.some((entry) => entry.ID === selected.ID)) {
        this.selected.set(null);
        if (entries[0]) {
          await this.open(entries[0]);
        }
      }
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  protected async loadMore(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const last = this.entries().at(-1);
    if (!rootPath || !last) {
      return;
    }
    const next =
      (await HistoryService.ListForRequest(rootPath, this.requestPath(), PAGE_SIZE, last.ID)) ?? [];
    this.entries.update((entries) => [...entries, ...next]);
    this.hasMore.set(next.length === PAGE_SIZE);
  }

  protected async open(entry: HistoryEntry): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      return;
    }
    try {
      this.selected.set(await HistoryService.Entry(rootPath, entry.ID));
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  /** Removes one execution, without confirmation: it is a single entry. */
  protected async remove(entry: HistoryEntry, event: Event): Promise<void> {
    event.stopPropagation();
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      return;
    }
    try {
      await HistoryService.Delete(rootPath, entry.ID);
      const entries = this.entries();
      const index = entries.findIndex((candidate) => candidate.ID === entry.ID);
      const remaining = entries.filter((candidate) => candidate.ID !== entry.ID);
      this.entries.set(remaining);
      if (this.selected()?.ID === entry.ID) {
        this.selected.set(null);
        const next = remaining[Math.min(index, remaining.length - 1)];
        if (next) {
          await this.open(next);
        }
      }
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  /** Removes every execution of this request, after confirmation. */
  protected async clear(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      return;
    }
    const confirmed = await this.confirmation.ask({
      title: 'Clear the history of this request?',
      message:
        'Every recorded execution of this request, with its payload and response, will be deleted.',
      confirmLabel: 'Clear history',
      danger: true,
    });
    if (!confirmed) {
      return;
    }
    try {
      await HistoryService.ClearForRequest(rootPath, this.requestPath());
      this.entries.set([]);
      this.selected.set(null);
      this.hasMore.set(false);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    }
  }

  protected statusClass(status: number): string {
    if (status >= 500) {
      return 'server-error';
    }
    if (status >= 400) {
      return 'client-error';
    }
    if (status >= 300) {
      return 'redirect';
    }
    return 'success';
  }

  protected headerList(headers: Record<string, string[] | null | undefined> | null | undefined) {
    return Object.entries(headers ?? {})
      .sort(([first], [second]) => first.localeCompare(second))
      .map(([name, values]) => ({ name, value: (values ?? []).join(', ') }));
  }

  /** Pretty prints JSON bodies, leaves anything else untouched. */
  protected formatBody(body: string): string {
    const trimmed = body.trim();
    if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) {
      return body;
    }
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2);
    } catch {
      return body;
    }
  }

  protected languageOf(body: string): CodeLanguage {
    const trimmed = body.trim();
    if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
      return 'json';
    }
    return trimmed.startsWith('<') ? 'xml' : 'text';
  }

  protected formatSize(bytes: number): string {
    if (bytes < 1024) {
      return `${bytes} B`;
    }
    if (bytes < 1024 * 1024) {
      return `${(bytes / 1024).toFixed(1)} KB`;
    }
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  }

  protected relativeTime(executedAt: string): string {
    const seconds = Math.round((Date.now() - new Date(executedAt).getTime()) / 1000);
    if (seconds < 60) {
      return 'just now';
    }
    if (seconds < 3600) {
      return `${Math.floor(seconds / 60)} min ago`;
    }
    if (seconds < 86_400) {
      return `${Math.floor(seconds / 3600)} h ago`;
    }
    return new Date(executedAt).toLocaleDateString();
  }

  protected timingWidth(entry: HistoryEntry): number {
    return Math.max(4, (entry.DurationMilliseconds / this.slowest()) * 100);
  }
}

function describeError(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
