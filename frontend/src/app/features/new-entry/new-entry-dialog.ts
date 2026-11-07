import {
  Component,
  DestroyRef,
  ElementRef,
  computed,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { slugify } from '@shared/slug';
import { Router } from '@angular/router';
import * as EnvironmentsService from '@bindings/github.com/clementherve/awake/src/features/environments/service';
import * as CollectionsService from '@bindings/github.com/clementherve/awake/src/features/collections/service';
import {
  Entry,
  Kind,
} from '@bindings/github.com/clementherve/awake/src/features/collections/models';
import { Protocol, Request } from '@bindings/github.com/clementherve/awake/src/domain/models';
import {
  CREATE_ENTRY_EVENT,
  CreationKind,
  CreationRequest,
  notifyCollectionsChanged,
} from '@core/collection-events';
import { EnvironmentStore } from '@core/environment-store';
import { TabsStore } from '@core/tabs-store';
import { WorkspaceStore } from '@core/workspace-store';
import { ColorPicker } from '@shared/color-picker';
import { Icon, IconName } from '@shared/icon';
import { ParsedRequestInput, isCurlCommand, parseRequestInput } from './request-input';

const COLLECTIONS_DIRECTORY = 'collections';
/** Collections kept on this computer only; the workspace ignores it in git. */
const PRIVATE_DIRECTORY = 'local';
const REQUEST_FILE_EXTENSION = '.yaml';
/** Upper bound on folders listed in the location picker. */
const MAX_FOLDERS = 1000;

const HTTP_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'];

/** How the request is described in the dialog. */
type RequestType = 'http' | 'sse' | 'websocket' | 'curl';

interface RequestTypeOption {
  type: RequestType;
  label: string;
  hint: string;
  icon: IconName;
}

const REQUEST_TYPES: RequestTypeOption[] = [
  { type: 'http', label: 'HTTP', hint: 'REST call', icon: 'send' },
  { type: 'sse', label: 'Events', hint: 'Server-sent', icon: 'layers' },
  { type: 'websocket', label: 'WebSocket', hint: 'Live socket', icon: 'switch' },
  { type: 'curl', label: 'From cURL', hint: 'Paste & go', icon: 'terminal' },
];

const KIND_ICONS: Record<CreationKind, IconName> = {
  request: 'send',
  folder: 'folder',
  collection: 'box',
  environment: 'globe',
};

const NAME_PLACEHOLDERS: Record<CreationKind, string> = {
  request: 'Get users',
  folder: 'Invoices',
  collection: 'Platform API',
  environment: 'Staging',
};

/** A directory that can receive the new entry. */
interface FolderOption {
  path: string;
  label: string;
  depth: number;
}

/**
 * Modal dialog creating requests, folders and collections. The request field
 * is smart: it recognises a pasted cURL command, a `METHOD url` pair or a
 * WebSocket URL and fills the rest of the form accordingly.
 */
@Component({
  selector: 'app-new-entry-dialog',
  imports: [FormsModule, Icon, ColorPicker],
  templateUrl: './new-entry-dialog.html',
  styleUrl: './new-entry-dialog.css',
})
export class NewEntryDialog {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly router = inject(Router);
  private readonly tabsStore = inject(TabsStore);
  private readonly environmentStore = inject(EnvironmentStore);
  private readonly dialog = viewChild<ElementRef<HTMLDialogElement>>('dialog');
  private readonly nameField = viewChild<ElementRef<HTMLInputElement>>('nameField');
  private readonly requestField =
    viewChild<ElementRef<HTMLInputElement | HTMLTextAreaElement>>('requestField');

  protected readonly requestTypes = REQUEST_TYPES;
  protected readonly httpMethods = HTTP_METHODS;

  protected readonly kind = signal<CreationKind>('request');
  protected readonly folders = signal<FolderOption[]>([]);
  protected readonly location = signal('');
  protected readonly requestType = signal<RequestType>('http');
  protected readonly method = signal('GET');
  protected readonly requestText = signal('');
  protected readonly name = signal('');
  protected readonly fileName = signal('');
  protected readonly showOptions = signal(false);
  /** Whether a new collection is shared through git or kept private. */
  protected readonly isPrivate = signal(false);
  /** Color of a new collection or environment. */
  protected readonly color = signal('');
  /** Scope of a new environment: empty for global, else a collection path. */
  protected readonly environmentScope = signal('');
  protected readonly collectionScopes = signal<FolderOption[]>([]);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly isSaving = signal(false);

  private nameEdited = false;
  private readonly fileNameEdited = signal(false);

  protected readonly title = computed(() => {
    switch (this.kind()) {
      case 'request':
        return 'New request';
      case 'folder':
        return 'New folder';
      case 'environment':
        return 'New environment';
      default:
        return 'New collection';
    }
  });

  protected readonly parsed = computed<ParsedRequestInput>(() =>
    parseRequestInput(this.requestText()),
  );

  protected readonly kindIcon = computed<IconName>(() => KIND_ICONS[this.kind()]);
  protected readonly namePlaceholder = computed(() => NAME_PLACEHOLDERS[this.kind()]);

  /** One line telling the user what the smart field understood. */
  protected readonly detection = computed(() => {
    const parsed = this.parsed();
    if (parsed.kind === 'empty') {
      return null;
    }
    if (parsed.kind === 'curl') {
      const parts = [`cURL command`, parsed.method];
      if (parsed.headers.length > 0) {
        parts.push(`${parsed.headers.length} header${parsed.headers.length > 1 ? 's' : ''}`);
      }
      if (parsed.body) {
        parts.push(`${parsed.body.Type} body`);
      }
      if (parsed.authentication) {
        parts.push('basic auth');
      }
      if (parsed.protocol === Protocol.ProtocolSSE) {
        parts.push('event stream');
      }
      return parts.join(' · ');
    }
    if (parsed.protocol === Protocol.ProtocolWebSocket) {
      return 'WebSocket URL';
    }
    if (parsed.kind === 'method-url') {
      return `${parsed.method} request`;
    }
    return null;
  });

  /** Method shown on the picker, including one typed before the URL. */
  protected readonly effectiveMethod = computed(() => {
    const parsed = this.parsed();
    return parsed.kind === 'method-url' ? parsed.method : this.method();
  });

  /** File or directory name, derived from the name until edited. */
  protected readonly fileNameValue = computed(() =>
    // An environment file always follows its display name.
    this.fileNameEdited() && this.kind() !== 'environment' ? this.fileName() : slugify(this.name()),
  );

  protected readonly slug = computed(() => slugify(this.fileNameValue()));

  /** Workspace relative path the new entry will be written to. */
  protected readonly targetPath = computed(() => {
    const slug = this.slug() || '…';
    switch (this.kind()) {
      case 'collection':
        return `${this.collectionRoot()}/${slug}/`;
      case 'environment': {
        const scope = this.environmentScope();
        return `${scope ? `${scope}/` : ''}environments/${slug}.yaml`;
      }
      case 'folder':
        return `${this.location() || COLLECTIONS_DIRECTORY}/${slug}/`;
      default:
        return `${this.location() || COLLECTIONS_DIRECTORY}/${slug}${REQUEST_FILE_EXTENSION}`;
    }
  });

  constructor() {
    const open = (event: Event) => {
      const detail = (event as CustomEvent<CreationRequest>).detail;
      void this.open(detail ?? { kind: 'request' });
    };
    window.addEventListener(CREATE_ENTRY_EVENT, open);
    inject(DestroyRef).onDestroy(() => window.removeEventListener(CREATE_ENTRY_EVENT, open));
  }

  async open(request: CreationRequest): Promise<void> {
    this.kind.set(request.kind);
    this.requestType.set('http');
    this.method.set('GET');
    this.requestText.set('');
    this.name.set('');
    this.fileName.set('');
    this.showOptions.set(false);
    this.isPrivate.set(false);
    this.color.set('');
    this.environmentScope.set(request.scope ?? '');
    this.errorMessage.set(null);
    this.nameEdited = false;
    this.fileNameEdited.set(false);

    this.dialog()?.nativeElement.showModal();
    this.focusFirstField();

    if (request.kind === 'environment') {
      await this.loadFolders();
      this.collectionScopes.set(this.folders().filter((folder) => folder.depth === 0));
      // Options arrive after the select rendered: apply the scope again.
      this.environmentScope.set('');
      queueMicrotask(() => this.environmentScope.set(request.scope ?? ''));
    } else if (request.kind === 'request' || request.kind === 'folder') {
      await this.loadFolders();
      const preferred = request.folder ?? '';
      const folders = this.folders();
      this.location.set(
        folders.some((folder) => folder.path === preferred) ? preferred : (folders[0]?.path ?? ''),
      );
    }
  }

  protected close(): void {
    this.dialog()?.nativeElement.close();
  }

  /** Closes the dialog when the backdrop, outside the panel, is clicked. */
  protected closeOnBackdrop(event: MouseEvent): void {
    if (event.target === this.dialog()?.nativeElement) {
      this.close();
    }
  }

  protected selectType(type: RequestType): void {
    this.requestType.set(type);
    this.focusRequestField();
  }

  /** Picking a method also drops one typed in front of the URL. */
  protected selectMethod(method: string): void {
    const parsed = this.parsed();
    if (parsed.kind === 'method-url') {
      this.requestText.set(parsed.url);
    }
    this.method.set(method);
  }

  protected updateRequestText(text: string): void {
    this.requestText.set(text);
    this.applyDetection(false);
  }

  /** Moves a method typed in front of the URL into the method picker. */
  protected splitMethodOnBlur(): void {
    const parsed = this.parsed();
    if (parsed.kind === 'method-url') {
      this.method.set(parsed.method);
      this.requestText.set(parsed.url);
    }
  }

  /** Reads pasted text before the single-line field strips its line breaks. */
  protected handlePaste(event: ClipboardEvent): void {
    const text = event.clipboardData?.getData('text/plain') ?? '';
    if (!isCurlCommand(text) && !/^\s*[a-z]+\s+\S+\s*$/i.test(text)) {
      return;
    }
    event.preventDefault();
    this.requestText.set(text.trim());
    this.applyDetection(true);
  }

  protected updateName(name: string): void {
    this.nameEdited = name.trim() !== '';
    this.name.set(name);
  }

  protected updateFileName(fileName: string): void {
    this.fileNameEdited.set(fileName.trim() !== '');
    this.fileName.set(fileName);
  }

  protected async submit(event?: Event): Promise<void> {
    event?.preventDefault();
    if (this.isSaving()) {
      return;
    }

    const rootPath = this.workspaceStore.rootPath();
    const name = this.name().trim();
    const slug = this.slug();
    if (!rootPath) {
      return;
    }
    if (name === '') {
      this.errorMessage.set('Give it a name.');
      this.nameField()?.nativeElement.focus();
      return;
    }
    if (slug === '') {
      this.errorMessage.set('The name needs at least one letter or digit.');
      return;
    }
    if ((this.kind() === 'request' || this.kind() === 'folder') && this.location() === '') {
      this.errorMessage.set('Create a collection first.');
      return;
    }

    this.isSaving.set(true);
    this.errorMessage.set(null);

    try {
      const created = await this.create(rootPath, name, slug);
      this.close();
      if (this.kind() === 'environment') {
        // A new environment is meant to be used: make it the active one.
        this.environmentStore.select(created);
        window.dispatchEvent(
          new CustomEvent('awake:environments-changed', {
            detail: { created, scope: this.environmentScope() },
          }),
        );
        await this.router.navigate(['/workspace/environments'], {
          queryParams: { environment: created, scope: this.environmentScope() || null },
        });
        return;
      }
      notifyCollectionsChanged({ created, kind: this.kind() });
      if (this.kind() === 'request') {
        await this.router.navigate(['/workspace/request', ...created.split('/')]);
        this.tabsStore.pin(created);
      }
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isSaving.set(false);
    }
  }

  protected handleKeydown(event: KeyboardEvent): void {
    if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      void this.submit(event);
    }
  }

  private async create(rootPath: string, name: string, slug: string): Promise<string> {
    switch (this.kind()) {
      case 'environment': {
        const scope = this.environmentScope();
        const existing = (await EnvironmentsService.List(rootPath, scope)) ?? [];
        if (existing.some((environment) => environment.FileName === slug)) {
          throw new Error('An environment with this name already exists here.');
        }
        await EnvironmentsService.Save(rootPath, scope, slug, {
          Name: name,
          Variables: {},
          SecretVariables: [],
          Color: this.color(),
        });
        return slug;
      }
      case 'collection': {
        const path = `${this.collectionRoot()}/${slug}`;
        await CollectionsService.CreateCollection(rootPath, path, {
          Name: name,
          Variables: null,
          Authentication: null,
          Documentation: '',
          Color: this.color(),
        });
        return path;
      }
      case 'folder': {
        const path = `${this.location()}/${slug}`;
        await CollectionsService.CreateFolder(rootPath, path, name);
        return path;
      }
      default: {
        const path = `${this.location()}/${slug}${REQUEST_FILE_EXTENSION}`;
        await CollectionsService.CreateRequest(rootPath, path, this.buildRequest(name));
        return path;
      }
    }
  }

  private buildRequest(name: string): Request {
    const parsed = this.parsed();
    const type = this.requestType();
    const protocol =
      type === 'sse'
        ? Protocol.ProtocolSSE
        : type === 'websocket'
          ? Protocol.ProtocolWebSocket
          : type === 'curl'
            ? parsed.protocol
            : Protocol.ProtocolHTTP;
    const url = parsed.url || 'https://api.example.com/resource';

    return {
      Name: name,
      Method: type === 'curl' ? parsed.method : this.effectiveMethod(),
      URL: url,
      Protocol: protocol,
      Parameters: null,
      PathParameters: null,
      Headers: parsed.headers.length > 0 ? parsed.headers : null,
      Authentication: parsed.authentication,
      Body: parsed.body,
      Scripts: null,
      Documentation: '',
    };
  }

  /**
   * Applies what the smart field recognised. A pasted cURL command switches
   * to the cURL editor; a WebSocket URL selects the WebSocket type; a
   * `METHOD url` pair is split into the method picker and the URL.
   */
  private applyDetection(fromPaste: boolean): void {
    const parsed = this.parsed();

    if (parsed.kind === 'curl') {
      if (this.requestType() !== 'curl') {
        this.requestType.set('curl');
        this.focusRequestField();
      }
    } else if (fromPaste && this.requestType() === 'curl' && parsed.kind !== 'empty') {
      this.requestType.set(parsed.protocol === Protocol.ProtocolWebSocket ? 'websocket' : 'http');
    } else if (parsed.protocol === Protocol.ProtocolWebSocket) {
      this.requestType.set('websocket');
    }

    if (parsed.kind === 'method-url' && fromPaste) {
      this.method.set(parsed.method);
      this.requestText.set(parsed.url);
    }

    if (!this.nameEdited) {
      this.name.set(parsed.suggestedName);
    }
  }

  private collectionRoot(): string {
    return this.isPrivate() ? PRIVATE_DIRECTORY : COLLECTIONS_DIRECTORY;
  }

  private async loadFolders(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      this.folders.set([]);
      return;
    }

    const folders: FolderOption[] = [];
    const visit = async (parent: string, prefix: string, depth: number) => {
      if (folders.length >= MAX_FOLDERS) {
        return;
      }
      let entries: Entry[];
      try {
        entries = (await CollectionsService.List(rootPath, parent)) ?? [];
      } catch {
        return;
      }
      for (const entry of entries) {
        if (entry.Kind === Kind.KindRequest || folders.length >= MAX_FOLDERS) {
          continue;
        }
        const label = prefix ? `${prefix} / ${entry.Name}` : entry.Name;
        folders.push({ path: entry.Path, label, depth });
        if (entry.HasChildren) {
          await visit(entry.Path, label, depth + 1);
        }
      }
    };

    await visit('', '', 0);
    await visit(PRIVATE_DIRECTORY, 'Private', 0);
    this.folders.set(folders);
  }

  private focusFirstField(): void {
    queueMicrotask(() => {
      if (this.kind() === 'request') {
        this.focusRequestField();
      } else {
        this.nameField()?.nativeElement.focus();
      }
    });
  }

  /** Focuses the request field with the caret after the existing text. */
  private focusRequestField(): void {
    setTimeout(() => {
      const field = this.requestField()?.nativeElement;
      if (!field) {
        return;
      }
      field.focus();
      field.setSelectionRange(field.value.length, field.value.length);
    });
  }
}

function describeError(error: unknown): string {
  const message = error instanceof Error ? error.message : String(error);
  if (message.includes('already exists')) {
    return 'Something with this name already exists here.';
  }
  if (message.includes('entry not found')) {
    return 'The chosen location no longer exists.';
  }
  return message;
}
