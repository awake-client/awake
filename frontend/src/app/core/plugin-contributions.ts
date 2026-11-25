import { Injectable, effect, inject, signal } from '@angular/core';
import {
  AuthenticationItem,
  CommandItem,
  DynamicVariableContribution,
  UIContributions,
  ViewItem,
} from '@bindings/github.com/awake-client/awake/src/features/plugins/models';
import * as PluginsService from '@bindings/github.com/awake-client/awake/src/features/plugins/service';
import { WorkspaceStore } from './workspace-store';

/** The contributions of the enabled plugins, with every list present. */
export interface PluginInterface {
  commands: CommandItem[];
  authentication: AuthenticationItem[];
  dynamicVariables: DynamicVariableContribution[];
  responseViewers: ViewItem[];
  requestTabs: ViewItem[];
}

const EMPTY_CONTRIBUTIONS: PluginInterface = {
  commands: [],
  authentication: [],
  dynamicVariables: [],
  responseViewers: [],
  requestTabs: [],
};

/**
 * What the enabled plugins of the open workspace add to the interface:
 * commands, authentication types, dynamic variables and views. Screens that
 * change plugins call `reload`.
 */
@Injectable({ providedIn: 'root' })
export class PluginContributions {
  private readonly workspaceStore = inject(WorkspaceStore);
  private loadToken = 0;

  readonly contributions = signal<PluginInterface>(EMPTY_CONTRIBUTIONS);

  constructor() {
    effect(() => {
      void this.load(this.workspaceStore.rootPath());
    });
  }

  reload(): Promise<void> {
    return this.load(this.workspaceStore.rootPath());
  }

  /** The authentication a plugin provides under this type, if it is enabled. */
  authentication(type: string): AuthenticationItem | null {
    return this.contributions().authentication.find((item) => item.type === type) ?? null;
  }

  /** Response viewers able to render a response of this content type. */
  viewersFor(contentType: string): ViewItem[] {
    return this.contributions().responseViewers.filter((viewer) =>
      matchesContentType(contentType, viewer.contentTypes ?? []),
    );
  }

  private async load(rootPath: string | null): Promise<void> {
    const token = ++this.loadToken;
    if (!rootPath) {
      this.contributions.set(EMPTY_CONTRIBUTIONS);
      return;
    }
    try {
      const contributions = await PluginsService.Contributions(rootPath);
      if (token === this.loadToken) {
        this.contributions.set(normalize(contributions));
      }
    } catch {
      // A build without plugins, or a workspace that cannot be read, simply
      // contributes nothing.
      if (token === this.loadToken) {
        this.contributions.set(EMPTY_CONTRIBUTIONS);
      }
    }
  }
}

function normalize(contributions: UIContributions | null): PluginInterface {
  if (!contributions) {
    return EMPTY_CONTRIBUTIONS;
  }
  return {
    commands: contributions.commands ?? [],
    authentication: contributions.authentication ?? [],
    dynamicVariables: contributions.dynamicVariables ?? [],
    responseViewers: contributions.responseViewers ?? [],
    requestTabs: contributions.requestTabs ?? [],
  };
}

/** The media type of a Content-Type header, without its parameters. */
export function mediaTypeOf(contentType: string): string {
  return contentType.split(';')[0].trim().toLowerCase();
}

/**
 * Whether a content type matches one of the patterns a viewer declares:
 * an exact media type, `type/*`, or `*` for anything.
 */
export function matchesContentType(contentType: string, patterns: string[]): boolean {
  const mediaType = mediaTypeOf(contentType);
  if (!mediaType) {
    return false;
  }
  return patterns.some((pattern) => {
    const expected = pattern.trim().toLowerCase();
    if (expected === '*' || expected === '*/*') {
      return true;
    }
    if (expected.endsWith('/*')) {
      return mediaType.startsWith(expected.slice(0, -1));
    }
    return mediaType === expected;
  });
}
