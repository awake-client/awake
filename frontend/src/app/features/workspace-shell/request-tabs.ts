import { Component, inject, signal } from '@angular/core';
import { CollectionColors } from '@core/collection-colors';
import { Confirmation } from '@core/confirmation';
import { RequestTab, TabsStore } from '@core/tabs-store';

/** What a tab menu entry closes, relative to the tab clicked. */
type CloseTarget = 'this' | 'others' | 'left' | 'right' | 'saved' | 'all';

interface TabMenu {
  tab: RequestTab;
  index: number;
  top: number;
  left: number;
}

/** Size of the tab menu, to keep it inside the window. */
const MENU_WIDTH = 200;
const MENU_HEIGHT = 230;
const MENU_MARGIN = 8;

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
    '(document:click)': 'closeMenu()',
    '(window:blur)': 'closeMenu()',
    '(window:resize)': 'closeMenu()',
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
          [attr.data-color]="collectionColors.colorOf(tab.path) || null"
          [attr.aria-selected]="tab.path === tabs.active()"
          [title]="tab.path"
          (click)="tabs.open(tab.path)"
          (dblclick)="tabs.pin(tab.path)"
          (auxclick)="$event.button === 1 && close(tab, $event)"
          (contextmenu)="openMenu($event, tab, index)"
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

    @if (menu(); as open) {
      <div
        class="tab-menu"
        role="menu"
        [style.top.px]="open.top"
        [style.left.px]="open.left"
        (click)="$event.stopPropagation()"
      >
        <button type="button" role="menuitem" class="menu-item" (click)="closeFromMenu('this')">
          Close
          <kbd>⌘W</kbd>
        </button>
        <button
          type="button"
          role="menuitem"
          class="menu-item"
          [disabled]="tabs.tabs().length < 2"
          (click)="closeFromMenu('others')"
        >
          Close others
        </button>
        <button
          type="button"
          role="menuitem"
          class="menu-item"
          [disabled]="open.index === 0"
          (click)="closeFromMenu('left')"
        >
          Close to the left
        </button>
        <button
          type="button"
          role="menuitem"
          class="menu-item"
          [disabled]="open.index === tabs.tabs().length - 1"
          (click)="closeFromMenu('right')"
        >
          Close to the right
        </button>
        <hr class="menu-separator" />
        <button
          type="button"
          role="menuitem"
          class="menu-item"
          [disabled]="!hasSavedTabs()"
          (click)="closeFromMenu('saved')"
        >
          Close saved
        </button>
        <button type="button" role="menuitem" class="menu-item" (click)="closeFromMenu('all')">
          Close all
        </button>
      </div>
    }
  `,
  styleUrl: './request-tabs.css',
})
export class RequestTabs {
  protected readonly tabs = inject(TabsStore);
  protected readonly collectionColors = inject(CollectionColors);
  private readonly confirmation = inject(Confirmation);

  protected readonly dropIndex = signal<number | null>(null);
  /** The tab menu, opened with a right click on a tab. */
  protected readonly menu = signal<TabMenu | null>(null);
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

  protected openMenu(event: MouseEvent, tab: RequestTab, index: number): void {
    event.preventDefault();
    event.stopPropagation();
    // Keep the menu inside the window.
    const left = Math.min(event.clientX, window.innerWidth - MENU_WIDTH - MENU_MARGIN);
    const top = Math.min(event.clientY, window.innerHeight - MENU_HEIGHT - MENU_MARGIN);
    this.menu.set({
      tab,
      index,
      left: Math.max(MENU_MARGIN, left),
      top: Math.max(MENU_MARGIN, top),
    });
  }

  protected closeMenu(): void {
    this.menu.set(null);
  }

  protected hasSavedTabs(): boolean {
    return this.tabs.tabs().some((tab) => !tab.dirty);
  }

  /** Closes the tabs a menu entry targets, relative to the tab clicked. */
  protected async closeFromMenu(target: CloseTarget): Promise<void> {
    const menu = this.menu();
    this.closeMenu();
    if (!menu) {
      return;
    }
    const tabs = this.tabs.tabs();
    const index = tabs.findIndex((tab) => tab.path === menu.tab.path);
    if (index < 0) {
      return;
    }

    const targets: Record<CloseTarget, RequestTab[]> = {
      this: [tabs[index]],
      others: tabs.filter((_tab, position) => position !== index),
      left: tabs.slice(0, index),
      right: tabs.slice(index + 1),
      saved: tabs.filter((tab) => !tab.dirty),
      all: tabs,
    };
    await this.closeTabs(targets[target]);
  }

  /** Closes tabs, asking once when some of them hold unsaved changes. */
  private async closeTabs(closing: RequestTab[]): Promise<void> {
    if (closing.length === 0) {
      return;
    }
    if (closing.length === 1) {
      await this.close(closing[0]);
      return;
    }
    const dirty = closing.filter((tab) => tab.dirty);
    if (dirty.length > 0) {
      const names = dirty.map((tab) => `“${tab.name}”`).join(', ');
      const confirmed = await this.confirmation.ask({
        title: `Close ${closing.length} tabs without saving?`,
        message: `Unsaved changes will be lost in ${names}.`,
        confirmLabel: 'Discard changes',
        danger: true,
      });
      if (!confirmed) {
        return;
      }
    }
    this.tabs.closeMany(closing.map((tab) => tab.path));
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
    if (event.key === 'Escape' && this.menu()) {
      this.closeMenu();
      return;
    }
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
