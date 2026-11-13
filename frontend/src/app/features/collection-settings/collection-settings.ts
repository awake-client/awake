import { Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';
import * as CollectionsService from '@bindings/github.com/awake-client/awake/src/features/collections/service';
import {
  Authentication,
  AuthenticationType,
  Collection,
  Header,
  OAuth2GrantType,
} from '@bindings/github.com/awake-client/awake/src/domain/models';
import { notifyCollectionsChanged } from '@core/collection-events';
import { Shortcuts } from '@core/shortcuts';
import { VariableStore } from '@core/variable-store';
import { WorkspaceStore } from '@core/workspace-store';
import { AuthEditor } from '@shared/auth-editor';
import { OAuth2Actions } from '../oauth2/oauth2-actions';
import { ColorPicker } from '@shared/color-picker';
import { Icon } from '@shared/icon';
import { VariableInput } from '@shared/variable-input';

type SettingsTab = 'overview' | 'variables' | 'headers' | 'authentication';

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
  imports: [AuthEditor, ColorPicker, Icon, OAuth2Actions, RouterLink, VariableInput],
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
  protected readonly headers = signal<Header[]>([]);
  protected readonly authentication = signal<Authentication>(emptyAuthentication());
  /** What a folder inherits from the folders and collection above it. */
  protected readonly inheritedAuthentication = signal<Authentication | null>(null);
  protected readonly authenticationSourceName = signal('');
  protected readonly authenticationSourcePath = signal('');
  protected readonly isLoading = signal(true);
  protected readonly isSaving = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly statusMessage = signal<string | null>(null);
  private readonly snapshot = signal('');

  protected readonly isPrivate = computed(() => this.collectionPath().startsWith('local/'));

  /**
   * A path with three segments is a folder of a collection, with two the
   * collection itself.
   */
  protected readonly isFolder = computed(() => this.collectionPath().split('/').length > 2);

  private readonly draft = computed<Collection>(() => {
    const variables: Record<string, string> = {};
    for (const variable of this.variables()) {
      if (variable.name.trim() !== '') {
        variables[variable.name.trim()] = variable.value;
      }
    }
    const authentication = this.authentication();
    // A folder inheriting from its parent stores no authentication; choosing
    // None stores an explicit one so requests below stop inheriting.
    let storedAuthentication: Authentication | null = authentication;
    if (authentication.Type === AuthenticationType.AuthenticationInherit) {
      storedAuthentication = null;
    } else if (!this.isFolder() && authentication.Type === AuthenticationType.AuthenticationNone) {
      storedAuthentication = null;
    }
    const headers = this.headers().filter((header) => header.Name.trim() !== '');
    return {
      Name: this.name().trim(),
      Documentation: this.documentation(),
      Color: this.color(),
      Variables: Object.keys(variables).length > 0 ? variables : null,
      Headers: headers.length > 0 ? headers : null,
      Authentication: storedAuthentication,
    };
  });

  protected readonly isDirty = computed(() => JSON.stringify(this.draft()) !== this.snapshot());

  /** The authentication in effect: its own, or the one a folder inherits. */
  protected readonly effectiveAuthentication = computed(() => {
    const authentication = this.authentication();
    return authentication.Type === AuthenticationType.AuthenticationInherit
      ? this.inheritedAuthentication()
      : authentication;
  });
  /** Bumped when the settings are loaded or saved, so OAuth 2 status reloads. */
  protected readonly savedRevision = signal(0);
  /** Saves pending edits before an OAuth 2 flow reads the settings file. */
  protected readonly saveBeforeAuthorize = async (): Promise<boolean> => {
    if (this.isDirty()) {
      await this.save();
    }
    return !this.isDirty();
  };

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
      if (
        tab === 'overview' ||
        tab === 'variables' ||
        tab === 'headers' ||
        tab === 'authentication'
      ) {
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

  protected addHeader(): void {
    this.headers.update((headers) => [...headers, { Name: '', Value: '', Enabled: true }]);
  }

  protected updateHeader(
    index: number,
    field: 'Name' | 'Value' | 'Enabled',
    value: string | boolean,
  ): void {
    this.headers.update((headers) =>
      headers.map((header, position) =>
        position === index ? ({ ...header, [field]: value } as Header) : header,
      ),
    );
  }

  protected removeHeader(index: number): void {
    this.headers.update((headers) => headers.filter((_, position) => position !== index));
  }

  protected inputValue(event: Event): string {
    return (event.target as HTMLInputElement | HTMLTextAreaElement).value;
  }

  protected isChecked(event: Event): boolean {
    return (event.target as HTMLInputElement).checked;
  }

  protected async save(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const draft = this.draft();
    if (!rootPath || this.isSaving()) {
      return;
    }
    if (draft.Name === '') {
      this.errorMessage.set(`A ${this.isFolder() ? 'folder' : 'collection'} needs a name.`);
      return;
    }

    this.isSaving.set(true);
    this.errorMessage.set(null);
    this.statusMessage.set(null);
    try {
      await CollectionsService.UpdateCollection(rootPath, this.collectionPath(), draft);
      this.snapshot.set(JSON.stringify(this.draft()));
      this.savedRevision.update((revision) => revision + 1);
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
      this.headers.set(collection.Headers ?? []);
      this.authentication.set({
        ...emptyAuthentication(),
        ...(collection.Authentication ?? {}),
        ...(this.isFolder() && !collection.Authentication
          ? { Type: AuthenticationType.AuthenticationInherit }
          : {}),
      });
      if (this.isFolder()) {
        await this.loadInheritedAuthentication(rootPath, path);
      } else {
        this.resetInheritedAuthentication();
      }
      this.snapshot.set(JSON.stringify(this.draft()));
      this.savedRevision.update((revision) => revision + 1);
    } catch (error) {
      this.errorMessage.set(error instanceof Error ? error.message : String(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  private async loadInheritedAuthentication(rootPath: string, path: string): Promise<void> {
    try {
      const inherited = await CollectionsService.LoadInheritedAuthenticationForScope(
        rootPath,
        path,
      );
      this.inheritedAuthentication.set(inherited.Authentication);
      this.authenticationSourceName.set(inherited.SourceName);
      this.authenticationSourcePath.set(inherited.SourcePath);
    } catch {
      this.resetInheritedAuthentication();
    }
  }

  private resetInheritedAuthentication(): void {
    this.inheritedAuthentication.set(null);
    this.authenticationSourceName.set('');
    this.authenticationSourcePath.set('');
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
