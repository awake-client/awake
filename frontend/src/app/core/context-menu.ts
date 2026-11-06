/**
 * Hides the browser context menu (Back, Reload, Inspect…), which makes no
 * sense in a desktop application. It stays available where it is useful:
 * text fields, the code editor and selected text, for copy and paste.
 * Components that open their own menu call preventDefault themselves.
 */
export function suppressDefaultContextMenu(): void {
  document.addEventListener('contextmenu', (event) => {
    if (event.defaultPrevented) {
      return;
    }
    const target = event.target as HTMLElement | null;
    const editable =
      target instanceof HTMLInputElement ||
      target instanceof HTMLTextAreaElement ||
      target?.isContentEditable ||
      target?.closest('.cm-editor') !== null;
    const hasSelection = (window.getSelection()?.toString() ?? '').length > 0;
    if (!editable && !hasSelection) {
      event.preventDefault();
    }
  });
}
