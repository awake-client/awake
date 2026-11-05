import { Component, computed, inject, signal, viewChild } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import * as CollectionsService from '@bindings/github.com/clementherve/awake/src/features/collections/service';
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
  Protocol,
  Request,
  ScriptOutcome,
} from '@bindings/github.com/clementherve/awake/src/domain/models';
import { EnvironmentStore } from '@core/environment-store';
import { WorkspaceStore } from '@core/workspace-store';
import { StreamPanel } from './stream-panel/stream-panel';

/** Sections of the request editor. */
type EditorTab =
  | 'parameters'
  | 'headers'
  | 'authentication'
  | 'body'
  | 'scripts'
  | 'documentation';

const HTTP_METHODS = [
  'GET',
  'POST',
  'PUT',
  'PATCH',
  'DELETE',
  'HEAD',
  'OPTIONS',
] as const;

const TAB_LABELS: Record<EditorTab, string> = {
  parameters: 'Parameters',
  headers: 'Headers',
  authentication: 'Authentication',
  body: 'Body',
  scripts: 'Scripts',
  documentation: 'Documentation',
};

const PROTOCOL_LABELS: Record<string, string> = {
  [Protocol.ProtocolHTTP]: 'HTTP',
  [Protocol.ProtocolSSE]: 'SSE',
  [Protocol.ProtocolWebSocket]: 'WebSocket',
};

/**
 * Edits a single request file. Changes stay local until the user saves, which
 * writes the canonical YAML representation back to disk.
 */
@Component({
  selector: 'app-request-editor',
  imports: [FormsModule, StreamPanel],
  templateUrl: './request-editor.html',
  styleUrl: './request-editor.css',
  host: {
    '(document:keydown)': 'handleShortcut($event)',
  },
})
export class RequestEditor {
  private readonly route = inject(ActivatedRoute);
  private readonly environmentStore = inject(EnvironmentStore);
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly streamPanel = viewChild<StreamPanel>(StreamPanel);

  protected readonly AuthenticationType = AuthenticationType;
  protected readonly OAuth2GrantType = OAuth2GrantType;
  protected readonly BodyType = BodyType;
  protected readonly Protocol = Protocol;
  protected readonly httpMethods = HTTP_METHODS;
  protected readonly protocols = [
    Protocol.ProtocolHTTP,
    Protocol.ProtocolSSE,
    Protocol.ProtocolWebSocket,
  ];
  protected readonly protocolLabels = PROTOCOL_LABELS;
  protected readonly tabs = Object.keys(TAB_LABELS) as EditorTab[];
  protected readonly tabLabels = TAB_LABELS;
  protected readonly selectedEnvironment = this.environmentStore.selected;

  protected readonly requestPath = signal('');
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
  protected readonly authentication = signal<Authentication>(
    defaultAuthentication(),
  );
  protected readonly body = signal<Body>({ Type: BodyType.BodyNone, Content: '' });
  protected readonly preRequestScript = signal('');
  protected readonly postResponseScript = signal('');
  protected readonly activeTab = signal<EditorTab>('parameters');
  protected readonly oauth2Status = signal<OAuth2Status | null>(null);
  protected readonly oauth2Busy = signal(false);
  protected readonly oauth2Error = signal<string | null>(null);

  protected readonly response = signal<HTTPResponse | null>(null);
  protected readonly responseBody = signal('');
  protected readonly responseHasMore = signal(false);
  protected readonly responseTab = signal<'body' | 'headers' | 'scripts'>(
    'body',
  );
  protected readonly responseError = signal<string | null>(null);
  protected readonly isSending = signal(false);
  protected readonly isLoadingChunk = signal(false);

  private readonly snapshot = signal('');
  private responseNextOffset = 0;

  private readonly draft = computed<Request>(() => ({
    Name: this.name(),
    Method: this.method(),
    URL: this.url(),
    Protocol: this.protocol(),
    Parameters: this.parameters().filter(
      (parameter) => parameter.Name.trim() !== '',
    ),
    Headers: this.headers().filter((header) => header.Name.trim() !== ''),
    Authentication: this.authentication(),
    Body: this.body(),
    Scripts:
      this.preRequestScript().trim() === '' &&
      this.postResponseScript().trim() === ''
        ? null
        : {
            PreRequest: this.preRequestScript(),
            PostResponse: this.postResponseScript(),
          },
    Documentation: this.documentation(),
  }));

  protected readonly isDirty = computed(
    () => JSON.stringify(this.draft()) !== this.snapshot(),
  );

  protected readonly isStreaming = computed(
    () => this.protocol() !== Protocol.ProtocolHTTP,
  );

  protected readonly hasResponseScripts = computed(() => {
    const response = this.response();
    return response ? this.hasScriptOutcome(response) : false;
  });

  protected readonly streamLive = computed(
    () => this.streamPanel()?.isLive() ?? false,
  );

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
  }

  ngOnDestroy(): void {
    void this.streamPanel()?.release();
    void this.closeResponse();
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

      this.name.set(request.Name);
      this.url.set(request.URL);
      this.snapshot.set(JSON.stringify(this.draft()));
      this.statusMessage.set('Saved');
      void this.refreshOAuth2Status();
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

      if (response.Encoding === 'utf-8') {
        this.responseBody.set(response.Preview);
        this.responseNextOffset = new TextEncoder().encode(
          response.Preview,
        ).length;
        this.responseHasMore.set(this.responseNextOffset < response.Size);
      } else {
        this.responseBody.set('');
        this.responseNextOffset = 0;
      }
    } catch (error) {
      this.responseError.set(describeError(error));
    } finally {
      this.isSending.set(false);
    }
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

  protected async closeResponsePanel(): Promise<void> {
    await this.closeResponse();
    this.responseError.set(null);
  }

  protected selectResponseTab(tab: 'body' | 'headers' | 'scripts'): void {
    this.responseTab.set(tab);
  }

  protected hasScriptOutcome(response: HTTPResponse): boolean {
    return (
      hasScriptContent(response.PreRequestOutcome) ||
      hasScriptContent(response.PostResponseOutcome)
    );
  }

  protected scriptOutcomes(
    response: HTTPResponse,
  ): { label: string; outcome: ScriptOutcome }[] {
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

  protected headerEntries(
    response: HTTPResponse,
  ): { name: string; value: string }[] {
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
    this.parameters.update((parameters) => [
      ...parameters,
      { Name: '', Value: '', Enabled: true },
    ]);
  }

  protected updateParameter(
    index: number,
    field: 'Name' | 'Value' | 'Enabled',
    value: string | boolean,
  ): void {
    this.parameters.update((parameters) =>
      parameters.map((parameter, position) =>
        position === index
          ? ({ ...parameter, [field]: value } as Parameter)
          : parameter,
      ),
    );
  }

  protected removeParameter(index: number): void {
    this.parameters.update((parameters) =>
      parameters.filter((_parameter, position) => position !== index),
    );
  }

  protected addHeader(): void {
    this.headers.update((headers) => [
      ...headers,
      { Name: '', Value: '', Enabled: true },
    ]);
  }

  protected updateHeader(
    index: number,
    field: 'Name' | 'Value' | 'Enabled',
    value: string | boolean,
  ): void {
    this.headers.update((headers) =>
      headers.map((header, position) =>
        position === index
          ? ({ ...header, [field]: value } as Header)
          : header,
      ),
    );
  }

  protected removeHeader(index: number): void {
    this.headers.update((headers) =>
      headers.filter((_header, position) => position !== index),
    );
  }

  protected selectAuthenticationType(event: Event): void {
    const type = (event.target as HTMLSelectElement)
      .value as AuthenticationType;
    this.authentication.update((authentication) => {
      if (type !== AuthenticationType.AuthenticationOAuth2) {
        return { ...authentication, Type: type };
      }
      return {
        ...authentication,
        Type: type,
        GrantType:
          authentication.GrantType ||
          OAuth2GrantType.OAuth2GrantAuthorizationCode,
      };
    });

    if (type === AuthenticationType.AuthenticationOAuth2) {
      void this.refreshOAuth2Status();
    } else {
      this.oauth2Status.set(null);
      this.oauth2Error.set(null);
    }
  }

  protected updateAuthentication(
    field:
      | 'Username'
      | 'Password'
      | 'Token'
      | 'Key'
      | 'Value'
      | 'Location'
      | 'AuthorizationURL'
      | 'TokenURL'
      | 'ClientID'
      | 'ClientSecret'
      | 'RedirectURL'
      | 'Audience',
    value: string,
  ): void {
    this.authentication.update((authentication) => ({
      ...authentication,
      [field]: value,
    }));
  }

  protected selectGrantType(event: Event): void {
    const grantType = (event.target as HTMLSelectElement)
      .value as OAuth2GrantType;
    this.authentication.update((authentication) => ({
      ...authentication,
      GrantType: grantType,
    }));
    void this.refreshOAuth2Status();
  }

  protected updateScopes(value: string): void {
    const scopes = value
      .split(/[\s,]+/)
      .map((scope) => scope.trim())
      .filter((scope) => scope !== '');
    this.authentication.update((authentication) => ({
      ...authentication,
      Scopes: scopes,
    }));
  }

  protected scopesText(): string {
    return (this.authentication().Scopes ?? []).join(' ');
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
      await OAuth2Service.Clear(
        rootPath,
        requestPath,
        this.environmentStore.selected(),
      );
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
    return `Token stored until ${new Date(
      status.ExpiresAtMilliseconds,
    ).toLocaleString()}.`;
  }

  protected selectBodyType(event: Event): void {
    const type = (event.target as HTMLSelectElement).value as BodyType;
    this.body.update((body) => ({ ...body, Type: type }));
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
    this.body.update((body) => ({ ...body, Content: value }));
  }

  private async load(path: string): Promise<void> {
    await this.streamPanel()?.release();
    await this.closeResponse();
    this.requestPath.set(path);
    this.isLoading.set(true);
    this.errorMessage.set(null);
    this.statusMessage.set(null);

    try {
      const request = await CollectionsService.LoadRequest(
        this.workspaceStore.rootPath() ?? '',
        path,
      );
      this.applyRequest(request);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  private applyRequest(request: Request): void {
    this.name.set(request.Name);
    this.method.set(request.Method || 'GET');
    this.protocol.set(normalizeProtocol(request.Protocol));
    this.url.set(request.URL);
    this.documentation.set(request.Documentation ?? '');
    this.parameters.set(request.Parameters ?? []);
    this.headers.set(request.Headers ?? []);
    this.authentication.set({
      ...defaultAuthentication(),
      ...(request.Authentication ?? {}),
    });
    this.body.set({
      Type: request.Body?.Type ?? BodyType.BodyNone,
      Content: request.Body?.Content ?? '',
    });
    this.preRequestScript.set(request.Scripts?.PreRequest ?? '');
    this.postResponseScript.set(request.Scripts?.PostResponse ?? '');
    this.activeTab.set('parameters');
    this.snapshot.set(JSON.stringify(this.draft()));
    void this.refreshOAuth2Status();
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
        await OAuth2Service.Status(
          rootPath,
          requestPath,
          this.environmentStore.selected(),
        ),
      );
    } catch {
      this.oauth2Status.set(null);
    }
  }
  private async closeResponse(): Promise<void> {
    const response = this.response();
    this.response.set(null);
    this.responseBody.set('');
    this.responseHasMore.set(false);
    this.responseNextOffset = 0;

    if (!response) {
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
        (outcome.Logs?.length ?? 0) > 0 ||
        (outcome.Tests?.length ?? 0) > 0),
  );
}
