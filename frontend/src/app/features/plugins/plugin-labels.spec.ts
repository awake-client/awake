import {
  Hook,
  Manifest,
  PluginState,
  PluginView,
  SettingType,
} from '@bindings/github.com/awake-client/awake/src/features/plugins/models';
import {
  contributionSummary,
  needsAttention,
  pluginTitle,
  settingsDraft,
  shortCommit,
  stateBadge,
  storedSettings,
} from './plugin-labels';

function manifest(overrides: Partial<Manifest> = {}): Manifest {
  return {
    version: 1,
    id: 'example',
    name: 'Example',
    pluginVersion: '1.0.0',
    main: 'dist/plugin.js',
    contributes: {},
    ...overrides,
  };
}

function view(state: PluginState, withManifest = true): PluginView {
  return {
    declaration: {
      id: 'example',
      repository: 'https://example.com/example.git',
      commit: 'a'.repeat(40),
    },
    state,
    manifest: withManifest ? manifest() : null,
  };
}

describe('stateBadge', () => {
  it('names every state', () => {
    expect(stateBadge(PluginState.PluginApproved)).toEqual({ label: 'Enabled', tone: 'success' });
    expect(stateBadge(PluginState.PluginNeedsApproval).tone).toBe('attention');
    expect(stateBadge(PluginState.PluginInvalid).tone).toBe('danger');
  });
});

describe('needsAttention', () => {
  it('flags plugins waiting on the user', () => {
    expect(needsAttention(view(PluginState.PluginNeedsApproval))).toBe(true);
    expect(needsAttention(view(PluginState.PluginNotDownloaded, false))).toBe(true);
    expect(needsAttention(view(PluginState.PluginApproved))).toBe(false);
    expect(needsAttention(view(PluginState.PluginInvalid))).toBe(false);
  });
});

describe('pluginTitle', () => {
  it('prefers the manifest name and falls back to the identifier', () => {
    expect(pluginTitle(view(PluginState.PluginApproved))).toBe('Example');
    expect(pluginTitle(view(PluginState.PluginNotDownloaded, false))).toBe('example');
  });
});

describe('shortCommit', () => {
  it('keeps seven characters', () => {
    expect(shortCommit('3f2a9c1e0b7d4a5f6e8c9d0a1b2c3d4e5f6a7b8c')).toBe('3f2a9c1');
  });
});

describe('contributionSummary', () => {
  it('lists each kind of contribution once', () => {
    const summary = contributionSummary(
      manifest({
        contributes: {
          hooks: [Hook.HookBeforeRequest, Hook.HookAfterResponse],
          dynamicVariables: [{ name: '$awsDate' }, { name: '$awsRegion' }],
          commands: [{ id: 'refresh', title: 'Refresh' }],
        },
      }),
    );

    expect(summary).toEqual([
      'Runs hooks on every request',
      'Adds 2 dynamic variables',
      'Adds 1 command',
    ]);
  });

  it('is empty for a plugin contributing nothing', () => {
    expect(contributionSummary(manifest())).toEqual([]);
  });
});

describe('settings', () => {
  const withSettings = manifest({
    settings: [
      {
        key: 'region',
        type: SettingType.SettingSelect,
        label: 'Region',
        options: ['eu', 'us'],
        default: 'eu',
      },
      { key: 'verbose', type: SettingType.SettingBoolean, label: 'Verbose' },
      { key: 'token', type: SettingType.SettingSecret, label: 'Token' },
    ],
  });

  it('drafts the stored values over the defaults, without secrets', () => {
    expect(settingsDraft(withSettings, { verbose: 'true' })).toEqual({
      region: 'eu',
      verbose: 'true',
    });
    expect(settingsDraft(withSettings, null)).toEqual({ region: 'eu', verbose: '' });
  });

  it('stores only the values that differ from their default', () => {
    expect(storedSettings(withSettings, { region: 'eu', verbose: 'true', token: 'leak' })).toEqual({
      verbose: 'true',
    });
    expect(storedSettings(withSettings, { region: 'us', verbose: '' })).toEqual({ region: 'us' });
  });
});
