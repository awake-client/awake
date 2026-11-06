import { Component, ElementRef, inject, signal } from '@angular/core';
import { ThemePreference, ThemeStore } from '@core/theme-store';
import { Icon, IconName } from '@shared/icon';

interface ThemeOption {
  preference: ThemePreference;
  label: string;
  icon: IconName;
}

const OPTIONS: ThemeOption[] = [
  { preference: 'system', label: 'System', icon: 'monitor' },
  { preference: 'light', label: 'Light', icon: 'sun' },
  { preference: 'dark', label: 'Dark', icon: 'moon' },
];

/** Small menu choosing the light, dark or system theme. */
@Component({
  selector: 'app-theme-picker',
  imports: [Icon],
  host: {
    '(document:click)': 'closeOutside($event)',
    '(document:keydown.escape)': 'isOpen.set(false)',
  },
  template: `
    <button
      type="button"
      class="icon-btn"
      [title]="'Theme: ' + current().label"
      aria-label="Theme"
      aria-haspopup="menu"
      [attr.aria-expanded]="isOpen()"
      (click)="isOpen.set(!isOpen())"
    >
      <app-icon [name]="current().icon" [size]="14" />
    </button>
    @if (isOpen()) {
      <div class="menu" role="menu">
        <p class="menu-title">Theme</p>
        @for (option of options; track option.preference) {
          <button
            type="button"
            role="menuitemradio"
            class="option"
            [class.selected]="theme.preference() === option.preference"
            [attr.aria-checked]="theme.preference() === option.preference"
            (click)="choose(option.preference)"
          >
            <app-icon [name]="option.icon" [size]="14" />
            {{ option.label }}
          </button>
        }
      </div>
    }
  `,
  styles: `
    :host {
      position: relative;
      display: inline-flex;
    }
    .menu {
      position: absolute;
      bottom: calc(100% + 6px);
      right: 0;
      z-index: 50;
      display: flex;
      flex-direction: column;
      min-width: 150px;
      padding: 5px;
      border-radius: var(--radius-lg);
      background: var(--color-surface-raised);
      box-shadow: var(--shadow-raised);
    }
    .menu-title {
      margin: 3px 8px 4px;
      font-size: var(--text-xs);
      font-weight: 600;
      letter-spacing: 0.06em;
      text-transform: uppercase;
      color: var(--color-text-subtle);
    }
    .option {
      display: flex;
      align-items: center;
      gap: 9px;
      height: 28px;
      padding: 0 8px;
      border: none;
      border-radius: var(--radius-md);
      background: transparent;
      color: var(--color-text);
      font-size: var(--text-base);
      text-align: left;
      cursor: pointer;
    }
    .option:hover {
      background: var(--color-active);
    }
    .option.selected app-icon {
      color: var(--color-accent);
    }
    .option.selected {
      color: var(--color-text-strong);
      font-weight: 500;
    }
  `,
})
export class ThemePicker {
  protected readonly theme = inject(ThemeStore);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  protected readonly isOpen = signal(false);
  protected readonly options = OPTIONS;

  protected current(): ThemeOption {
    return OPTIONS.find((option) => option.preference === this.theme.preference()) ?? OPTIONS[0];
  }

  protected choose(preference: ThemePreference): void {
    this.theme.set(preference);
    this.isOpen.set(false);
  }

  protected closeOutside(event: MouseEvent): void {
    if (this.isOpen() && !this.host.nativeElement.contains(event.target as Node)) {
      this.isOpen.set(false);
    }
  }
}
