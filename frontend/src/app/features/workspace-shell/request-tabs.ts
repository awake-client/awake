import { Component, inject, signal } from '@angular/core';
import { Confirmation } from '@core/confirmation';
import { RequestTab, TabsStore } from '@core/tabs-store';

/** Short labels keep tabs narrow. */
const METHOD_LABELS: Record<string, string> = { DELETE: 'DEL', OPTIONS: 'OPT' };

/**
 * Tab bar of the open requests, shown at the top of the work area. Tabs can
 * be reordered by dragging, pinned with a double click and closed with the
 * close button, a middle click or ⌘W.
 */
@Component({
  selector: 'app-request-tabs',
  host: {
    '(document:keydown)': 'handleShortcut($event)',
  },
  template: `
    <div class="tab-strip" role="tablist" aria-label="Open requests">
      @for (tab of tabs.tabs(); track tab.path; let index = $index) {
        <div
          class="tab"
          role="tab"
          tabindex="0"
          draggable="true"
          [class.active]="tab.path === tabs.active()"
          [class.preview]="tab.preview"
          [class.drop-before]="dropIndex() === index"
          [attr.aria-selected]="tab.path === tabs.active()"
          [title]="tab.path"
          (click)="tabs.open(tab.path)"
          (dblclick)="tabs.pin(tab.path)"
          (auxclick)="$event.button === 1 && close(tab, $event)"
          (keydown.enter)="tabs.open(tab.path)"
          (dragstart)="startDrag($event, tab)"
          (dragover)="dragOver($event, index)"
          (drop)="drop($event, index)"
          (dragend)="endDrag()"
        >
          <span class="tab-method" [attr.data-method]="tab.protocol === 'http' ? tab.method : null">
            {{ label(tab) }}
          </span>
          <span class="tab-name">{{ tab.name }}</span>
          <button
            type="button"
            class="tab-close"
            [class.dirty]="tab.dirty"
            [attr.aria-label]="'Close ' + tab.name"
            [title]="tab.dirty ? 'Unsaved changes' : 'Close (⌘W)'"
            (click)="close(tab, $event)"
          >
            <span class="dirty-dot" aria-hidden="true"></span>
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M7 7l10 10M17 7 7 17" /></svg>
          </button>
        </div>
      }
      <div
        class="tab-tail"
        [class.drop-before]="dropIndex() === tabs.tabs().length"
        (dragover)="dragOver($event, tabs.tabs().length)"
        (drop)="drop($event, tabs.tabs().length)"
      ></div>
    </div>
  `,
  styleUrl: './request-tabs.css',
})
export class RequestTabs {
  protected readonly tabs = inject(TabsStore);
  private readonly confirmation = inject(Confirmation);

  protected readonly dropIndex = signal<number | null>(null);
  private dragged: string | null = null;

  protected label(tab: RequestTab): string {
    if (tab.protocol === 'sse') {
      return 'SSE';
    }
    if (tab.protocol === 'websocket') {
      return 'WS';
    }
    return METHOD_LABELS[tab.method] ?? tab.method;
  }

  /** Closes a tab, asking first when it holds unsaved changes. */
  protected async close(tab: RequestTab, event?: Event): Promise<void> {
    event?.stopPropagation();
    event?.preventDefault();
    if (tab.dirty) {
      const confirmed = await this.confirmation.ask({
        title: `Close “${tab.name}” without saving?`,
        message: 'Your unsaved changes to this request will be lost.',
        confirmLabel: 'Discard changes',
        danger: true,
      });
      if (!confirmed) {
        return;
      }
    }
    this.tabs.close(tab.path);
  }

  protected handleShortcut(event: KeyboardEvent): void {
    const command = event.metaKey || event.ctrlKey;
    if (command && event.key.toLowerCase() === 'w') {
      const active = this.tabs.activeTab();
      if (active) {
        event.preventDefault();
        void this.close(active);
      }
      return;
    }
    if (event.ctrlKey && event.key === 'Tab') {
      event.preventDefault();
      this.tabs.cycle(event.shiftKey ? -1 : 1);
      return;
    }
    if (command && event.shiftKey && (event.key === '[' || event.key === '{')) {
      event.preventDefault();
      this.tabs.cycle(-1);
    } else if (command && event.shiftKey && (event.key === ']' || event.key === '}')) {
      event.preventDefault();
      this.tabs.cycle(1);
    }
  }

  protected startDrag(event: DragEvent, tab: RequestTab): void {
    this.dragged = tab.path;
    event.dataTransfer?.setData('text/plain', tab.path);
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move';
    }
  }

  protected dragOver(event: DragEvent, index: number): void {
    if (!this.dragged) {
      return;
    }
    event.preventDefault();
    this.dropIndex.set(index);
  }

  protected drop(event: DragEvent, index: number): void {
    event.preventDefault();
    const dragged = this.dragged;
    if (dragged) {
      const from = this.tabs.tabs().findIndex((tab) => tab.path === dragged);
      this.tabs.move(dragged, from < index ? index - 1 : index);
    }
    this.endDrag();
  }

  protected endDrag(): void {
    this.dragged = null;
    this.dropIndex.set(null);
  }
}
