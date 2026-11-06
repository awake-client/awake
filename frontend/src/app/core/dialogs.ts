import { Dialogs } from '@wailsio/runtime';

/** Options of the native directory chooser. */
export interface DirectoryDialogOptions {
  title?: string;
  buttonText?: string;
  /** Directory the dialog opens in. */
  directory?: string;
}

/**
 * Opens the native directory chooser and resolves with the chosen absolute
 * path, or `null` when the user cancels.
 */
export async function chooseDirectory(
  options: DirectoryDialogOptions = {},
): Promise<string | null> {
  const path = await Dialogs.OpenFile({
    CanChooseDirectories: true,
    CanChooseFiles: false,
    CanCreateDirectories: true,
    ResolvesAliases: true,
    AllowsMultipleSelection: false,
    Title: options.title,
    ButtonText: options.buttonText,
    Directory: options.directory || undefined,
  });
  return path ? path : null;
}
