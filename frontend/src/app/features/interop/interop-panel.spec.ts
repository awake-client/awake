import { extensionPattern, isPluginFormat } from './interop-panel';

describe('plugin formats', () => {
  it('recognizes plugin:<id>/<format>', () => {
    expect(isPluginFormat('plugin:har/har')).toBe(true);
    expect(isPluginFormat('openapi')).toBe(false);
  });

  it('builds the file chooser pattern from the extensions', () => {
    expect(extensionPattern(['.har', '.json'])).toBe('*.har;*.json');
    expect(extensionPattern(null)).toBe('*.*');
  });
});
