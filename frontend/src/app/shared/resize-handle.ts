import { Component, input, output } from '@angular/core';

/**
 * A bar the user drags to resize two panes, drawn as a hairline with a grip. It reports the pointer
 * position while dragging; the parent turns it into a size. A double click
 * asks for the default size.
 */
@Component({
  selector: 'app-resize-handle',
  host: {
    role: 'separator',
    tabindex: '-1',
    '[attr.aria-orientation]': "orientation() === 'vertical' ? 'vertical' : 'horizontal'",
    '[class.vertical]': "orientation() === 'vertical'",
    '[class.horizontal]': "orientation() === 'horizontal'",
    '(pointerdown)': 'start($event)',
    '(dblclick)': 'reset.emit()',
  },
  template: '<span class="line"></span><span class="grip"></span>',
  styles: `
    :host {
      position: relative;
      z-index: 5;
      flex: 0 0 auto;
      touch-action: none;
      --wails-draggable: no-drag;
    }
    /* A vertical bar separates side by side panes. */
    :host(.vertical) {
      width: 9px;
      margin: 0 -4px;
      cursor: col-resize;
    }
    :host(.horizontal) {
      height: 9px;
      margin: -4px 0;
      cursor: row-resize;
    }
    /* A hairline and a small grip, shown after a short hover so the cursor
       can cross the bar without it flashing. */
    .line,
    .grip {
      position: absolute;
      opacity: 0;
      transition:
        opacity 140ms ease,
        background-color 140ms ease;
      pointer-events: none;
    }
    .line {
      background: var(--color-border-strong);
    }
    .grip {
      border-radius: 999px;
      background: var(--color-text-subtle);
    }
    :host(.vertical) .line {
      top: 0;
      bottom: 0;
      left: 4px;
      width: 1px;
    }
    :host(.horizontal) .line {
      left: 0;
      right: 0;
      top: 4px;
      height: 1px;
    }
    :host(.vertical) .grip {
      top: 50%;
      left: 2px;
      width: 5px;
      height: 32px;
      transform: translateY(-50%);
    }
    :host(.horizontal) .grip {
      left: 50%;
      top: 2px;
      width: 32px;
      height: 5px;
      transform: translateX(-50%);
    }
    :host(:hover) .line,
    :host(:hover) .grip {
      opacity: 1;
      transition-delay: 150ms;
    }
    :host(.dragging) .line,
    :host(.dragging) .grip {
      opacity: 1;
      transition-delay: 0ms;
    }
    :host(.dragging) .line {
      background: color-mix(in srgb, var(--color-accent) 55%, transparent);
    }
    :host(.dragging) .grip {
      background: var(--color-accent);
    }
    @media (prefers-reduced-motion: reduce) {
      .line,
      .grip {
        transition: none;
      }
    }
  `,
})
export class ResizeHandle {
  readonly orientation = input<'vertical' | 'horizontal'>('vertical');
  /** Pointer position, in viewport coordinates, while dragging. */
  readonly resize = output<{ x: number; y: number }>();
  readonly reset = output<void>();

  protected start(event: PointerEvent): void {
    if (event.button !== 0) {
      return;
    }
    event.preventDefault();
    const handle = event.currentTarget as HTMLElement;
    handle.setPointerCapture(event.pointerId);
    handle.classList.add('dragging');
    document.body.style.cursor = this.orientation() === 'vertical' ? 'col-resize' : 'row-resize';

    const move = (moveEvent: PointerEvent) =>
      this.resize.emit({ x: moveEvent.clientX, y: moveEvent.clientY });
    const stop = () => {
      handle.classList.remove('dragging');
      document.body.style.cursor = '';
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', stop);
      handle.removeEventListener('pointercancel', stop);
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', stop);
    handle.addEventListener('pointercancel', stop);
  }
}
