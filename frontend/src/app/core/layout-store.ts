import { Injectable, signal } from '@angular/core';

const STORAGE_KEY = 'awake.layout';

/** Where the response sits relative to the request. */
export type EditorLayout = 'stacked' | 'side';

interface StoredLayout {
  sidebarWidth: number;
  editorLayout: EditorLayout;
  responseRatio: number;
}

const DEFAULTS: StoredLayout = { sidebarWidth: 272, editorLayout: 'stacked', responseRatio: 0.4 };
/** The minimum keeps the footer, with its sync status, readable. */
export const SIDEBAR_WIDTH = { minimum: 264, maximum: 560, default: DEFAULTS.sidebarWidth };
export const RESPONSE_RATIO = { minimum: 0.18, maximum: 0.82, default: DEFAULTS.responseRatio };

/**
 * Personal layout preferences: the sidebar width, whether the response sits
 * below or beside the request, and the share of the editor it takes. They
 * are remembered on this computer only.
 */
@Injectable({ providedIn: 'root' })
export class LayoutStore {
  private readonly stored = readStorage();

  readonly sidebarWidth = signal(this.stored.sidebarWidth);
  readonly editorLayout = signal<EditorLayout>(this.stored.editorLayout);
  readonly responseRatio = signal(this.stored.responseRatio);

  setSidebarWidth(width: number): void {
    this.sidebarWidth.set(clamp(Math.round(width), SIDEBAR_WIDTH.minimum, SIDEBAR_WIDTH.maximum));
    this.persist();
  }

  setResponseRatio(ratio: number): void {
    this.responseRatio.set(clamp(ratio, RESPONSE_RATIO.minimum, RESPONSE_RATIO.maximum));
    this.persist();
  }

  toggleEditorLayout(): void {
    this.editorLayout.update((layout) => (layout === 'stacked' ? 'side' : 'stacked'));
    this.persist();
  }

  private persist(): void {
    const layout: StoredLayout = {
      sidebarWidth: this.sidebarWidth(),
      editorLayout: this.editorLayout(),
      responseRatio: this.responseRatio(),
    };
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(layout));
    } catch {
      // The layout still applies to this session.
    }
  }
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value));
}

function readStorage(): StoredLayout {
  try {
    const parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}') as Partial<StoredLayout>;
    return {
      sidebarWidth:
        typeof parsed.sidebarWidth === 'number'
          ? clamp(parsed.sidebarWidth, SIDEBAR_WIDTH.minimum, SIDEBAR_WIDTH.maximum)
          : DEFAULTS.sidebarWidth,
      editorLayout: parsed.editorLayout === 'side' ? 'side' : 'stacked',
      responseRatio:
        typeof parsed.responseRatio === 'number' ? parsed.responseRatio : DEFAULTS.responseRatio,
    };
  } catch {
    return { ...DEFAULTS };
  }
}
