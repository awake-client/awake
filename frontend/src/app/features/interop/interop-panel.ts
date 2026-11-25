import { Component, computed, inject, signal } from '@angular/core';
import * as InteropService from '@bindings/github.com/awake-client/awake/src/features/interop/service';
import {
  ExportOptions,
  Format,
  PluginFormat,
  PluginFormatList,
} from '@bindings/github.com/awake-client/awake/src/features/interop/models';
import { notifyCollectionsChanged } from '@core/collection-events';
import { chooseDirectory, chooseFile, chooseSaveFile } from '@core/dialogs';
import { WorkspaceStore } from '@core/workspace-store';
import { Icon } from '@shared/icon';

const FORMAT_LABELS: Record<Format, string> = {
  [Format.$zero]: '',
  [Format.FormatOpenAPI]: 'OpenAPI',
  [Format.FormatPostman]: 'Postman',
  [Format.FormatBruno]: 'Bruno',
};

const BUILT_IN_FORMATS = [Format.FormatOpenAPI, Format.FormatPostman, Format.FormatBruno];

/** Whether a format is provided by a plugin, named plugin:<id>/<format>. */
export function isPluginFormat(format: string): boolean {
  return format.startsWith('plugin:');
}

/** The file chooser pattern of a plugin format, such as *.har;*.json. */
export function extensionPattern(extensions: string[] | null | undefined): string {
  const patterns = (extensions ?? []).map((extension) => `*${extension}`);
  return patterns.length > 0 ? patterns.join(';') : '*.*';
}

/**
 * Imports external collections into the workspace and exports the workspace
 * collections to an interchange document. Enabled plugins add formats.
 */
@Component({
  selector: 'app-interop-panel',
  imports: [Icon],
  templateUrl: './interop-panel.html',
  styleUrl: './interop-panel.css',
})
export class InteropPanel {
  private readonly workspaceStore = inject(WorkspaceStore);

  private readonly pluginFormats = signal<PluginFormatList>({ imports: [], exports: [] });
  protected readonly importFormats = computed(() => [
    ...BUILT_IN_FORMATS,
    ...(this.pluginFormats().imports ?? []).map((format) => format.format),
  ]);
  protected readonly exportFormats = computed(() => [
    ...BUILT_IN_FORMATS,
    ...(this.pluginFormats().exports ?? []).map((format) => format.format),
  ]);

  /** The label of a built-in or plugin format. */
  protected formatLabel(format: Format): string {
    return FORMAT_LABELS[format] ?? this.pluginFormat(format)?.label ?? format;
  }

  private pluginFormat(format: Format): PluginFormat | undefined {
    const formats = [
      ...(this.pluginFormats().imports ?? []),
      ...(this.pluginFormats().exports ?? []),
    ];
    return formats.find((candidate) => candidate.format === format);
  }

  protected readonly importFormat = signal<Format>(Format.FormatOpenAPI);
  protected readonly importPath = signal('');
  protected readonly isImporting = signal(false);
  protected readonly importMessage = signal<string | null>(null);
  protected readonly importWarnings = signal<string[]>([]);
  protected readonly importError = signal<string | null>(null);

  protected readonly exportFormat = signal<Format>(Format.FormatOpenAPI);
  /** Writes one document per collection instead of one unified document. */
  protected readonly exportSplit = signal(false);
  protected readonly exportPath = signal('');
  protected readonly isExporting = signal(false);
  protected readonly exportMessage = signal<string | null>(null);
  protected readonly exportPaths = signal<string[]>([]);
  protected readonly exportError = signal<string | null>(null);

  protected readonly importHint = computed(() => {
    const format = this.importFormat();
    if (isPluginFormat(format)) {
      return `Path to a ${this.formatLabel(format)} document, read by its plugin.`;
    }
    switch (format) {
      case Format.FormatBruno:
        return 'Path to a Bruno collection directory (.bru or OpenCollection YAML).';
      case Format.FormatPostman:
        return 'Path to a Postman collection JSON file.';
      default:
        return 'Path to an OpenAPI JSON or YAML file.';
    }
  });

  protected readonly exportHint = computed(() => {
    if (isPluginFormat(this.exportFormat())) {
      return `File that receives the ${this.formatLabel(this.exportFormat())} document, written by its plugin.`;
    }
    if (this.exportFormat() === Format.FormatBruno) {
      return 'Directory that receives one folder per collection, with their environments.';
    }
    if (this.exportSplit()) {
      return this.exportFormat() === Format.FormatPostman
        ? 'Directory that receives one Postman collection per Awake collection, plus environments.'
        : 'Directory that receives one OpenAPI document per collection.';
    }
    if (this.exportFormat() === Format.FormatPostman) {
      return 'File that receives the collection. Environments go to a .environments folder beside it.';
    }
    return 'File that receives the OpenAPI document.';
  });

  constructor() {
    const rootPath = this.workspaceStore.rootPath();
    if (rootPath) {
      this.exportPath.set(`${rootPath}/awake-export`);
      void InteropService.PluginFormats(rootPath)
        .then((formats) => this.pluginFormats.set(formats))
        .catch(() => this.pluginFormats.set({ imports: [], exports: [] }));
    }
  }

  protected readonly importIsDirectory = computed(() => this.importFormat() === Format.FormatBruno);
  /** OpenAPI and Postman can split by collection; Bruno always does. */
  protected readonly exportSupportsSplit = computed(
    () => this.exportFormat() !== Format.FormatBruno && !isPluginFormat(this.exportFormat()),
  );
  protected readonly exportIsDirectory = computed(
    () => this.exportFormat() === Format.FormatBruno || this.exportSplit(),
  );

  protected selectImportFormat(format: Format): void {
    this.importFormat.set(format);
  }

  protected inputValue(event: Event): string {
    return (event.target as HTMLInputElement).value;
  }

  /** Opens a folder chooser for Bruno, a file chooser for the other formats. */
  protected async browseImport(): Promise<void> {
    const path = this.importIsDirectory()
      ? await chooseDirectory({ title: 'Choose a Bruno collection', buttonText: 'Import' })
      : await chooseFile({
          title: `Choose a ${this.formatLabel(this.importFormat())} document`,
          buttonText: 'Choose',
          types: [
            {
              name: 'API documents',
              pattern: isPluginFormat(this.importFormat())
                ? extensionPattern(this.pluginFormat(this.importFormat())?.extensions)
                : '*.json;*.yaml;*.yml',
            },
          ],
        });
    if (path) {
      this.importPath.set(path);
      this.importError.set(null);
    }
  }

  /** Opens a folder chooser for a directory export, a save dialog otherwise. */
  protected async browseExport(): Promise<void> {
    const format = this.exportFormat();
    const path = this.exportIsDirectory()
      ? await chooseDirectory({
          title:
            format === Format.FormatBruno
              ? 'Choose where to write the Bruno collections'
              : 'Choose where to write the collections',
          buttonText: 'Export here',
        })
      : await chooseSaveFile({
          title: `Save the ${this.formatLabel(format)} export`,
          buttonText: 'Save',
          fileName: this.exportFileName(format),
        });
    if (path) {
      this.exportPath.set(path);
      this.exportError.set(null);
    }
  }

  private exportFileName(format: Format): string {
    if (isPluginFormat(format)) {
      return `awake-export${this.pluginFormat(format)?.extensions?.[0] ?? '.txt'}`;
    }
    return format === Format.FormatPostman ? 'collection.postman.json' : 'openapi.yaml';
  }

  protected selectExportFormat(format: Format): void {
    this.exportFormat.set(format);
    if (format === Format.FormatBruno || isPluginFormat(format)) {
      // Bruno always writes one directory per collection; a plugin writes
      // one document.
      this.exportSplit.set(false);
    }
  }

  protected setExportSplit(split: boolean): void {
    this.exportSplit.set(split);
    this.exportError.set(null);
  }

  protected async importDocument(): Promise<void> {
    const workspaceRoot = this.workspaceStore.rootPath();
    const sourcePath = this.importPath().trim();
    if (!workspaceRoot || this.isImporting()) {
      return;
    }
    if (sourcePath === '') {
      this.importError.set('Choose the document to import.');
      return;
    }

    this.isImporting.set(true);
    this.importError.set(null);
    this.importMessage.set(null);
    this.importWarnings.set([]);

    try {
      const result = await InteropService.Import(workspaceRoot, sourcePath, this.importFormat());

      const environments = result.Environments ? ` and ${result.Environments} environment(s)` : '';
      this.importMessage.set(
        `Imported ${result.Requests} request(s)${environments} into ${result.Collections} collection(s).`,
      );
      this.importWarnings.set(result.Warnings ?? []);
      this.importPath.set('');
      notifyCollectionsChanged();
    } catch (error) {
      this.importError.set(describeError(error));
    } finally {
      this.isImporting.set(false);
    }
  }

  protected async exportDocument(): Promise<void> {
    const workspaceRoot = this.workspaceStore.rootPath();
    const destinationPath = this.exportPath().trim();
    if (!workspaceRoot || this.isExporting()) {
      return;
    }
    if (destinationPath === '') {
      this.exportError.set('Choose where to write the export.');
      return;
    }

    this.isExporting.set(true);
    this.exportError.set(null);
    this.exportMessage.set(null);
    this.exportPaths.set([]);

    try {
      const result = await InteropService.Export(
        workspaceRoot,
        destinationPath,
        this.exportFormat(),
        {
          SplitCollections: this.exportSplit(),
        } as ExportOptions,
      );

      const paths = result.Paths ?? [];
      this.exportPaths.set(paths);
      this.exportMessage.set(
        paths.length === 1 ? 'Export written.' : `Export written as ${paths.length} files.`,
      );
    } catch (error) {
      this.exportError.set(describeError(error));
    } finally {
      this.isExporting.set(false);
    }
  }
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
