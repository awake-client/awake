import {
  Component,
  DestroyRef,
  computed,
  effect,
  inject,
  signal,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import * as CollectionsService from '@bindings/github.com/clementherve/awake/src/features/collections/service';
import { Entry } from '@bindings/github.com/clementherve/awake/src/features/collections/models';
import * as EnvironmentsService from '@bindings/github.com/clementherve/awake/src/features/environments/service';
import { Summary } from '@bindings/github.com/clementherve/awake/src/features/environments/models';
import { WorkspaceStore } from '@core/workspace-store';

const COLLECTIONS_DIRECTORY = 'collections';

/** Workspace overview shown in the editor area until a request is selected. */
@Component({
  selector: 'app-workspace-welcome',
  imports: [FormsModule, RouterLink],
  templateUrl: './workspace-welcome.html',
  styleUrl: './workspace-welcome.css',
})
export class WorkspaceWelcome {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly workspaceName = computed(
    () => this.workspaceStore.workspace()?.configuration.Name ?? 'Workspace',
  );
  protected readonly rootPath = computed(
    () => this.workspaceStore.rootPath() ?? '',
  );

  protected readonly collections = signal<Entry[]>([]);
  protected readonly environments = signal<Summary[]>([]);
  protected readonly selectedCollection = signal<Entry | null>(null);
  protected readonly selectedDocumentation = signal('');
  protected readonly isLoading = signal(false);
  protected readonly errorMessage = signal<string | null>(null);

  protected readonly isCreating = signal(false);
  protected readonly creationName = signal('');
  protected readonly creationError = signal<string | null>(null);
  protected readonly isSaving = signal(false);

  constructor() {
    effect(() => {
      const rootPath = this.rootPath();
      if (rootPath) {
        void this.load(rootPath);
      }
    });

    const reload = () => {
      const rootPath = this.rootPath();
      if (rootPath) {
        void this.load(rootPath);
      }
    };

    window.addEventListener('awake:environments-changed', reload);
    window.addEventListener('awake:collections-changed', reload);
    this.destroyRef.onDestroy(() => {
      window.removeEventListener('awake:environments-changed', reload);
      window.removeEventListener('awake:collections-changed', reload);
    });
  }

  protected async load(rootPath: string): Promise<void> {
    this.isLoading.set(true);
    this.errorMessage.set(null);

    try {
      const [collections, environments] = await Promise.all([
        CollectionsService.List(rootPath, ''),
        EnvironmentsService.List(rootPath),
      ]);
      this.collections.set(collections ?? []);
      this.environments.set(environments ?? []);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  protected async openCollection(entry: Entry): Promise<void> {
    this.selectedCollection.set(entry);
    this.selectedDocumentation.set('');

    const rootPath = this.rootPath();
    if (!rootPath) {
      return;
    }

    try {
      const collection = await CollectionsService.LoadCollection(
        rootPath,
        entry.Path,
      );
      this.selectedDocumentation.set(collection.Documentation ?? '');
    } catch {
      this.selectedDocumentation.set('');
    }

    window.dispatchEvent(
      new CustomEvent('awake:open-collection', { detail: entry.Path }),
    );
  }

  protected toggleCreate(): void {
    this.isCreating.update((creating) => !creating);
    this.creationError.set(null);
  }

  protected async createCollection(): Promise<void> {
    const rootPath = this.rootPath();
    const name = this.creationName().trim();
    const slug = slugify(name);
    if (!rootPath || slug === '') {
      this.creationError.set('The name needs at least one letter or digit.');
      return;
    }

    this.isSaving.set(true);
    this.creationError.set(null);

    try {
      await CollectionsService.CreateCollection(
        rootPath,
        `${COLLECTIONS_DIRECTORY}/${slug}`,
        {
          Name: name,
          Variables: null,
          Authentication: null,
          Documentation: '',
        },
      );
      this.creationName.set('');
      this.isCreating.set(false);
      await this.load(rootPath);
      window.dispatchEvent(new CustomEvent('awake:collections-changed'));
    } catch (error) {
      this.creationError.set(describeError(error));
    } finally {
      this.isSaving.set(false);
    }
  }

  protected collectionPath(entry: Entry): string {
    return `/${entry.Path}`;
  }
}

function slugify(value: string): string {
  return value
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
