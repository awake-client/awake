import { Injectable } from '@angular/core';
import { isMacPlatform } from './platform';

/**
 * Keyboard shortcuts as the interface shows them. macOS uses the Apple key,
 * Windows and Linux use Control, which is what people expect on each system.
 * The handlers accept both modifiers; only the labels change.
 */
@Injectable({ providedIn: 'root' })
export class Shortcuts {
  readonly isMac = isMacPlatform();
  /** The command modifier alone: ⌘ on macOS, Ctrl elsewhere. */
  readonly command = this.isMac ? '⌘' : 'Ctrl';
  /** The key that confirms: ↵ everywhere, as the palette already shows it. */
  readonly enter = '↵';

  /** One key with the command modifier, such as ⌘P or Ctrl+P. */
  key(key: string): string {
    return this.isMac ? `⌘${key}` : `Ctrl+${key}`;
  }
}
