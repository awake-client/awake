import { Component, input, output } from '@angular/core';

/**
 * A thin bar the user drags to resize two panes. It reports the pointer
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
  template: '<span class="grip"></span>',
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
    .grip {
      position: absolute;
      border-radius: 2px;
      background: transparent;
      transition: background-color 120ms ease;
    }
    :host(.vertical) .grip {
      top: 0;
      bottom: 0;
      left: 3px;
      width: 3px;
    }
    :host(.horizontal) .grip {
      left: 0;
      right: 0;
      top: 3px;
      height: 3px;
    }
    :host(:hover) .grip,
    :host(.dragging) .grip {
      background: var(--color-accent);
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
