import * as PlatformService from '@bindings/github.com/clementherve/awake/src/features/platform/service';

/**
 * Reads the host operating system from the backend and exposes it as a data
 * attribute on the document element. Styles use it to adapt window chrome,
 * for example the space reserved for the macOS traffic lights.
 */
export async function detectPlatform(): Promise<void> {
  const platform = await PlatformService.Name();
  document.documentElement.dataset['platform'] = platform;
}
