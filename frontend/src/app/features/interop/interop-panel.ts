import { Component, computed, inject, signal } from '@angular/core';
import * as InteropService from '@bindings/github.com/clementherve/awake/src/features/interop/service';
import { Format } from '@bindings/github.com/clementherve/awake/src/features/interop/models';
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

/**
 * Imports external collections into the workspace and exports the workspace
 * collections to an interchange document.
 */
@Component({
  selector: 'app-interop-panel',
  imports: [Icon],
  templateUrl: './interop-panel.html',
  styleUrl: './interop-panel.css',
})
export class InteropPanel {
  private readonly workspaceStore = inject(WorkspaceStore);

  protected readonly formats = [Format.FormatOpenAPI, Format.FormatPostman, Format.FormatBruno];
  protected readonly formatLabels = FORMAT_LABELS;

  protected readonly importFormat = signal<Format>(Format.FormatOpenAPI);
  protected readonly importPath = signal('');
  protected readonly isImporting = signal(false);
  protected readonly importMessage = signal<string | null>(null);
  protected readonly importWarnings = signal<string[]>([]);
  protected readonly importError = signal<string | null>(null);

  protected readonly exportFormat = signal<Format>(Format.FormatOpenAPI);
  protected readonly exportPath = signal('');
  protected readonly isExporting = signal(false);
  protected readonly exportMessage = signal<string | null>(null);
  protected readonly exportPaths = signal<string[]>([]);
  protected readonly exportError = signal<string | null>(null);

  protected readonly importHint = computed(() => {
    switch (this.importFormat()) {
      case Format.FormatBruno:
        return 'Path to a Bruno collection directory (.bru or OpenCollection YAML).';
      case Format.FormatPostman:
        return 'Path to a Postman collection JSON file.';
      default:
        return 'Path to an OpenAPI JSON or YAML file.';
    }
  });

  protected readonly exportHint = computed(() => {
    switch (this.exportFormat()) {
      case Format.FormatBruno:
        return 'Directory that receives the .bru files.';
      case Format.FormatPostman:
        return 'File that receives the Postman collection.';
      default:
        return 'File that receives the OpenAPI document.';
    }
  });

  constructor() {
    const rootPath = this.workspaceStore.rootPath();
    if (rootPath) {
      this.exportPath.set(`${rootPath}/awake-export`);
    }
  }

  protected readonly importIsDirectory = computed(() => this.importFormat() === Format.FormatBruno);
  protected readonly exportIsDirectory = computed(() => this.exportFormat() === Format.FormatBruno);

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
          title: `Choose a ${FORMAT_LABELS[this.importFormat()]} document`,
          buttonText: 'Choose',
          types: [{ name: 'API documents', pattern: '*.json;*.yaml;*.yml' }],
        });
    if (path) {
      this.importPath.set(path);
      this.importError.set(null);
    }
  }

  /** Opens a folder chooser for Bruno, a save dialog for the other formats. */
  protected async browseExport(): Promise<void> {
    const format = this.exportFormat();
    const path = this.exportIsDirectory()
      ? await chooseDirectory({
          title: 'Choose where to write the Bruno collection',
          buttonText: 'Export here',
        })
      : await chooseSaveFile({
          title: `Save the ${FORMAT_LABELS[format]} export`,
          buttonText: 'Save',
          fileName: format === Format.FormatPostman ? 'collection.postman.json' : 'openapi.yaml',
        });
    if (path) {
      this.exportPath.set(path);
      this.exportError.set(null);
    }
  }

  protected selectExportFormat(format: Format): void {
    this.exportFormat.set(format);
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

      this.importMessage.set(
        `Imported ${result.Requests} request(s) into ${result.Collections} collection(s).`,
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
