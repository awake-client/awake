import { ChangeDetectionStrategy, Component, OnDestroy, signal } from '@angular/core';
import { Events, Window } from '@wailsio/runtime';
import { isWindowsPlatform } from '@core/platform';

const maximiseEvent = 'common:WindowMaximise';
const unmaximiseEvent = 'common:WindowUnMaximise';

/**
 * The minimize, restore and close buttons of the frameless Windows window.
 * Windows cannot repaint its native title bar when the application theme
 * changes, so the window drops its frame there and this component draws the
 * caption controls in the web title bar. It stays hidden on the other
 * platforms, where the operating system draws them.
 */
@Component({
  selector: 'app-window-controls',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    '[class.windows]': 'windows',
  },
  template: `
    <button
      type="button"
      class="control"
      title="Minimize"
      aria-label="Minimize"
      (click)="minimise()"
    >
      <svg viewBox="0 0 10 10" aria-hidden="true"><path d="M0 5h10" /></svg>
    </button>

    <button
      type="button"
      class="control"
      [title]="maximised() ? 'Restore' : 'Maximize'"
      [attr.aria-label]="maximised() ? 'Restore' : 'Maximize'"
      (click)="toggleMaximise()"
    >
      @if (maximised()) {
        <svg viewBox="0 0 10 10" aria-hidden="true">
          <path d="M2 2h6v6" />
          <path d="M4 4h4v4H4Z" />
        </svg>
      } @else {
        <svg viewBox="0 0 10 10" aria-hidden="true"><path d="M0.5 0.5h9v9h-9Z" /></svg>
      }
    </button>

    <button type="button" class="control close" title="Close" aria-label="Close" (click)="close()">
      <svg viewBox="0 0 10 10" aria-hidden="true"><path d="m0.5 0.5 9 9M9.5 0.5l-9 9" /></svg>
    </button>
  `,
  styles: `
    :host {
      display: none;
    }

    /* The buttons sit in the corner the native caption would occupy, above
       every screen and dialog, so they stay reachable like the OS controls. */
    :host(.windows) {
      position: fixed;
      top: 0;
      right: 0;
      z-index: 1000;
      display: flex;
      height: var(--titlebar-height);
      --wails-draggable: no-drag;
    }

    /* A fullscreen window has no title bar and no caption buttons. */
    :host-context(html[data-fullscreen='true']) {
      display: none;
    }

    .control {
      display: grid;
      place-items: center;
      width: 46px;
      height: 100%;
      padding: 0;
      border: 0;
      background: transparent;
      color: var(--color-text-muted);
      cursor: default;
      transition:
        background-color 100ms ease,
        color 100ms ease;
    }

    .control svg {
      width: 10px;
      height: 10px;
      fill: none;
      stroke: currentColor;
      stroke-width: 1;
    }

    .control:hover {
      background: var(--color-hover);
      color: var(--color-text-strong);
    }

    .control:active {
      background: var(--color-active);
    }

    /* Closing is the one destructive caption button: Windows paints it red. */
    .control.close:hover {
      background: #c42b1c;
      color: #ffffff;
    }

    .control.close:active {
      background: #b0261a;
    }
  `,
})
export class WindowControls implements OnDestroy {
  protected readonly windows = isWindowsPlatform();
  protected readonly maximised = signal(false);
  private readonly stopListening: Array<() => void> = [];

  constructor() {
    if (!this.windows) {
      return;
    }

    void Window.IsMaximised()
      .then((maximised) => this.maximised.set(maximised))
      .catch(() => undefined);
    this.stopListening.push(
      Events.On(maximiseEvent, () => this.maximised.set(true)),
      Events.On(unmaximiseEvent, () => this.maximised.set(false)),
    );

    document.addEventListener('dblclick', this.maximiseOnTitleBarDoubleClick);
  }

  ngOnDestroy(): void {
    for (const stop of this.stopListening) {
      stop();
    }
    document.removeEventListener('dblclick', this.maximiseOnTitleBarDoubleClick);
  }

  protected minimise(): void {
    void Window.Minimise().catch(() => undefined);
  }

  protected toggleMaximise(): void {
    void Window.ToggleMaximise().catch(() => undefined);
  }

  protected close(): void {
    void Window.Close().catch(() => undefined);
  }

  /**
   * The frameless drag region is a web element, so the double click that
   * maximizes a native title bar never reaches Windows. Reproduce it, but only
   * on the areas that declared themselves as drag regions.
   */
  private readonly maximiseOnTitleBarDoubleClick = (event: MouseEvent): void => {
    const target = event.target instanceof Element ? event.target : null;
    if (!target || !isDragRegion(target)) {
      return;
    }
    event.preventDefault();
    this.toggleMaximise();
  };
}

function isDragRegion(target: Element): boolean {
  return getComputedStyle(target).getPropertyValue('--wails-draggable').trim() === 'drag';
}
