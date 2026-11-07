import {
  Component,
  DestroyRef,
  ElementRef,
  computed,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';
import * as CollectionsService from '@bindings/github.com/clementherve/awake/src/features/collections/service';
import * as HistoryService from '@bindings/github.com/clementherve/awake/src/features/history/service';
import * as HTTPClientService from '@bindings/github.com/clementherve/awake/src/features/httpclient/service';
import {
  Chunk,
  Response as HTTPResponse,
} from '@bindings/github.com/clementherve/awake/src/features/httpclient/models';
import * as OAuth2Service from '@bindings/github.com/clementherve/awake/src/features/oauth2/service';
import { Status as OAuth2Status } from '@bindings/github.com/clementherve/awake/src/features/oauth2/models';
import {
  Authentication,
  AuthenticationType,
  Body,
  BodyType,
  Header,
  OAuth2GrantType,
  Parameter,
  PathParameter,
  RequestOverride,
  Protocol,
  Request,
  ScriptOutcome,
} from '@bindings/github.com/clementherve/awake/src/domain/models';
import { EnvironmentStore } from '@core/environment-store';
import {
  COLLECTIONS_CHANGED_EVENT,
  CollectionsChange,
  notifyCollectionsChanged,
} from '@core/collection-events';
import { LayoutStore, RESPONSE_RATIO } from '@core/layout-store';
import { StashedEditorState, TabsStore } from '@core/tabs-store';
import { emptyBody, isFormBody, parseFormContent } from '@shared/form-body';
import {
  pathParameterNames,
  replaceUrlPlaceholders,
  urlPlaceholders,
} from '@shared/path-parameters';
import { VariableStore } from '@core/variable-store';
import { WorkspaceStore } from '@core/workspace-store';
import { AuthEditor } from '@shared/auth-editor';
import { CodeEditor, CodeLanguage, prettyJson } from '@shared/code-editor';
import { looksLikeMarkup, prettyMarkup } from '@shared/markup-format';
import { ResizeHandle } from '@shared/resize-handle';
import { Icon } from '@shared/icon';
import { VariableInput } from '@shared/variable-input';
import { RequestHistory } from './request-history/request-history';
import { StreamPanel } from './stream-panel/stream-panel';

/** Sections of the request editor. */
type EditorTab =
  'parameters' | 'headers' | 'authentication' | 'body' | 'scripts' | 'documentation' | 'history';

const HTTP_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'] as const;

/** Largest image the response panel decodes, so a huge payload stays in check. */
const MAX_IMAGE_BYTES = 16 << 20;

/** Above this size, the body is only loaded once the user asks for it. */
const LARGE_PAYLOAD_BYTES = 1 << 20;

/** Width the response pane keeps when it sits beside the request. */
const RESPONSE_MIN_WIDTH = 560;

/** Width the request pane keeps when the response sits beside it. */
const REQUEST_MIN_WIDTH = 360;

/** Reason phrases for the statuses shown from the history. */
const STATUS_TEXTS: Record<number, string> = {
  200: 'OK',
  201: 'Created',
  202: 'Accepted',
  204: 'No Content',
  301: 'Moved Permanently',
  302: 'Found',
  304: 'Not Modified',
  400: 'Bad Request',
  401: 'Unauthorized',
  403: 'Forbidden',
  404: 'Not Found',
  409: 'Conflict',
  422: 'Unprocessable Entity',
  429: 'Too Many Requests',
  500: 'Internal Server Error',
  502: 'Bad Gateway',
  503: 'Service Unavailable',
  504: 'Gateway Timeout',
};

const TAB_LABELS: Record<EditorTab, string> = {
  parameters: 'Parameters',
  headers: 'Headers',
  authentication: 'Authentication',
  body: 'Body',
  scripts: 'Scripts',
  documentation: 'Documentation',
  history: 'History',
};

/**
 * Edits a single request file. Changes stay local until the user saves, which
 * writes the canonical YAML representation back to disk.
 */
/** The version of an overridable tab: tracked by git, or this machine only. */
type OverrideSource = 'shared' | 'local';
type OverrideSection = 'parameters' | 'body';

function emptyOverride(): RequestOverride {
  return { Parameters: null, PathParameters: null, Body: null };
}

function valueOf(parameters: PathParameter[] | null, name: string): string | undefined {
  return parameters?.find((parameter) => parameter.Name === name)?.Value;
}

@Component({
  selector: 'app-request-editor',
  imports: [
    FormsModule,
    RouterLink,
    StreamPanel,
    RequestHistory,
    Icon,
    AuthEditor,
    CodeEditor,
    ResizeHandle,
    VariableInput,
  ],
  templateUrl: './request-editor.html',
  styleUrls: ['./request-editor.css', './request-layout.css', './request-response.css'],
  host: {
    '(document:keydown)': 'handleShortcut($event)',
    '(document:click)': 'closeKindMenu()',
  },
})
export class RequestEditor {
  private readonly route = inject(ActivatedRoute);
  private readonly environmentStore = inject(EnvironmentStore);
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly variableStore = inject(VariableStore);
  private readonly tabsStore = inject(TabsStore);
  private readonly editor = viewChild.required<ElementRef<HTMLElement>>('editor');
  protected readonly layout = inject(LayoutStore);
  protected readonly defaultResponseRatio = RESPONSE_RATIO.default;
  private readonly streamPanel = viewChild<StreamPanel>(StreamPanel);

  protected readonly AuthenticationType = AuthenticationType;
  protected readonly OAuth2GrantType = OAuth2GrantType;
  protected readonly BodyType = BodyType;
  protected readonly Protocol = Protocol;
  protected readonly httpMethods = HTTP_METHODS;
  protected readonly tabs = Object.keys(TAB_LABELS) as EditorTab[];
  protected readonly tabLabels = TAB_LABELS;
  /** Labels used when the request pane is narrow, side by side. */
  protected readonly shortTabLabels: Record<EditorTab, string> = {
    parameters: 'Params',
    headers: 'Headers',
    authentication: 'Auth',
    body: 'Body',
    scripts: 'Scripts',
    documentation: 'Docs',
    history: 'History',
  };
  protected readonly bodyTypes = [
    { type: BodyType.BodyNone, label: 'None' },
    { type: BodyType.BodyJSON, label: 'JSON' },
    { type: BodyType.BodyText, label: 'Text' },
    { type: BodyType.BodyXML, label: 'XML' },
    { type: BodyType.BodyFormURLEncoded, label: 'Form' },
    { type: BodyType.BodyMultipart, label: 'Multipart' },
    { type: BodyType.BodyBinary, label: 'Binary' },
  ];
  protected readonly selectedEnvironment = this.environmentStore.selected;

  protected readonly requestPath = signal('');
  /** The collection of the request, whose authentication it may inherit. */
  protected readonly collectionPath = computed(() => {
    const segments = this.requestPath().split('/');
    return segments.length >= 3 ? `${segments[0]}/${segments[1]}` : '';
  });
  protected readonly collectionName = signal('');
  protected readonly inheritedAuthentication = signal<Authentication | null>(null);
  /** Request path split for the breadcrumb, without the collections root. */
  protected readonly pathSegments = computed(() => {
    const segments = this.requestPath().split('/').filter(Boolean);
    if (segments[0] === 'collections') {
      segments.shift();
    }
    return segments;
  });
  protected readonly isLoading = signal(true);
  protected readonly isSaving = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly statusMessage = signal<string | null>(null);

  protected readonly name = signal('');
  protected readonly method = signal<string>('GET');
  protected readonly protocol = signal<Protocol>(Protocol.ProtocolHTTP);
  protected readonly url = signal('');
  protected readonly documentation = signal('');
  protected readonly parameters = signal<Parameter[]>([]);
  protected readonly headers = signal<Header[]>([]);
  protected readonly authentication = signal<Authentication>(defaultAuthentication());
  protected readonly body = signal<Body>(emptyBody());
  /**
   * Local overrides, kept on this machine in <request>.local.yaml: null keeps
   * the shared section, a value replaces it when the request is sent.
   */
  protected readonly localParameters = signal<Parameter[] | null>(null);
  /** Values of the :name segments of the URL, shared and local. */
  protected readonly pathParameters = signal<PathParameter[]>([]);
  protected readonly localPathParameters = signal<PathParameter[] | null>(null);
  protected readonly pathNames = computed(() => pathParameterNames(this.url()));
  /**
   * One row per :name segment of the URL, with the value of the version on
   * screen; a local override falls back to the shared value of a segment it
   * does not set.
   */
  protected readonly shownPathParameters = computed<(PathParameter & { InQuery: boolean })[]>(
    () => {
      const local = this.parametersSource() === 'local' && this.localParameters() !== null;
      const placeholders = urlPlaceholders(this.url());
      return this.pathNames().map((name) => ({
        Name: name,
        Value:
          (local ? valueOf(this.localPathParameters(), name) : undefined) ??
          valueOf(this.pathParameters(), name) ??
          '',
        // Tagged when the name only stands in the query.
        InQuery: placeholders.every(
          (placeholder) => placeholder.name !== name || placeholder.inQuery,
        ),
      }));
    },
  );
  protected readonly localBody = signal<Body | null>(null);
  /** Which version each overridable tab shows and edits. */
  protected readonly parametersSource = signal<OverrideSource>('shared');
  protected readonly bodySource = signal<OverrideSource>('shared');
  /** The parameters and body shown, local when a local override is open. */
  protected readonly shownParameters = computed(() =>
    this.parametersSource() === 'local' ? (this.localParameters() ?? []) : this.parameters(),
  );
  protected readonly shownBody = computed(() =>
    this.bodySource() === 'local' ? (this.localBody() ?? emptyBody()) : this.body(),
  );
  /** The parameters the request sends, for the tab count. */
  /**
   * The URL as it will be sent: variables resolved (secrets masked), path
   * parameters filled in and enabled query parameters appended. Shown only
   * when it differs from what is typed.
   */
  protected readonly resolvedUrl = computed(() => {
    const raw = this.url().trim();
    const local = this.localParameters() !== null;
    const encode = (text: string) =>
      encodeURIComponent(this.variableStore.preview(text)).replace(/%E2%80%A2/g, '•');

    let resolved = replaceUrlPlaceholders(raw, (placeholder) => {
      const value =
        (local ? valueOf(this.localPathParameters(), placeholder.name) : undefined) ??
        valueOf(this.pathParameters(), placeholder.name) ??
        '';
      return value.trim() === '' ? ':' + placeholder.name : encode(value);
    });
    resolved = this.variableStore.preview(resolved);

    const query = (this.localParameters() ?? this.parameters())
      .filter((parameter) => parameter.Enabled && parameter.Name.trim() !== '')
      .map((parameter) => `${encode(parameter.Name)}=${encode(parameter.Value)}`);
    if (query.length > 0) {
      resolved += (resolved.includes('?') ? '&' : '?') + query.join('&');
    }
    return resolved === raw ? '' : resolved;
  });

  protected readonly sentParameters = computed(
    () => (this.localParameters() ?? this.parameters()).length + this.pathNames().length,
  );
  protected readonly overrideFileName = computed(
    () => (this.requestPath().split('/').pop() ?? '').replace(/\.yaml$/, '') + '.local.yaml',
  );
  protected readonly preRequestScript = signal('');
  protected readonly postResponseScript = signal('');
  protected readonly activeTab = signal<EditorTab>('parameters');
  protected readonly oauth2Status = signal<OAuth2Status | null>(null);
  protected readonly oauth2Busy = signal(false);
  protected readonly oauth2Error = signal<string | null>(null);

  protected readonly response = signal<HTTPResponse | null>(null);
  protected readonly responseBody = signal('');
  protected readonly responseHasMore = signal(false);
  protected readonly responseTab = signal<'body' | 'headers' | 'scripts'>('body');
  protected readonly responseError = signal<string | null>(null);
  protected readonly isSending = signal(false);
  protected readonly isLoadingChunk = signal(false);

  /** Images get a picture instead of the "binary response" note. */
  protected readonly isImageResponse = computed(() =>
    isImageContentType(this.response()?.ContentType ?? ''),
  );
  protected readonly isHtmlResponse = computed(() =>
    isHtmlContentType(this.response()?.ContentType ?? ''),
  );
  /** HTML responses can also be rendered, not just shown as source. */
  protected readonly htmlPreview = signal(false);
  protected readonly imageUrl = signal<string | null>(null);
  protected readonly isLoadingImage = signal(false);
  protected readonly imageError = signal<string | null>(null);
  /** A large body stays hidden until the user agrees to load it. */
  protected readonly largeResponseShown = signal(false);
  protected readonly awaitingLargeResponse = computed(() => {
    const response = this.response();
    return (
      !!response &&
      response.Encoding === 'utf-8' &&
      response.Size > LARGE_PAYLOAD_BYTES &&
      this.responseHasMore() &&
      !this.largeResponseShown() &&
      !isImageContentType(response.ContentType)
    );
  });
  private imageObjectUrl: string | null = null;
  private imageLoadToken = 0;

  private readonly snapshot = signal('');
  private readonly overrideSnapshot = signal('');
  private responseNextOffset = 0;

  private readonly draft = computed<Request>(() => ({
    Name: this.name(),
    Method: this.method(),
    URL: this.url(),
    Protocol: this.protocol(),
    Parameters: this.parameters().filter((parameter) => parameter.Name.trim() !== ''),
    // Only the segments still in the URL are kept.
    PathParameters: this.pathNames().length
      ? this.pathNames().map((name) => ({
          Name: name,
          Value: valueOf(this.pathParameters(), name) ?? '',
        }))
      : null,
    Headers: this.headers().filter((header) => header.Name.trim() !== ''),
    Authentication: this.authentication(),
    Body: this.body(),
    Scripts:
      this.preRequestScript().trim() === '' && this.postResponseScript().trim() === ''
        ? null
        : {
            PreRequest: this.preRequestScript(),
            PostResponse: this.postResponseScript(),
          },
    Documentation: this.documentation(),
  }));

  private readonly overrideDraft = computed<RequestOverride>(() => ({
    PathParameters:
      this.localParameters() !== null && this.pathNames().length
        ? this.pathNames().map((name) => ({
            Name: name,
            Value:
              valueOf(this.localPathParameters(), name) ??
              valueOf(this.pathParameters(), name) ??
              '',
          }))
        : null,
    Parameters: this.localParameters()?.filter((parameter) => parameter.Name.trim() !== '') ?? null,
    Body: this.localBody(),
  }));

  private readonly isOverrideDirty = computed(
    () => JSON.stringify(this.overrideDraft()) !== this.overrideSnapshot(),
  );

  protected readonly isDirty = computed(
    () => JSON.stringify(this.draft()) !== this.snapshot() || this.isOverrideDirty(),
  );

  protected readonly isStreaming = computed(() => this.protocol() !== Protocol.ProtocolHTTP);

  protected readonly hasResponseScripts = computed(() => {
    const response = this.response();
    return response ? this.hasScriptOutcome(response) : false;
  });

  protected readonly streamLive = computed(() => this.streamPanel()?.isLive() ?? false);

  protected readonly bodyLanguage = computed<CodeLanguage>(() => {
    switch (this.shownBody().Type) {
      case BodyType.BodyJSON:
        return 'json';
      case BodyType.BodyXML:
        return 'xml';
      default:
        return 'text';
    }
  });

  /** Language of the response, from its content type. */
  protected readonly responseLanguage = computed<CodeLanguage>(() => {
    const contentType = (this.response()?.ContentType ?? '').toLowerCase();
    if (contentType.includes('json')) {
      return 'json';
    }
    if (contentType.includes('xml') || contentType.includes('html')) {
      return 'xml';
    }

    // A missing or generic content type still gets formatted when the body
    // clearly is JSON or markup.
    const body = this.responseBody();
    if (prettyJson(body) !== body) {
      return 'json';
    }
    return looksLikeMarkup(body) ? 'xml' : 'text';
  });

  /** The response body, indented when it is JSON, XML or HTML. */
  protected readonly formattedResponseBody = computed(() => {
    const body = this.responseBody();
    switch (this.responseLanguage()) {
      case 'json':
        return prettyJson(body);
      case 'xml':
        return prettyMarkup(body);
      default:
        return body;
    }
  });

  /** Set when the response shown is the last run read from the history. */
  protected readonly lastRunAt = signal<string | null>(null);

  protected readonly isKindMenuOpen = signal(false);

  /** Label of the method and protocol picker: the method, SSE or WS. */
  protected readonly kindLabel = computed(() => {
    switch (this.protocol()) {
      case Protocol.ProtocolSSE:
        return 'SSE';
      case Protocol.ProtocolWebSocket:
        return 'WS';
      default:
        return this.method();
    }
  });

  /**
   * Variables of the URL that the active environment does not define, shown
   * as a compact warning in the URL bar. The resolved URL is in its tooltip.
   */
  protected readonly missingVariables = computed(() => {
    const references = this.variableStore.references(this.url());
    const missing = [
      ...new Set(
        references.filter((reference) => !reference.variable).map((reference) => reference.name),
      ),
    ];
    if (missing.length === 0) {
      return null;
    }
    const noEnvironment = !this.selectedEnvironment();
    return {
      label: noEnvironment
        ? 'No environment'
        : missing.length === 1
          ? `${missing[0]} undefined`
          : `${missing.length} undefined`,
      title: noEnvironment
        ? `Select an environment to resolve ${missing.join(', ')}.`
        : `Not defined in the active environment: ${missing.join(', ')}. Click to edit environments.`,
    };
  });

  protected readonly sendLabel = computed(() => {
    if (this.isStreaming()) {
      return this.streamLive() ? 'Disconnect' : 'Connect';
    }
    return this.isSending() ? 'Sending…' : 'Send';
  });

  constructor() {
    this.route.url.pipe(takeUntilDestroyed()).subscribe((segments) => {
      const path = segments.map((segment) => segment.path).join('/');
      if (path) {
        void this.load(path);
      }
    });

    // Follow the request when it moves in the tree, keeping unsaved edits.
    const collectionsChanged = (event: Event) => {
      void this.loadCollection(this.collectionPath());
      const moved = (event as CustomEvent<CollectionsChange | null>).detail?.moved;
      const path = this.requestPath();
      if (moved && path && (path === moved.from || path.startsWith(`${moved.from}/`))) {
        this.requestPath.set(moved.to + path.slice(moved.from.length));
      }
    };
    window.addEventListener(COLLECTIONS_CHANGED_EVENT, collectionsChanged);
    inject(DestroyRef).onDestroy(() =>
      window.removeEventListener(COLLECTIONS_CHANGED_EVENT, collectionsChanged),
    );

    // Load what an inheriting request receives from its collection.
    effect(() => {
      void this.loadCollection(this.collectionPath());
    });

    // Confirmations such as "Saved" fade after a moment.
    effect((onCleanup) => {
      if (this.statusMessage()) {
        const timer = setTimeout(() => this.statusMessage.set(null), 2500);
        onCleanup(() => clearTimeout(timer));
      }
    });

    // Keep the tab title, method and unsaved marker in sync with the editor.
    effect(() => {
      const path = this.requestPath();
      if (!path || this.isLoading()) {
        return;
      }
      this.tabsStore.describe(path, {
        name: this.name() || 'Untitled',
        method: this.method(),
        protocol: this.protocol(),
      });
      this.tabsStore.markDirty(path, this.isDirty());
    });
  }

  ngOnDestroy(): void {
    void this.streamPanel()?.release();
    this.stashCurrent();
    this.releaseImage();
  }

  /** Turns the handle position into the share of the editor the response takes. */
  protected resizeResponse(position: { x: number; y: number }): void {
    const rect = this.editor().nativeElement.getBoundingClientRect();
    const side = this.layout.editorLayout() === 'side';
    let ratio = side
      ? (rect.right - position.x) / rect.width
      : (rect.bottom - position.y) / rect.height;

    // Beside the request, the pane stays wide enough for its toolbar without
    // squeezing the request pane below its own minimum.
    if (side && rect.width > 0) {
      const available = Math.max(0, rect.width - REQUEST_MIN_WIDTH);
      const minimum = Math.min(RESPONSE_MIN_WIDTH, available) / rect.width;
      const maximum = available / rect.width;
      ratio = Math.min(Math.max(ratio, minimum), maximum);
    }

    this.layout.setResponseRatio(ratio);
  }

  protected selectTab(tab: EditorTab): void {
    this.activeTab.set(tab);
  }

  protected async save(): Promise<boolean> {
    const draft = this.draft();
    if (draft.Name.trim() === '' || draft.URL.trim() === '') {
      this.errorMessage.set('A request needs a name and a URL.');
      return false;
    }

    const previousName = this.savedName();

    const request: Request = {
      ...draft,
      Name: draft.Name.trim(),
      URL: draft.URL.trim(),
    };

    this.isSaving.set(true);
    this.errorMessage.set(null);
    this.statusMessage.set(null);

    try {
      await CollectionsService.UpdateRequest(
        this.workspaceStore.rootPath() ?? '',
        this.requestPath(),
        request,
      );

      if (this.isOverrideDirty()) {
        await CollectionsService.SaveRequestOverride(
          this.workspaceStore.rootPath() ?? '',
          this.requestPath(),
          this.overrideDraft(),
        );
        this.overrideSnapshot.set(JSON.stringify(this.overrideDraft()));
      }

      this.name.set(request.Name);
      this.url.set(request.URL);
      this.snapshot.set(JSON.stringify(this.draft()));
      this.statusMessage.set('Saved');
      void this.refreshOAuth2Status();
      // The tree labels requests with their name from the file, so a rename
      // only shows there once it reloads.
      if (request.Name !== previousName) {
        notifyCollectionsChanged();
      }
      return true;
    } catch (error) {
      this.errorMessage.set(describeError(error));
      return false;
    } finally {
      this.isSaving.set(false);
    }
  }

  protected async send(): Promise<void> {
    if (this.isLoading() || this.isSending()) {
      return;
    }

    if (this.isDirty() && !(await this.save())) {
      return;
    }

    if (this.isStreaming()) {
      await this.streamPanel()?.toggle();
      return;
    }

    this.isSending.set(true);
    this.responseError.set(null);
    this.lastRunAt.set(null);
    await this.closeResponse();

    try {
      const response = await HTTPClientService.Send(
        this.workspaceStore.rootPath() ?? '',
        this.requestPath(),
        this.environmentStore.selected(),
      );

      this.response.set(response);
      this.responseTab.set('body');
      this.responseHasMore.set(false);
      this.htmlPreview.set(false);
      this.largeResponseShown.set(false);

      if (response.Encoding === 'utf-8') {
        this.responseBody.set(response.Preview);
        this.responseNextOffset = new TextEncoder().encode(response.Preview).length;
        this.responseHasMore.set(this.responseNextOffset < response.Size);
      } else {
        this.responseBody.set('');
        this.responseNextOffset = 0;
      }

      void this.loadImage(response);
    } catch (error) {
      this.responseError.set(describeError(error));
    } finally {
      this.isSending.set(false);
    }
  }

  /**
   * Builds a blob URL for an image response. The preview covers small images;
   * larger ones are read from the stored body one chunk at a time.
   */
  private async loadImage(response: HTTPResponse | null): Promise<void> {
    this.releaseImage();
    if (!response || !isImageContentType(response.ContentType)) {
      return;
    }

    if (response.Size === 0) {
      this.imageError.set('The response has no image data.');
      return;
    }
    if (response.Size > MAX_IMAGE_BYTES) {
      this.imageError.set(`Image too large to preview (${this.formatSize(response.Size)}).`);
      return;
    }

    const token = ++this.imageLoadToken;
    this.isLoadingImage.set(true);

    try {
      const bytes = await this.imageBytes(response, token);
      if (token !== this.imageLoadToken) {
        return;
      }
      const type = mediaTypeOf(response.ContentType) || 'application/octet-stream';
      const url = URL.createObjectURL(new Blob([bytes.buffer as ArrayBuffer], { type }));
      this.imageObjectUrl = url;
      this.imageUrl.set(url);
    } catch (error) {
      if (token === this.imageLoadToken) {
        this.imageError.set(describeError(error));
      }
    } finally {
      if (token === this.imageLoadToken) {
        this.isLoadingImage.set(false);
      }
    }
  }

  private async imageBytes(response: HTTPResponse, token: number): Promise<Uint8Array> {
    if (response.Encoding === 'utf-8') {
      // SVG arrives as text; read the rest of the body when it was truncated.
      let text = this.responseBody();
      let offset = new TextEncoder().encode(text).length;
      while (response.ID && offset < response.Size) {
        const chunk = await HTTPClientService.ReadChunk(response.ID, offset, 1 << 20);
        if (token !== this.imageLoadToken) {
          throw new Error('Image preview replaced.');
        }
        text += decodeChunk(chunk);
        if (!chunk.HasMore) {
          break;
        }
        offset = chunk.NextOffset;
      }
      return new TextEncoder().encode(text);
    }

    const preview = base64Bytes(response.Preview);
    if (preview.length >= response.Size) {
      return preview;
    }
    if (!response.ID) {
      throw new Error('This image is truncated in the history.');
    }

    const parts = [preview];
    let offset = preview.length;
    while (offset < response.Size) {
      const chunk = await HTTPClientService.ReadChunk(response.ID, offset, 1 << 20);
      if (token !== this.imageLoadToken) {
        throw new Error('Image preview replaced.');
      }
      parts.push(base64Bytes(chunk.Content));
      if (!chunk.HasMore) {
        break;
      }
      offset = chunk.NextOffset;
    }
    return concatBytes(parts);
  }

  private releaseImage(): void {
    this.imageLoadToken += 1;
    if (this.imageObjectUrl) {
      URL.revokeObjectURL(this.imageObjectUrl);
      this.imageObjectUrl = null;
    }
    this.imageUrl.set(null);
    this.imageError.set(null);
    this.isLoadingImage.set(false);
  }

  protected async loadMore(): Promise<void> {
    const response = this.response();
    if (!response || this.isLoadingChunk() || !this.responseHasMore()) {
      return;
    }

    this.isLoadingChunk.set(true);

    try {
      const chunk = await HTTPClientService.ReadChunk(
        response.ID,
        this.responseNextOffset,
        256 << 10,
      );
      this.responseBody.update((body) => body + decodeChunk(chunk));
      this.responseNextOffset = chunk.NextOffset;
      this.responseHasMore.set(chunk.HasMore);
    } catch (error) {
      this.responseError.set(describeError(error));
    } finally {
      this.isLoadingChunk.set(false);
    }
  }

  /** Loads the whole body after the user agreed to see a large response. */
  protected async displayLargeResponse(): Promise<void> {
    this.largeResponseShown.set(true);
    const response = this.response();
    if (!response || this.isLoadingChunk() || !this.responseHasMore()) {
      return;
    }

    this.isLoadingChunk.set(true);
    this.responseError.set(null);
    let body = this.responseBody();
    let offset = this.responseNextOffset;
    let hasMore = this.responseHasMore();

    try {
      while (hasMore) {
        const chunk = await HTTPClientService.ReadChunk(response.ID, offset, 1 << 20);
        if (this.response()?.ID !== response.ID) {
          return;
        }
        body += decodeChunk(chunk);
        offset = chunk.NextOffset;
        hasMore = chunk.HasMore;
      }
    } catch (error) {
      this.responseError.set(describeError(error));
    } finally {
      if (this.response()?.ID === response.ID) {
        this.responseBody.set(body);
        this.responseNextOffset = offset;
        this.responseHasMore.set(hasMore);
      }
      this.isLoadingChunk.set(false);
    }
  }

  protected async closeResponsePanel(): Promise<void> {
    this.lastRunAt.set(null);
    await this.closeResponse();
    this.responseError.set(null);
  }

  protected selectResponseTab(tab: 'body' | 'headers' | 'scripts'): void {
    this.responseTab.set(tab);
  }

  protected hasScriptOutcome(response: HTTPResponse): boolean {
    return (
      hasScriptContent(response.PreRequestOutcome) || hasScriptContent(response.PostResponseOutcome)
    );
  }

  protected scriptOutcomes(response: HTTPResponse): { label: string; outcome: ScriptOutcome }[] {
    return [
      { label: 'Before the request', outcome: response.PreRequestOutcome },
      { label: 'After the response', outcome: response.PostResponseOutcome },
    ].filter((entry) => hasScriptContent(entry.outcome));
  }

  protected statusClass(response: HTTPResponse): string {
    if (response.StatusCode >= 500) {
      return 'server-error';
    }
    if (response.StatusCode >= 400) {
      return 'client-error';
    }
    return 'success';
  }

  protected headerEntries(response: HTTPResponse): { name: string; value: string }[] {
    return Object.entries(response.Headers ?? {}).map(([name, values]) => ({
      name,
      value: (values ?? []).join(', '),
    }));
  }

  protected formatSize(bytes: number): string {
    if (bytes < 1024) {
      return `${bytes} B`;
    }
    if (bytes < 1024 * 1024) {
      return `${(bytes / 1024).toFixed(1)} kB`;
    }
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }

  protected handleShortcut(event: KeyboardEvent): void {
    if (!(event.metaKey || event.ctrlKey)) {
      return;
    }

    if (event.key.toLowerCase() === 's') {
      event.preventDefault();
      if (this.isDirty() && !this.isSaving()) {
        void this.save();
      }
      return;
    }

    if (event.key === 'Enter') {
      event.preventDefault();
      void this.send();
    }
  }

  protected addParameter(): void {
    this.editParameters((parameters) => [...parameters, { Name: '', Value: '', Enabled: true }]);
  }

  protected updateParameter(
    index: number,
    field: 'Name' | 'Value' | 'Enabled',
    value: string | boolean,
  ): void {
    this.editParameters((parameters) =>
      parameters.map((parameter, position) =>
        position === index ? ({ ...parameter, [field]: value } as Parameter) : parameter,
      ),
    );
  }

  protected removeParameter(index: number): void {
    this.editParameters((parameters) =>
      parameters.filter((_parameter, position) => position !== index),
    );
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
    this.headers.update((headers) => headers.filter((_header, position) => position !== index));
  }

  /**
   * Shows the last execution of the request, from the history, so opening a
   * request always shows what it returned last time.
   */
  private async loadLastRun(path: string): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath) {
      return;
    }
    try {
      const [latest] = (await HistoryService.ListForRequest(rootPath, path, 1, 0)) ?? [];
      if (!latest) {
        return;
      }
      const entry = await HistoryService.Entry(rootPath, latest.ID);
      const exchange = entry.Exchange;
      // The user may have moved on, or sent the request, in the meantime.
      if (!exchange || this.requestPath() !== path || this.response() || this.isSending()) {
        return;
      }
      const emptyOutcome = {
        Logs: null,
        Tests: null,
        Error: '',
        EnvironmentChanges: null,
        EnvironmentError: '',
      };
      this.response.set({
        ID: '',
        StatusCode: entry.StatusCode,
        Status: `${entry.StatusCode} ${STATUS_TEXTS[entry.StatusCode] ?? ''}`.trim(),
        Headers: exchange.ResponseHeaders,
        DurationMilliseconds: entry.DurationMilliseconds,
        Size: entry.SizeBytes,
        ContentType: exchange.ResponseHeaders?.['Content-Type']?.[0] ?? '',
        Preview: exchange.ResponseBody,
        Encoding: exchange.ResponseEncoding,
        Truncated: exchange.ResponseTruncated,
        HistoryWarning: '',
        PreRequestOutcome: emptyOutcome,
        PostResponseOutcome: emptyOutcome,
      });
      this.responseBody.set(exchange.ResponseEncoding === 'base64' ? '' : exchange.ResponseBody);
      this.responseHasMore.set(false);
      this.htmlPreview.set(false);
      this.largeResponseShown.set(false);
      void this.loadImage(this.response());
      this.lastRunAt.set(entry.ExecutedAt);
    } catch {
      // Without history, the editor simply opens without a response.
    }
  }

  protected lastRunLabel(executedAt: string): string {
    const minutes = Math.round((Date.now() - new Date(executedAt).getTime()) / 60_000);
    if (minutes < 1) {
      return 'just now';
    }
    if (minutes < 60) {
      return `${minutes} min ago`;
    }
    if (minutes < 1440) {
      return `${Math.floor(minutes / 60)} h ago`;
    }
    return new Date(executedAt).toLocaleDateString();
  }

  private async loadCollection(collectionPath: string): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    if (!rootPath || !collectionPath) {
      this.collectionName.set('');
      this.inheritedAuthentication.set(null);
      return;
    }
    try {
      const collection = await CollectionsService.LoadCollection(rootPath, collectionPath);
      this.collectionName.set(collection.Name);
      this.inheritedAuthentication.set(collection.Authentication);
    } catch {
      this.collectionName.set('');
      this.inheritedAuthentication.set(null);
    }
  }

  protected setAuthentication(authentication: Authentication): void {
    const previous = this.authentication();
    this.authentication.set(authentication);
    if (authentication.Type !== AuthenticationType.AuthenticationOAuth2) {
      this.oauth2Status.set(null);
      this.oauth2Error.set(null);
    } else if (
      previous.Type !== authentication.Type ||
      previous.GrantType !== authentication.GrantType
    ) {
      void this.refreshOAuth2Status();
    }
  }

  protected async authorizeOAuth2(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const requestPath = this.requestPath();
    if (!rootPath || !requestPath || this.oauth2Busy()) {
      return;
    }
    if (this.isDirty() && !(await this.save())) {
      return;
    }

    this.oauth2Busy.set(true);
    this.oauth2Error.set(null);

    try {
      const status = await OAuth2Service.Authorize(
        rootPath,
        requestPath,
        this.environmentStore.selected(),
      );
      this.oauth2Status.set(status);
      this.statusMessage.set('OAuth 2 token obtained.');
    } catch (error) {
      this.oauth2Error.set(describeError(error));
    } finally {
      this.oauth2Busy.set(false);
    }
  }

  protected async clearOAuth2(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const requestPath = this.requestPath();
    if (!rootPath || !requestPath) {
      return;
    }

    this.oauth2Busy.set(true);
    this.oauth2Error.set(null);

    try {
      await OAuth2Service.Clear(rootPath, requestPath, this.environmentStore.selected());
      this.oauth2Status.set(null);
      this.statusMessage.set('OAuth 2 token cleared.');
    } catch (error) {
      this.oauth2Error.set(describeError(error));
    } finally {
      this.oauth2Busy.set(false);
    }
  }

  protected oauth2Expiry(): string {
    const status = this.oauth2Status();
    if (!status?.HasToken) {
      return 'No token stored.';
    }
    if (!status.ExpiresAtMilliseconds) {
      return 'Token stored without an expiry.';
    }
    return `Token stored until ${new Date(status.ExpiresAtMilliseconds).toLocaleString()}.`;
  }

  /** Starts a local override of a tab from a copy of its shared version. */
  protected overrideLocally(section: OverrideSection): void {
    if (section === 'parameters') {
      this.localParameters.set(this.parameters().map((parameter) => ({ ...parameter })));
      this.localPathParameters.set(this.pathParameters().map((parameter) => ({ ...parameter })));
      this.parametersSource.set('local');
    } else {
      this.localBody.set({ ...this.body() });
      this.bodySource.set('local');
    }
  }

  /** Drops a local override; the shared version is sent again once saved. */
  protected removeOverride(section: OverrideSection): void {
    if (section === 'parameters') {
      this.localParameters.set(null);
      this.localPathParameters.set(null);
      this.parametersSource.set('shared');
    } else {
      this.localBody.set(null);
      this.bodySource.set('shared');
    }
  }

  /** Sets the value of a :name segment in the version on screen. */
  protected updatePathParameter(name: string, value: string): void {
    const upsert = (parameters: PathParameter[] | null): PathParameter[] => [
      ...(parameters ?? []).filter((parameter) => parameter.Name !== name),
      { Name: name, Value: value },
    ];
    if (this.parametersSource() === 'local' && this.localParameters()) {
      this.localPathParameters.update(upsert);
    } else {
      this.pathParameters.update(upsert);
    }
  }

  private editParameters(change: (parameters: Parameter[]) => Parameter[]): void {
    if (this.parametersSource() === 'local' && this.localParameters()) {
      this.localParameters.update((parameters) => change(parameters ?? []));
    } else {
      this.parameters.update(change);
    }
  }

  private editBody(change: (body: Body) => Body): void {
    if (this.bodySource() === 'local' && this.localBody()) {
      this.localBody.update((body) => change(body ?? emptyBody()));
    } else {
      this.body.update(change);
    }
  }

  protected formatBody(): void {
    const content = this.shownBody().Content;
    const formatted = prettyJson(content);
    if (formatted !== content) {
      this.updateBodyContent(formatted);
    }
  }

  /**
   * Changes the body format. A text body becoming a form is read into
   * fields, so nothing typed is lost.
   */
  protected selectBodyType(type: BodyType): void {
    this.editBody((body) => {
      if (isFormBody(type) && !body.Fields?.length && body.Content.trim() !== '') {
        return { Type: type, Content: '', Fields: parseFormContent(body.Content) };
      }
      return { ...body, Type: type };
    });
  }

  protected addFormField(): void {
    this.editBody((body) => ({
      ...body,
      Fields: [...(body.Fields ?? []), { Name: '', Value: '', Enabled: true }],
    }));
  }

  protected updateFormField(index: number, field: keyof Parameter, value: string | boolean): void {
    this.editBody((body) => ({
      ...body,
      Fields: (body.Fields ?? []).map((entry, position) =>
        position === index ? ({ ...entry, [field]: value } as Parameter) : entry,
      ),
    }));
  }

  protected removeFormField(index: number): void {
    this.editBody((body) => ({
      ...body,
      Fields: (body.Fields ?? []).filter((_entry, position) => position !== index),
    }));
  }

  protected toggleKindMenu(event: Event): void {
    event.stopPropagation();
    this.isKindMenuOpen.update((open) => !open);
  }

  protected closeKindMenu(): void {
    this.isKindMenuOpen.set(false);
  }

  protected chooseMethod(method: string): void {
    this.method.set(method);
    this.selectProtocol(Protocol.ProtocolHTTP);
    this.isKindMenuOpen.set(false);
  }

  protected chooseProtocol(protocol: Protocol): void {
    this.selectProtocol(protocol);
    this.isKindMenuOpen.set(false);
  }

  protected selectProtocol(value: string): void {
    const protocol = value as Protocol;
    if (protocol === this.protocol()) {
      return;
    }

    void this.streamPanel()?.release();
    void this.closeResponse();
    this.protocol.set(protocol);
  }

  protected updateBodyContent(value: string): void {
    this.editBody((body) => ({ ...body, Content: value }));
  }

  /**
   * Hands the edits and the last response of the request on screen to the
   * tabs store, so coming back to its tab restores them.
   */
  private stashCurrent(): void {
    const path = this.requestPath();
    if (!path || this.isLoading()) {
      return;
    }
    const state: StashedEditorState = {
      draft: this.draft(),
      snapshot: this.snapshot(),
      override: this.overrideDraft(),
      overrideSnapshot: this.overrideSnapshot(),
      responseIdentifier: this.response()?.ID,
      response: this.response(),
      responseBody: this.responseBody(),
      responseHasMore: this.responseHasMore(),
      responseNextOffset: this.responseNextOffset,
      responseTab: this.responseTab(),
      responseError: this.responseError(),
      activeTab: this.activeTab(),
      lastRunAt: this.lastRunAt(),
    };
    this.tabsStore.stash(path, state);
    // The store owns the response handle from now on.
    this.response.set(null);
    this.releaseImage();
  }

  private restore(state: StashedEditorState): void {
    this.applyRequest(state.draft as Request);
    this.applyOverride((state['override'] as RequestOverride | undefined) ?? emptyOverride());
    this.snapshot.set(state.snapshot);
    this.overrideSnapshot.set((state['overrideSnapshot'] as string) ?? this.overrideSnapshot());
    this.response.set((state['response'] as HTTPResponse | null) ?? null);
    this.responseBody.set((state['responseBody'] as string) ?? '');
    this.responseHasMore.set(Boolean(state['responseHasMore']));
    this.responseNextOffset = (state['responseNextOffset'] as number) ?? 0;
    this.responseTab.set((state['responseTab'] as 'body' | 'headers' | 'scripts') ?? 'body');
    this.responseError.set((state['responseError'] as string | null) ?? null);
    this.activeTab.set((state['activeTab'] as EditorTab) ?? 'parameters');
    this.lastRunAt.set((state['lastRunAt'] as string | null) ?? null);
    this.htmlPreview.set(false);
    this.largeResponseShown.set(false);
    void this.loadImage(this.response());
  }

  private async load(path: string): Promise<void> {
    // Already on screen, for instance after the request moved in the tree.
    if (path === this.requestPath() && !this.isLoading()) {
      return;
    }
    await this.streamPanel()?.release();
    if (this.requestPath() && this.requestPath() !== path) {
      this.stashCurrent();
    }
    await this.closeResponse();
    this.lastRunAt.set(null);
    this.requestPath.set(path);
    this.errorMessage.set(null);
    this.statusMessage.set(null);

    const stashed = this.tabsStore.takeStash(path);
    if (stashed) {
      this.restore(stashed);
      this.isLoading.set(false);
      return;
    }

    this.isLoading.set(true);
    try {
      const rootPath = this.workspaceStore.rootPath() ?? '';
      const [request, override] = await Promise.all([
        CollectionsService.LoadRequest(rootPath, path),
        CollectionsService.LoadRequestOverride(rootPath, path),
      ]);
      this.applyRequest(request);
      this.applyOverride(override);
      void this.loadLastRun(path);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  /** Name of the last version written to disk, from the saved draft snapshot. */
  private savedName(): string {
    try {
      return (JSON.parse(this.snapshot()) as Request).Name ?? '';
    } catch {
      return '';
    }
  }

  private applyRequest(request: Request): void {
    this.name.set(request.Name);
    this.method.set(request.Method || 'GET');
    this.protocol.set(normalizeProtocol(request.Protocol));
    this.url.set(request.URL);
    this.documentation.set(request.Documentation ?? '');
    this.parameters.set(request.Parameters ?? []);
    this.pathParameters.set(request.PathParameters ?? []);
    this.headers.set(request.Headers ?? []);
    this.authentication.set({
      ...defaultAuthentication(),
      ...(request.Authentication ?? {}),
    });
    this.body.set({
      Type: request.Body?.Type ?? BodyType.BodyNone,
      Content: request.Body?.Content ?? '',
      Fields: request.Body?.Fields ?? null,
    });
    this.preRequestScript.set(request.Scripts?.PreRequest ?? '');
    this.postResponseScript.set(request.Scripts?.PostResponse ?? '');
    this.activeTab.set('parameters');
    this.snapshot.set(JSON.stringify(this.draft()));
    void this.refreshOAuth2Status();
  }

  /** Shows the local version of a tab whenever it has one. */
  private applyOverride(override: RequestOverride): void {
    const overridesParameters = !!override.Parameters || !!override.PathParameters;
    this.localParameters.set(
      override.Parameters
        ? [...override.Parameters]
        : overridesParameters
          ? this.parameters().map((parameter) => ({ ...parameter }))
          : null,
    );
    this.localPathParameters.set(override.PathParameters ? [...override.PathParameters] : null);
    this.localBody.set(override.Body ? { ...override.Body } : null);
    this.parametersSource.set(overridesParameters ? 'local' : 'shared');
    this.bodySource.set(override.Body ? 'local' : 'shared');
    this.overrideSnapshot.set(JSON.stringify(this.overrideDraft()));
  }

  private async refreshOAuth2Status(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const requestPath = this.requestPath();
    const authentication = this.authentication();
    if (
      !rootPath ||
      !requestPath ||
      authentication.Type !== AuthenticationType.AuthenticationOAuth2 ||
      !authentication.GrantType
    ) {
      this.oauth2Status.set(null);
      return;
    }

    try {
      this.oauth2Status.set(
        await OAuth2Service.Status(rootPath, requestPath, this.environmentStore.selected()),
      );
    } catch {
      this.oauth2Status.set(null);
    }
  }
  private async closeResponse(): Promise<void> {
    const response = this.response();
    this.releaseImage();
    this.largeResponseShown.set(false);
    this.response.set(null);
    this.responseBody.set('');
    this.responseHasMore.set(false);
    this.responseNextOffset = 0;

    if (!response?.ID) {
      return;
    }

    try {
      await HTTPClientService.Close(response.ID);
    } catch {
      // The response may already have expired; nothing else to release.
    }
  }
}

function decodeChunk(chunk: Chunk): string {
  if (chunk.Encoding === 'utf-8') {
    return chunk.Content;
  }

  const binary = atob(chunk.Content);
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

/** "image/png; charset=binary" becomes "image/png". */
function mediaTypeOf(contentType: string): string {
  return contentType.split(';', 1)[0].trim().toLowerCase();
}

function isImageContentType(contentType: string): boolean {
  return mediaTypeOf(contentType).startsWith('image/');
}

function isHtmlContentType(contentType: string): boolean {
  const mediaType = mediaTypeOf(contentType);
  return mediaType === 'text/html' || mediaType === 'application/xhtml+xml';
}

function base64Bytes(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

function concatBytes(parts: Uint8Array[]): Uint8Array {
  const total = parts.reduce((size, part) => size + part.length, 0);
  const result = new Uint8Array(total);
  let offset = 0;
  for (const part of parts) {
    result.set(part, offset);
    offset += part.length;
  }
  return result;
}

function defaultAuthentication(): Authentication {
  return {
    Type: AuthenticationType.AuthenticationInherit,
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

function normalizeProtocol(protocol: Protocol | undefined): Protocol {
  switch (protocol) {
    case Protocol.ProtocolSSE:
      return Protocol.ProtocolSSE;
    case Protocol.ProtocolWebSocket:
      return Protocol.ProtocolWebSocket;
    default:
      return Protocol.ProtocolHTTP;
  }
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}

function hasScriptContent(outcome: ScriptOutcome | null | undefined): boolean {
  return Boolean(
    outcome &&
    (outcome.Error ||
      outcome.EnvironmentError ||
      (outcome.Logs?.length ?? 0) > 0 ||
      (outcome.Tests?.length ?? 0) > 0 ||
      (outcome.EnvironmentChanges?.length ?? 0) > 0),
  );
}
