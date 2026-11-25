import { Injectable, computed, inject, signal } from '@angular/core';
import {
  MarketplaceView,
  PluginPreview,
  PluginSource,
  PluginView,
} from '@bindings/github.com/awake-client/awake/src/features/plugins/models';
import * as PluginsService from '@bindings/github.com/awake-client/awake/src/features/plugins/service';
import { WorkspaceStore } from '@core/workspace-store';
import { needsAttention } from './plugin-labels';

/** What the approval dialog asks the user to confirm. */
export interface PluginReview {
  kind: 'install' | 'update' | 'approve';
  preview: PluginPreview;
}

/**
 * State of the plugins panel: the plugins the workspace declares, the
 * marketplaces it browses, and the review the user is asked to confirm.
 */
@Injectable()
export class PluginsStore {
  private readonly workspaceStore = inject(WorkspaceStore);

  readonly plugins = signal<PluginView[]>([]);
  readonly marketplaces = signal<MarketplaceView[]>([]);
  readonly isLoading = signal(true);
  /** Key of the operation under way, such as 'install:aws-sigv4'. */
  readonly busy = signal<string | null>(null);
  readonly errorMessage = signal<string | null>(null);
  readonly statusMessage = signal<string | null>(null);
  readonly review = signal<PluginReview | null>(null);
  /** Secret settings of the plugin whose settings are open, by key: whether each is set. */
  readonly secretStates = signal<Record<string, boolean>>({});

  readonly attentionCount = computed(() => this.plugins().filter(needsAttention).length);

  private get rootPath(): string {
    return this.workspaceStore.rootPath() ?? '';
  }

  /** Reads the declared plugins and catalogs without touching the network. */
  async load(): Promise<void> {
    this.isLoading.set(true);
    try {
      const [plugins, marketplaces] = await Promise.all([
        PluginsService.ListPlugins(this.rootPath),
        PluginsService.ListMarketplaces(this.rootPath),
      ]);
      this.plugins.set(plugins ?? []);
      this.marketplaces.set(marketplaces ?? []);
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoading.set(false);
    }
  }

  refreshMarketplaces(): Promise<boolean> {
    return this.run('refresh', async () => {
      this.marketplaces.set((await PluginsService.RefreshMarketplaces(this.rootPath)) ?? []);
    });
  }

  addMarketplace(repository: string): Promise<boolean> {
    return this.run('add-marketplace', async () => {
      const added = await PluginsService.AddMarketplace(this.rootPath, repository.trim());
      this.marketplaces.update((marketplaces) => [...marketplaces, added]);
      return `Added the ${added.name} marketplace.`;
    });
  }

  removeMarketplace(name: string): Promise<boolean> {
    return this.run(`remove-marketplace:${name}`, async () => {
      await PluginsService.RemoveMarketplace(this.rootPath, name);
      this.marketplaces.update((marketplaces) =>
        marketplaces.filter((marketplace) => marketplace.name !== name),
      );
    });
  }

  /** Downloads what a source points at and asks the user to review it. */
  reviewInstall(source: PluginSource, key: string): Promise<boolean> {
    return this.run(`preview:${key}`, async () => {
      const preview = await PluginsService.Preview(this.rootPath, source);
      this.review.set({ kind: preview.installedCommit ? 'update' : 'install', preview });
    });
  }

  checkUpdate(pluginID: string): Promise<boolean> {
    return this.run(`update:${pluginID}`, async () => {
      const preview = await PluginsService.CheckUpdate(this.rootPath, pluginID);
      if (!preview) {
        return 'This plugin is up to date.';
      }
      this.review.set({ kind: 'update', preview });
      return undefined;
    });
  }

  /** Downloads a plugin a teammate added and asks the user to approve it. */
  reviewApproval(view: PluginView): Promise<boolean> {
    return this.run(`approve:${view.declaration.id}`, async () => {
      const downloaded = await PluginsService.Download(this.rootPath, view.declaration.id);
      this.replacePlugin(downloaded);
      if (!downloaded.manifest) {
        throw new Error(downloaded.problem || 'The plugin could not be read.');
      }
      this.review.set({
        kind: 'approve',
        preview: {
          source: { pluginId: downloaded.declaration.id },
          repository: downloaded.declaration.repository,
          ref: downloaded.declaration.ref,
          commit: downloaded.declaration.commit,
          manifest: downloaded.manifest,
        },
      });
    });
  }

  /** Applies the review the user confirmed. */
  confirmReview(): Promise<boolean> {
    const review = this.review();
    if (!review) {
      return Promise.resolve(false);
    }
    const { preview } = review;
    return this.run('confirm', async () => {
      const view =
        review.kind === 'approve'
          ? await PluginsService.Approve(this.rootPath, preview.manifest.id, preview.commit)
          : await PluginsService.Install(this.rootPath, preview.source, preview.commit);
      this.review.set(null);
      this.replacePlugin(view);
      this.markInstalled(view.declaration.id);
      const verbs = { install: 'Installed', update: 'Updated', approve: 'Enabled' };
      return `${verbs[review.kind]} ${preview.manifest.name}.`;
    });
  }

  cancelReview(): void {
    this.review.set(null);
  }

  revoke(pluginID: string): Promise<boolean> {
    return this.run(`revoke:${pluginID}`, async () => {
      await PluginsService.Revoke(this.rootPath, pluginID);
      await this.reloadPlugins();
      return 'The plugin is disabled on this computer. It stays installed for your team.';
    });
  }

  uninstall(pluginID: string): Promise<boolean> {
    return this.run(`uninstall:${pluginID}`, async () => {
      await PluginsService.Uninstall(this.rootPath, pluginID);
      this.plugins.update((plugins) =>
        plugins.filter((plugin) => plugin.declaration.id !== pluginID),
      );
      this.marketplaces.update((marketplaces) =>
        marketplaces.map((marketplace) => ({
          ...marketplace,
          plugins: (marketplace.plugins ?? []).map((entry) =>
            entry.id === pluginID ? { ...entry, installed: false } : entry,
          ),
        })),
      );
    });
  }

  /** Whether each secret setting of a plugin has a value on this computer. */
  async loadSecretStates(pluginID: string): Promise<void> {
    try {
      const states = (await PluginsService.SecretSettings(this.rootPath, pluginID)) ?? [];
      this.secretStates.set(Object.fromEntries(states.map((state) => [state.key, state.isSet])));
    } catch (error) {
      this.secretStates.set({});
      this.errorMessage.set(describeError(error));
    }
  }

  /**
   * Saves the shared settings in awake.yaml and the secret ones typed in the
   * keychain. A secret left empty keeps its stored value.
   */
  saveSettings(
    pluginID: string,
    settings: Record<string, string>,
    secrets: Record<string, string>,
  ): Promise<boolean> {
    return this.run(`settings:${pluginID}`, async () => {
      this.replacePlugin(await PluginsService.UpdateSettings(this.rootPath, pluginID, settings));
      for (const [key, value] of Object.entries(secrets)) {
        if (value) {
          await PluginsService.SetSecretSetting(this.rootPath, pluginID, key, value);
        }
      }
      await this.reloadPlugins();
      return 'Settings saved. Secret values stay on this computer.';
    });
  }

  clearSecret(pluginID: string, key: string): Promise<boolean> {
    return this.run(`secret:${pluginID}:${key}`, async () => {
      await PluginsService.SetSecretSetting(this.rootPath, pluginID, key, '');
      this.secretStates.update((states) => ({ ...states, [key]: false }));
      await this.reloadPlugins();
    });
  }

  isBusy(key: string): boolean {
    return this.busy() === key;
  }

  private async reloadPlugins(): Promise<void> {
    this.plugins.set((await PluginsService.ListPlugins(this.rootPath)) ?? []);
  }

  private replacePlugin(view: PluginView): void {
    this.plugins.update((plugins) => {
      const position = plugins.findIndex((plugin) => plugin.declaration.id === view.declaration.id);
      if (position < 0) {
        return [...plugins, view];
      }
      return plugins.map((plugin, index) => (index === position ? view : plugin));
    });
  }

  private markInstalled(pluginID: string): void {
    this.marketplaces.update((marketplaces) =>
      marketplaces.map((marketplace) => ({
        ...marketplace,
        plugins: (marketplace.plugins ?? []).map((entry) =>
          entry.id === pluginID ? { ...entry, installed: true } : entry,
        ),
      })),
    );
  }

  /**
   * Runs one operation at a time and reports its outcome. The operation may
   * return a success message. Resolves to whether it succeeded.
   */
  private async run(
    key: string,
    operation: () => Promise<string | void | undefined>,
  ): Promise<boolean> {
    if (this.busy() !== null) {
      return false;
    }
    this.busy.set(key);
    this.errorMessage.set(null);
    this.statusMessage.set(null);
    try {
      const message = await operation();
      if (message) {
        this.statusMessage.set(message);
      }
      return true;
    } catch (error) {
      this.errorMessage.set(describeError(error));
      return false;
    } finally {
      this.busy.set(null);
    }
  }
}

function describeError(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
