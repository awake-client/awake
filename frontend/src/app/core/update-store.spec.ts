import { Status } from '@bindings/github.com/awake-client/awake/src/features/updates/models';
import { isReleaseVersion, phaseOf } from './update-store';

function status(state: string, withRelease: boolean): Status {
  return {
    currentVersion: '0.8.0',
    state,
    installBlocker: '',
    release: withRelease
      ? { version: '0.9.0', name: '', publishedAt: '', url: '', size: 0, changes: [] }
      : null,
  };
}

describe('phaseOf', () => {
  it('offers a release the check found', () => {
    expect(phaseOf(status('available', true))).toBe('available');
  });

  it('reports an up to date application', () => {
    expect(phaseOf(status('up-to-date', false))).toBe('up-to-date');
  });

  it('follows an installation under way', () => {
    expect(phaseOf(status('downloading', true))).toBe('downloading');
    expect(phaseOf(status('verifying', true))).toBe('installing');
    expect(phaseOf(status('installing', true))).toBe('installing');
    expect(phaseOf(status('ready', true))).toBe('ready');
  });
});

describe('isReleaseVersion', () => {
  it('accepts published versions only', () => {
    expect(isReleaseVersion('0.8.0')).toBe(true);
    expect(isReleaseVersion('v1.2.3-rc.1')).toBe(true);
    expect(isReleaseVersion('dev')).toBe(false);
    expect(isReleaseVersion('')).toBe(false);
  });
});
