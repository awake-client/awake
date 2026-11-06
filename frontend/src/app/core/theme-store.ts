import { Injectable, signal } from '@angular/core';

const STORAGE_KEY = 'awake.theme';

/** Theme chosen by the user; `system` follows the operating system. */
export type ThemePreference = 'system' | 'light' | 'dark';

/**
 * Applies the chosen theme through a `data-theme` attribute on the document,
 * which the design tokens read. The choice is remembered on this computer.
 */
@Injectable({ providedIn: 'root' })
export class ThemeStore {
  readonly preference = signal<ThemePreference>(readPreference());

  constructor() {
    apply(this.preference());
  }

  set(preference: ThemePreference): void {
    this.preference.set(preference);
    apply(preference);
    try {
      localStorage.setItem(STORAGE_KEY, preference);
    } catch {
      // The theme still applies to this session.
    }
  }
}

function apply(preference: ThemePreference): void {
  if (preference === 'system') {
    delete document.documentElement.dataset['theme'];
  } else {
    document.documentElement.dataset['theme'] = preference;
  }
}

function readPreference(): ThemePreference {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    return stored === 'light' || stored === 'dark' ? stored : 'system';
  } catch {
    return 'system';
  }
}
