import { Component, inject, signal } from '@angular/core';
import {
  CatalogEntry,
  MarketplaceView,
  PluginView,
  Setting,
  SettingType,
} from '@bindings/github.com/awake-client/awake/src/features/plugins/models';
import { Confirmation } from '@core/confirmation';
import { Icon } from '@shared/icon';
import { Select, SelectOption } from '@shared/select';
import { PluginApprovalDialog } from './plugin-approval-dialog';
import {
  contributionSummary,
  describePermission,
  hasSettings,
  pluginTitle,
  secretSettings,
  settingsDraft,
  sharedSettings,
  shortCommit,
  stateBadge,
  storedSettings,
} from './plugin-labels';
import { PluginsStore } from './plugins-store';

type PluginsTab = 'installed' | 'marketplaces';

/**
 * Plugins of the workspace: those awake.yaml declares, with what each needs
 * before it runs on this computer, and the marketplaces to install new ones
 * from.
 */
@Component({
  selector: 'app-plugins-panel',
  imports: [Icon, Select, PluginApprovalDialog],
  providers: [PluginsStore],
  templateUrl: './plugins-panel.html',
  styleUrl: './plugins-panel.css',
})
export class PluginsPanel {
  protected readonly store = inject(PluginsStore);
  private readonly confirmation = inject(Confirmation);

  protected readonly tab = signal<PluginsTab>('installed');
  protected readonly marketplaceDraft = signal('');
  protected readonly repositoryDraft = signal('');
  protected readonly refDraft = signal('');
  /** Plugin whose settings are open, and the values being edited. */
  protected readonly settingsOpen = signal<string | null>(null);
  protected readonly settingsValues = signal<Record<string, string>>({});
  /** Secret values typed in the open settings, sent to the keychain on save. */
  protected readonly secretDrafts = signal<Record<string, string>>({});

  protected readonly badge = stateBadge;
  protected readonly title = pluginTitle;
  protected readonly short = shortCommit;
  protected readonly describePermission = describePermission;
  protected readonly contributions = contributionSummary;
  protected readonly sharedSettings = sharedSettings;
  protected readonly secretSettings = secretSettings;
  protected readonly hasSettings = hasSettings;
  protected readonly settingTypes = SettingType;

  constructor() {
    void this.store.load();
  }

  protected async addMarketplace(): Promise<void> {
    const repository = this.marketplaceDraft().trim();
    if (!repository) {
      this.store.errorMessage.set('Enter the git URL of the marketplace repository.');
      return;
    }
    if (await this.store.addMarketplace(repository)) {
      this.marketplaceDraft.set('');
    }
  }

  protected async removeMarketplace(marketplace: MarketplaceView): Promise<void> {
    const confirmed = await this.confirmation.ask({
      title: `Remove the ${marketplace.name} marketplace?`,
      message: 'Plugins installed from it keep working; they just stop receiving updates from it.',
      confirmLabel: 'Remove marketplace',
      danger: true,
    });
    if (confirmed) {
      await this.store.removeMarketplace(marketplace.name);
    }
  }

  protected installFromMarketplace(marketplace: MarketplaceView, entry: CatalogEntry): void {
    void this.store.reviewInstall(
      { marketplace: marketplace.name, pluginId: entry.id },
      `${marketplace.name}/${entry.id}`,
    );
  }

  protected installFromRepository(): void {
    const repository = this.repositoryDraft().trim();
    if (!repository) {
      this.store.errorMessage.set('Enter the git URL of the plugin repository.');
      return;
    }
    void this.store.reviewInstall({ repository, ref: this.refDraft().trim() }, 'repository');
  }

  protected async confirmReview(): Promise<void> {
    if (await this.store.confirmReview()) {
      this.repositoryDraft.set('');
      this.refDraft.set('');
      this.tab.set('installed');
    }
  }

  protected async uninstall(view: PluginView): Promise<void> {
    const confirmed = await this.confirmation.ask({
      title: `Uninstall ${pluginTitle(view)}?`,
      message: 'It is removed from awake.yaml, so your teammates lose it on their next pull.',
      confirmLabel: 'Uninstall',
      danger: true,
    });
    if (confirmed) {
      await this.store.uninstall(view.declaration.id);
    }
  }

  protected toggleSettings(view: PluginView): void {
    if (this.settingsOpen() === view.declaration.id || !view.manifest) {
      this.settingsOpen.set(null);
      return;
    }
    this.settingsValues.set(settingsDraft(view.manifest, view.declaration.settings));
    this.secretDrafts.set({});
    this.store.secretStates.set({});
    this.settingsOpen.set(view.declaration.id);
    if (secretSettings(view.manifest).length > 0) {
      void this.store.loadSecretStates(view.declaration.id);
    }
  }

  protected setSecret(key: string, value: string): void {
    this.secretDrafts.update((drafts) => ({ ...drafts, [key]: value }));
  }

  protected setSetting(key: string, value: string): void {
    this.settingsValues.update((values) => ({ ...values, [key]: value }));
  }

  protected async saveSettings(view: PluginView): Promise<void> {
    if (!view.manifest) {
      return;
    }
    const stored = storedSettings(view.manifest, this.settingsValues());
    if (await this.store.saveSettings(view.declaration.id, stored, this.secretDrafts())) {
      this.settingsOpen.set(null);
    }
  }

  protected selectOptions(setting: Setting): SelectOption[] {
    return (setting.options ?? []).map((option) => ({ value: option, label: option }));
  }

  protected inputValue(event: Event): string {
    return (event.target as HTMLInputElement).value;
  }

  protected checkedValue(event: Event): string {
    return (event.target as HTMLInputElement).checked ? 'true' : 'false';
  }
}
