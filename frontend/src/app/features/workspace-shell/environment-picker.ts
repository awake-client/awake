import { Component, ElementRef, computed, inject, input, output, signal } from '@angular/core';
import { Router } from '@angular/router';
import { requestEntryCreation } from '@core/collection-events';
import { Icon } from '@shared/icon';

/** An environment offered by the picker. */
export interface EnvironmentChoice {
  FileName: string;
  Name: string;
  HasLocalOverrides: boolean;
  CollectionOnly: boolean;
  Color: string;
}

/**
 * Environment picker of the title bar. A custom menu rather than a native
 * select, so it matches the interface and can offer to create or manage
 * environments right where they are chosen.
 */
@Component({
  selector: 'app-environment-picker',
  imports: [Icon],
  host: {
    '(document:click)': 'handleOutsideClick($event)',
  },
  template: `
    <button
      type="button"
      class="picker"
      [class.active]="selected()"
      [class.open]="isOpen()"
      [attr.data-color]="selectedColor() || null"
      aria-haspopup="listbox"
      [attr.aria-expanded]="isOpen()"
      title="Active environment"
      (click)="toggle()"
      (keydown)="handleKeydown($event)"
    >
      <span class="dot" aria-hidden="true"></span>
      <span class="label">{{ selectedLabel() }}</span>
      <app-icon name="chevron-down" [size]="12" />
    </button>

    @if (isOpen()) {
      <div class="menu" role="listbox" aria-label="Environments" (keydown)="handleKeydown($event)">
        <p class="menu-title">Environment</p>
        @for (option of options(); track option.FileName; let index = $index) {
          <button
            type="button"
            role="option"
            class="option"
            [class.highlighted]="index === highlighted()"
            [attr.aria-selected]="option.FileName === selected()"
            (mouseenter)="highlighted.set(index)"
            (click)="choose(option.FileName)"
          >
            <span class="check" aria-hidden="true">
              @if (option.FileName === selected()) {
                <app-icon name="check" [size]="13" />
              }
            </span>
            <span class="option-dot" [attr.data-color]="option.Color || null"></span>
            <span class="option-name">{{ option.Name }}</span>
            @if (option.HasLocalOverrides) {
              <span class="badge">local</span>
            }
            @if (option.CollectionOnly) {
              <span class="badge">collections</span>
            }
          </button>
        }
        <hr />
        <button type="button" class="action" (click)="create()">
          <app-icon name="plus" [size]="14" />
          New environment…
        </button>
        <button type="button" class="action" (click)="manage()">
          <app-icon name="settings" [size]="14" />
          Manage environments
        </button>
      </div>
    }
  `,
  styleUrl: './environment-picker.css',
})
export class EnvironmentPicker {
  private readonly router = inject(Router);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly environments = input<EnvironmentChoice[]>([]);
  readonly selected = input('');
  readonly selectedChange = output<string>();

  protected readonly isOpen = signal(false);
  protected readonly highlighted = signal(0);

  /** "No environment" first, then every environment name. */
  protected readonly options = computed<EnvironmentChoice[]>(() => [
    {
      FileName: '',
      Name: 'No environment',
      HasLocalOverrides: false,
      CollectionOnly: false,
      Color: '',
    },
    ...this.environments(),
  ]);

  protected readonly selectedColor = computed(
    () =>
      this.environments().find((environment) => environment.FileName === this.selected())?.Color ??
      '',
  );

  protected readonly selectedLabel = computed(
    () =>
      this.environments().find((environment) => environment.FileName === this.selected())?.Name ??
      'No environment',
  );

  protected toggle(): void {
    if (this.isOpen()) {
      this.isOpen.set(false);
      return;
    }
    const index = this.options().findIndex((option) => option.FileName === this.selected());
    this.highlighted.set(Math.max(0, index));
    this.isOpen.set(true);
  }

  protected choose(fileName: string): void {
    this.selectedChange.emit(fileName);
    this.isOpen.set(false);
  }

  protected create(): void {
    this.isOpen.set(false);
    requestEntryCreation({ kind: 'environment' });
  }

  protected manage(): void {
    this.isOpen.set(false);
    void this.router.navigate(['/workspace/environments']);
  }

  protected handleKeydown(event: KeyboardEvent): void {
    const count = this.options().length;
    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault();
        if (!this.isOpen()) {
          this.toggle();
        } else {
          this.highlighted.update((index) => (index + 1) % count);
        }
        break;
      case 'ArrowUp':
        event.preventDefault();
        this.highlighted.update((index) => (index - 1 + count) % count);
        break;
      case 'Enter':
        if (this.isOpen()) {
          event.preventDefault();
          this.choose(this.options()[this.highlighted()].FileName);
        }
        break;
      case 'Escape':
        this.isOpen.set(false);
        break;
    }
  }

  protected handleOutsideClick(event: MouseEvent): void {
    if (this.isOpen() && !this.host.nativeElement.contains(event.target as Node)) {
      this.isOpen.set(false);
    }
  }
}
