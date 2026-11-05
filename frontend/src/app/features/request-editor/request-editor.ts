import { Component, computed, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import * as CollectionsService from '@bindings/github.com/clementherve/awake/src/features/collections/service';
import {
  Authentication,
  AuthenticationType,
  Body,
  BodyType,
  Header,
  Parameter,
  Request,
} from '@bindings/github.com/clementherve/awake/src/domain/models';
import { WorkspaceStore } from '@core/workspace-store';

/** Sections of the request editor. */
type EditorTab =
  | 'parameters'
  | 'headers'
  | 'authentication'
  | 'body'
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
  documentation: 'Documentation',
};

/**
 * Edits a single request file. Changes stay local until the user saves, which
 * writes the canonical YAML representation back to disk.
 */
@Component({
  selector: 'app-request-editor',
  imports: [FormsModule],
  templateUrl: './request-editor.html',
  styleUrl: './request-editor.css',
  host: {
    '(document:keydown)': 'handleShortcut($event)',
  },
})
export class RequestEditor {
  private readonly route = inject(ActivatedRoute);
  private readonly workspaceStore = inject(WorkspaceStore);

  protected readonly AuthenticationType = AuthenticationType;
  protected readonly BodyType = BodyType;
  protected readonly httpMethods = HTTP_METHODS;
  protected readonly tabs = Object.keys(TAB_LABELS) as EditorTab[];
  protected readonly tabLabels = TAB_LABELS;

  protected readonly requestPath = signal('');
  protected readonly isLoading = signal(true);
  protected readonly isSaving = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly statusMessage = signal<string | null>(null);

  protected readonly name = signal('');
  protected readonly method = signal<string>('GET');
  protected readonly url = signal('');
  protected readonly documentation = signal('');
  protected readonly parameters = signal<Parameter[]>([]);
  protected readonly headers = signal<Header[]>([]);
  protected readonly authentication = signal<Authentication>(
    defaultAuthentication(),
  );
  protected readonly body = signal<Body>({ Type: BodyType.BodyNone, Content: '' });
  protected readonly activeTab = signal<EditorTab>('parameters');

  private readonly snapshot = signal('');

  private readonly draft = computed<Request>(() => ({
    Name: this.name(),
    Method: this.method(),
    URL: this.url(),
    Parameters: this.parameters().filter(
      (parameter) => parameter.Name.trim() !== '',
    ),
    Headers: this.headers().filter((header) => header.Name.trim() !== ''),
    Authentication: this.authentication(),
    Body: this.body(),
    Documentation: this.documentation(),
  }));

  protected readonly isDirty = computed(
    () => JSON.stringify(this.draft()) !== this.snapshot(),
  );

  constructor() {
    this.route.url.pipe(takeUntilDestroyed()).subscribe((segments) => {
      const path = segments.map((segment) => segment.path).join('/');
      if (path) {
        void this.load(path);
      }
    });
  }

  protected selectTab(tab: EditorTab): void {
    this.activeTab.set(tab);
  }

  protected async save(): Promise<void> {
    const draft = this.draft();
    if (draft.Name.trim() === '' || draft.URL.trim() === '') {
      this.errorMessage.set('A request needs a name and a URL.');
      return;
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
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isSaving.set(false);
    }
  }

  protected handleShortcut(event: KeyboardEvent): void {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's') {
      event.preventDefault();
      if (this.isDirty() && !this.isSaving()) {
        void this.save();
      }
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
    this.authentication.update((authentication) => ({ ...authentication, Type: type }));
  }

  protected updateAuthentication(
    field: 'Username' | 'Password' | 'Token' | 'Key' | 'Value' | 'Location',
    value: string,
  ): void {
    this.authentication.update((authentication) => ({
      ...authentication,
      [field]: value,
    }));
  }

  protected selectBodyType(event: Event): void {
    const type = (event.target as HTMLSelectElement).value as BodyType;
    this.body.update((body) => ({ ...body, Type: type }));
  }

  protected updateBodyContent(value: string): void {
    this.body.update((body) => ({ ...body, Content: value }));
  }

  private async load(path: string): Promise<void> {
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
    this.activeTab.set('parameters');
    this.snapshot.set(JSON.stringify(this.draft()));
  }
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
  };
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
