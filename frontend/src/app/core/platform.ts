import { Events, Window } from '@wailsio/runtime';
import * as PlatformService from '@bindings/github.com/awake-client/awake/src/features/platform/service';

const fullscreenEvent = 'common:WindowFullscreen';
const unfullscreenEvent = 'common:WindowUnFullscreen';

/**
 * Reads the host operating system and the window state from the backend and
 * exposes them as data attributes on the document element. Styles use them to
 * adapt window chrome, for example the space reserved for the macOS traffic
 * lights.
 */
export async function detectPlatform(): Promise<void> {
  const platform = await PlatformService.Name();
  document.documentElement.dataset['platform'] = platform;
  await detectFullscreen();
}

/**
 * Tracks whether the window is fullscreen. macOS hides the traffic lights in
 * fullscreen, so the title bar can reclaim their space instead of leaving an
 * empty inset.
 */
async function detectFullscreen(): Promise<void> {
  const apply = (fullscreen: boolean) => {
    document.documentElement.dataset['fullscreen'] = fullscreen ? 'true' : 'false';
  };

  try {
    apply(await Window.IsFullscreen());
  } catch {
    // The window API is unavailable outside the Wails runtime.
  }

  Events.On(fullscreenEvent, () => apply(true));
  Events.On(unfullscreenEvent, () => apply(false));
}

/**
 * Reports whether the host is macOS, from the platform detected at startup.
 * Falls back to the browser when the backend is not available, as in tests.
 */
export function isMacPlatform(): boolean {
  const platform = document.documentElement.dataset['platform'];
  if (platform) {
    return platform === 'darwin';
  }
  return /mac/i.test(navigator.userAgent);
}
