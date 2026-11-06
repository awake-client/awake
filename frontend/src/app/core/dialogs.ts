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

/** A family of files offered by a file dialog, such as `*.json;*.yaml`. */
export interface FileType {
  name: string;
  pattern: string;
}

/** Opens the native file chooser; resolves with the path or `null`. */
export async function chooseFile(
  options: DirectoryDialogOptions & { types?: FileType[] } = {},
): Promise<string | null> {
  const path = await Dialogs.OpenFile({
    CanChooseDirectories: false,
    CanChooseFiles: true,
    ResolvesAliases: true,
    AllowsMultipleSelection: false,
    Title: options.title,
    ButtonText: options.buttonText,
    Directory: options.directory || undefined,
    Filters: options.types?.map((type) => ({ DisplayName: type.name, Pattern: type.pattern })),
  });
  return path ? path : null;
}

/** Opens the native save dialog; resolves with the chosen path or `null`. */
export async function chooseSaveFile(
  options: DirectoryDialogOptions & { fileName?: string; types?: FileType[] } = {},
): Promise<string | null> {
  const path = await Dialogs.SaveFile({
    CanCreateDirectories: true,
    Filename: options.fileName,
    Title: options.title,
    ButtonText: options.buttonText,
    Directory: options.directory || undefined,
    Filters: options.types?.map((type) => ({ DisplayName: type.name, Pattern: type.pattern })),
  });
  return path ? path : null;
}
