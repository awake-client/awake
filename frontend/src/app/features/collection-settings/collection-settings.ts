import { Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';
import * as CollectionsService from '@bindings/github.com/awake-client/awake/src/features/collections/service';
import {
  Authentication,
  AuthenticationType,
  Collection,
  OAuth2GrantType,
} from '@bindings/github.com/awake-client/awake/src/domain/models';
import { notifyCollectionsChanged } from '@core/collection-events';
import { Shortcuts } from '@core/shortcuts';
import { VariableStore } from '@core/variable-store';
import { WorkspaceStore } from '@core/workspace-store';
import { AuthEditor } from '@shared/auth-editor';
import { ColorPicker } from '@shared/color-picker';
import { Icon } from '@shared/icon';
import { VariableInput } from '@shared/variable-input';

type SettingsTab = 'overview' | 'variables' | 'authentication';

interface VariableRow {
  name: string;
  value: string;
}

/**
 * Settings of a collection: its name, documentation, variables and the
 * authentication inherited by its requests. Everything is stored in the
 * collection.yaml file of the collection.
 */
@Component({
  selector: 'app-collection-settings',
  imports: [AuthEditor, ColorPicker, Icon, RouterLink, VariableInput],
  host: {
    '(document:keydown)': 'handleShortcut($event)',
  },
  templateUrl: './collection-settings.html',
  styleUrl: './collection-settings.css',
})
export class CollectionSettings {
  private readonly route = inject(ActivatedRoute);
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly variableStore = inject(VariableStore);
  protected readonly shortcuts = inject(Shortcuts);

  protected readonly collectionPath = signal('');
  protected readonly tab = signal<SettingsTab>('overview');
  protected readonly name = signal('');
  protected readonly documentation = signal('');
  protected readonly color = signal('');
  protected readonly variables = signal<VariableRow[]>([]);
  protected readonly authentication = signal<Authentication>(emptyAuthentication());
  protected readonly isLoading = signal(true);
  protected readonly isSaving = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly statusMessage = signal<string | null>(null);
  private readonly snapshot = signal('');

  protected readonly isPrivate = computed(() => this.collectionPath().startsWith('local/'));

  private readonly draft = computed<Collection>(() => {
    const variables: Record<string, string> = {};
    for (const variable of this.variables()) {
      if (variable.name.trim() !== '') {
        variables[variable.name.trim()] = variable.value;
      }
    }
    const authentication = this.authentication();
    return {
      Name: this.name().trim(),
      Documentation: this.documentation(),
      Color: this.color(),
      Variables: Object.keys(variables).length > 0 ? variables : null,
      Authentication:
        authentication.Type === AuthenticationType.AuthenticationNone ? null : authentication,
    };
  });

  protected readonly isDirty = computed(() => JSON.stringify(this.draft()) !== this.snapshot());

  constructor() {
    inject(DestroyRef).onDestroy(() => this.variableStore.useCollection(null));
    this.route.url.pipe(takeUntilDestroyed()).subscribe((segments) => {
      const path = segments.map((segment) => segment.path).join('/');
      if (path) {
        void this.load(path);
      }
    });
    this.route.queryParamMap.pipe(takeUntilDestroyed()).subscribe((query) => {
      const tab = query.get('tab');
      if (tab === 'overview' || tab === 'variables' || tab === 'authentication') {
        this.tab.set(tab);
      }
    });
  }

  protected addVariable(): void {
    this.variables.update((variables) => [...variables, { name: '', value: '' }]);
  }

  protected updateVariable(index: number, field: keyof VariableRow, value: string): void {
    this.variables.update((variables) =>
      variables.map((variable, position) =>
        position === index ? { ...variable, [field]: value } : variable,
      ),
    );
  }

  protected removeVariable(index: number): void {
    this.variables.update((variables) => variables.filter((_, position) => position !== index));
  }

  protected inputValue(event: Event): string {
    return (event.target as HTMLInputElement | HTMLTextAreaElement).value;
  }

  protected async save(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const draft = this.draft();
    if (!rootPath || this.isSaving()) {
      return;
    }
    if (draft.Name === '') {
      this.errorMessage.set('A collection needs a name.');
      return;
    }

    this.isSaving.set(true);
    this.errorMessage.set(null);
    this.statusMessage.set(null);
    try {
      await CollectionsService.UpdateCollection(rootPath, this.collectionPath(), draft);
      this.snapshot.set(JSON.stringify(this.draft()));
      this.statusMessage.set('Saved');
      notifyCollectionsChanged();
    } catch (error) {
      this.errorMessage.set(error instanceof Error ? error.message : String(error));
    } finally {
      this.isSaving.set(false);
    }
  }

  protected handleShortcut(event: KeyboardEvent): void {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's') {
      event.preventDefault();
      if (this.isDirty()) {
        void this.save();
      }
    }
  }

  private async load(path: string): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    this.collectionPath.set(path);
    // Highlight variables as the requests of this collection will see them.
    this.variableStore.useCollection(path);
    this.isLoading.set(true);
    this.errorMessage.set(null);
    this.statusMessage.set(null);
    if (!rootPath) {
      return;
    }

    try {
      const collection = await CollectionsService.LoadCollection(rootPath, path);
      this.name.set(collection.Name);
      this.documentation.set(collection.Documentation ?? '');
      this.color.set(collection.Color ?? '');
      this.variables.set(
        Object.entries(collection.Variables ?? {}).map(([name, value]) => ({
          name,
          value: value ?? '',
        })),
      );
      this.authentication.set({ ...emptyAuthentication(), ...(collection.Authentication ?? {}) });
      this.snapshot.set(JSON.stringify(this.draft()));
    } catch (error) {
      this.errorMessage.set(error instanceof Error ? error.message : String(error));
    } finally {
      this.isLoading.set(false);
    }
  }
}

function emptyAuthentication(): Authentication {
  return {
    Type: AuthenticationType.AuthenticationNone,
    Username: '',
    Password: '',
    Token: '',
    Key: '',
    Value: '',
    Location: 'header',
    GrantType: OAuth2GrantType.OAuth2GrantAuthorizationCode,
    AuthorizationURL: '',
    TokenURL: '',
    ClientID: '',
    ClientSecret: '',
    Scopes: [],
    RedirectURL: '',
    Audience: '',
  };
}
