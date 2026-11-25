import { AuthenticationType } from '@bindings/github.com/awake-client/awake/src/domain/models';
import {
  describeAuthentication,
  isPluginAuthentication,
  pluginOfAuthentication,
} from './auth-editor';

describe('plugin authentication types', () => {
  it('recognizes plugin:<id>/<type>', () => {
    expect(isPluginAuthentication('plugin:aws-sigv4/awsSigV4')).toBe(true);
    expect(isPluginAuthentication(AuthenticationType.AuthenticationBearer)).toBe(false);
    expect(isPluginAuthentication(undefined)).toBe(false);
    expect(pluginOfAuthentication('plugin:aws-sigv4/awsSigV4')).toBe('aws-sigv4');
  });

  it('names the plugin in the description', () => {
    const description = describeAuthentication({
      Type: 'plugin:aws-sigv4/awsSigV4' as AuthenticationType,
    } as Parameters<typeof describeAuthentication>[0]);
    expect(description).toBe('Authentication from the aws-sigv4 plugin');
  });
});
