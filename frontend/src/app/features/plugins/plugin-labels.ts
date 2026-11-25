import {
  Manifest,
  Permission,
  PluginState,
  PluginView,
  SettingType,
} from '@bindings/github.com/awake-client/awake/src/features/plugins/models';

/** Characters of a commit hash shown in lists, as git abbreviates it. */
const SHORT_COMMIT_LENGTH = 7;

/** What a permission lets a plugin do, in the words the approval dialog uses. */
// Wails enums also carry an empty $zero value, hence Partial.
export const PERMISSION_DESCRIPTIONS: Partial<Record<Permission, string>> = {
  [Permission.PermissionNetwork]: 'Send HTTP requests to any server',
  [Permission.PermissionSecrets]: 'Read secret variables from your keychain',
  [Permission.PermissionWorkspaceWrite]: 'Change the files of this workspace',
  [Permission.PermissionClipboard]: 'Write to your clipboard',
};

export function describePermission(permission: Permission): string {
  return PERMISSION_DESCRIPTIONS[permission] ?? permission;
}

/** How the state of a plugin is shown: a label and a tone. */
export interface StateBadge {
  label: string;
  tone: 'success' | 'attention' | 'muted' | 'danger';
}

const STATE_BADGES: Partial<Record<PluginState, StateBadge>> = {
  [PluginState.PluginApproved]: { label: 'Enabled', tone: 'success' },
  [PluginState.PluginNeedsApproval]: { label: 'Needs approval', tone: 'attention' },
  [PluginState.PluginNotDownloaded]: { label: 'Not downloaded', tone: 'muted' },
  [PluginState.PluginIncompatible]: { label: 'Needs a newer Awake', tone: 'danger' },
  [PluginState.PluginInvalid]: { label: 'Invalid', tone: 'danger' },
};

export function stateBadge(state: PluginState): StateBadge {
  return STATE_BADGES[state] ?? { label: state, tone: 'muted' };
}

/** Whether a plugin waits on the user before it runs. */
export function needsAttention(view: PluginView): boolean {
  return (
    view.state === PluginState.PluginNeedsApproval || view.state === PluginState.PluginNotDownloaded
  );
}

export function shortCommit(commit: string): string {
  return commit.slice(0, SHORT_COMMIT_LENGTH);
}

/** The name of a plugin, or its identifier until its manifest is downloaded. */
export function pluginTitle(view: PluginView): string {
  return view.manifest?.name || view.declaration.id;
}

function plural(count: number, singular: string, pluralForm = singular + 's'): string {
  return `${count} ${count === 1 ? singular : pluralForm}`;
}

/** One line per kind of contribution, for the approval dialog and the list. */
export function contributionSummary(manifest: Manifest): string[] {
  const contributions = manifest.contributes ?? {};
  const lines: string[] = [];
  const hooks = contributions.hooks ?? [];
  if (hooks.length > 0) {
    lines.push(`Runs ${hooks.length === 1 ? 'a hook' : 'hooks'} on every request`);
  }
  const counted: [unknown[] | null | undefined, string, string?][] = [
    [contributions.dynamicVariables, 'dynamic variable'],
    [contributions.authentication, 'authentication type'],
    [contributions.importFormats, 'import format'],
    [contributions.exportFormats, 'export format'],
    [contributions.commands, 'command'],
    [contributions.responseViewers, 'response viewer'],
    [contributions.requestTabs, 'request tab'],
  ];
  for (const [items, singular, pluralForm] of counted) {
    if (items && items.length > 0) {
      lines.push(`Adds ${plural(items.length, singular, pluralForm)}`);
    }
  }
  return lines;
}

/** Settings the team shares through awake.yaml; secrets stay out of it. */
export function sharedSettings(manifest: Manifest) {
  return (manifest.settings ?? []).filter((setting) => setting.type !== SettingType.SettingSecret);
}

/** The value each shared setting has today: the stored one, or its default. */
export function settingsDraft(
  manifest: Manifest,
  stored: Record<string, string | undefined> | null | undefined,
): Record<string, string> {
  const draft: Record<string, string> = {};
  for (const setting of sharedSettings(manifest)) {
    draft[setting.key] = stored?.[setting.key] ?? setting.default ?? '';
  }
  return draft;
}

/**
 * The values to store: only those that differ from their default, so a new
 * default in a plugin update reaches the team.
 */
export function storedSettings(
  manifest: Manifest,
  draft: Record<string, string>,
): Record<string, string> {
  const stored: Record<string, string> = {};
  for (const setting of sharedSettings(manifest)) {
    const value = draft[setting.key] ?? '';
    if (value !== (setting.default ?? '')) {
      stored[setting.key] = value;
    }
  }
  return stored;
}
