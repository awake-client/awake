import { Component, inject, signal } from '@angular/core';
import { Router } from '@angular/router';
import * as HistoryService from '@bindings/github.com/clementherve/awake/src/features/history/service';
import { HistoryEntry } from '@bindings/github.com/clementherve/awake/src/domain/models';
import { WorkspaceStore } from '@core/workspace-store';

const PAGE_SIZE = 50;

/** Lists the requests executed in this workspace, newest first. */
@Component({
  selector: 'app-history-panel',
  templateUrl: './history-panel.html',
  styleUrl: './history-panel.css',
})
export class HistoryPanel {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly router = inject(Router);

  protected readonly entries = signal<HistoryEntry[]>([]);
  protected readonly isLoading = signal(true);
  protected readonly isLoadingMore = signal(false);
  protected readonly isClearing = signal(false);
  protected readonly confirmingClear = signal(false);
  protected readonly hasMore = signal(false);
  protected readonly errorMessage = signal<string | null>(null);

  constructor() {
    void this.load();
  }

  protected async load(): Promise<void> {
    const workspaceRoot = this.workspaceStore.rootPath();
    if (!workspaceRoot) {
      return;
    }

    this.isLoading.set(true);
    this.errorMessage.set(null);

    try {
      const entries = (await HistoryService.List(workspaceRoot, PAGE_SIZE, 0)) ?? [];
      this.entries.set(entries);
      this.hasMore.set(entries.length === PAGE_SIZE);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  protected async loadMore(): Promise<void> {
    const workspaceRoot = this.workspaceStore.rootPath();
    const lastEntry = this.entries().at(-1);
    if (!workspaceRoot || !lastEntry || this.isLoadingMore()) {
      return;
    }

    this.isLoadingMore.set(true);

    try {
      const entries =
        (await HistoryService.List(workspaceRoot, PAGE_SIZE, lastEntry.ID)) ?? [];
      this.entries.update((current) => [...current, ...entries]);
      this.hasMore.set(entries.length === PAGE_SIZE);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoadingMore.set(false);
    }
  }

  protected open(entry: HistoryEntry): void {
    void this.router.navigate([
      '/workspace/request',
      ...entry.RequestPath.split('/'),
    ]);
  }

  protected askClear(): void {
    this.confirmingClear.set(true);
  }

  protected cancelClear(): void {
    this.confirmingClear.set(false);
  }

  protected async clear(): Promise<void> {
    const workspaceRoot = this.workspaceStore.rootPath();
    if (!workspaceRoot || this.isClearing()) {
      return;
    }

    this.isClearing.set(true);
    this.errorMessage.set(null);

    try {
      await HistoryService.Clear(workspaceRoot);
      this.entries.set([]);
      this.hasMore.set(false);
      this.confirmingClear.set(false);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isClearing.set(false);
    }
  }

  protected statusClass(statusCode: number): string {
    if (statusCode >= 500) {
      return 'server-error';
    }
    if (statusCode >= 400) {
      return 'client-error';
    }
    return 'success';
  }

  protected formatDate(value: string): string {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return value;
    }
    return date.toLocaleString();
  }

  protected formatSize(bytes: number): string {
    if (bytes < 1024) {
      return `${bytes} B`;
    }
    if (bytes < 1024 * 1024) {
      return `${(bytes / 1024).toFixed(1)} kB`;
    }
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
