import { Clipboard } from '@wailsio/runtime';

/**
 * Copies text with the native clipboard of the application. WebKit refuses the
 * web clipboard API once an await has run since the click, as when the text
 * first comes from Go, so it is never used.
 */
export function copyText(text: string): Promise<void> {
  return Clipboard.SetText(text);
}
