import {
  Component,
  ElementRef,
  computed,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { Router } from '@angular/router';
import { requestEntryCreation } from '@core/collection-events';
import { Icon } from '@shared/icon';
import {
  environmentOptions,
  type EnvironmentChoice,
  type EnvironmentSort,
} from './environment-choice';

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
      <div class="menu" (keydown)="handleKeydown($event)">
        <div class="menu-header">
          <label class="search">
            <app-icon name="search" [size]="13" />
            <input
              #searchInput
              type="search"
              placeholder="Search environments"
              aria-label="Search environments"
              autocomplete="off"
              spellcheck="false"
              [value]="query()"
              (input)="search($event)"
            />
          </label>
          <button
            type="button"
            class="sort"
            [title]="sort() === 'ascending' ? 'Sort Z to A' : 'Sort A to Z'"
            [attr.aria-label]="
              sort() === 'ascending' ? 'Sort environments Z to A' : 'Sort environments A to Z'
            "
            (click)="toggleSort()"
          >
            <app-icon
              [name]="sort() === 'ascending' ? 'sort-ascending' : 'sort-descending'"
              [size]="14"
            />
          </button>
        </div>

        <div class="options" role="listbox" aria-label="Environments">
          @for (option of options(); track option.FileName; let index = $index) {
            <div
              class="option-row"
              [class.highlighted]="index === highlighted()"
              (mouseenter)="highlighted.set(index)"
            >
              <button
                type="button"
                role="option"
                class="option"
                [attr.aria-selected]="option.FileName === selected()"
                (click)="choose(option.FileName)"
              >
                <span class="check" aria-hidden="true">
                  @if (option.FileName === selected()) {
                    <app-icon name="check" [size]="13" />
                  }
                </span>
                <span class="option-dot" [attr.data-color]="option.Color || null"></span>
                <span class="option-name">{{ option.Name }}</span>
                @if (option.Private) {
                  <span class="badge">private</span>
                } @else if (option.HasLocalOverrides) {
                  <span class="badge">local</span>
                }
                @if (option.CollectionOnly) {
                  <span class="badge">collection</span>
                }
              </button>
              @if (option.FileName) {
                <button
                  type="button"
                  class="pin"
                  [class.pinned]="isPinned(option.FileName)"
                  [title]="(isPinned(option.FileName) ? 'Unpin ' : 'Pin ') + option.Name"
                  [attr.aria-label]="(isPinned(option.FileName) ? 'Unpin ' : 'Pin ') + option.Name"
                  [attr.aria-pressed]="isPinned(option.FileName)"
                  (click)="togglePin(option.FileName)"
                >
                  <app-icon name="pin" [size]="13" />
                </button>
                <button
                  type="button"
                  class="edit"
                  [title]="'Edit ' + option.Name"
                  [attr.aria-label]="'Edit ' + option.Name"
                  (click)="edit(option)"
                >
                  <app-icon name="pencil" [size]="13" />
                </button>
              }
            </div>
          }
          @if (options().length === 0) {
            <p class="empty">No environment matches this search.</p>
          }
        </div>

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
  /** File names the user pinned; they are offered first. */
  readonly pinned = input<readonly string[]>([]);
  readonly selected = input('');
  /** Collection receiving a new environment; empty for a global one. */
  readonly scope = input('');
  readonly selectedChange = output<string>();
  /** Asks to edit an environment, from the pencil next to it. */
  readonly editRequested = output<EnvironmentChoice>();
  /** Asks to pin or unpin an environment, from the pin next to it. */
  readonly pinToggle = output<string>();

  private readonly searchInput = viewChild<ElementRef<HTMLInputElement>>('searchInput');

  protected readonly isOpen = signal(false);
  protected readonly highlighted = signal(0);
  protected readonly query = signal('');
  protected readonly sort = signal<EnvironmentSort>('ascending');

  /** "No environment" first, then the matching environments. */
  protected readonly options = computed<EnvironmentChoice[]>(() =>
    environmentOptions(this.environments(), this.pinned(), this.query(), this.sort()),
  );

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
    this.query.set('');
    const index = this.options().findIndex((option) => option.FileName === this.selected());
    this.highlighted.set(Math.max(0, index));
    this.isOpen.set(true);
    setTimeout(() => this.searchInput()?.nativeElement.focus());
  }

  protected search(event: Event): void {
    this.query.set((event.target as HTMLInputElement).value);
    this.highlighted.set(0);
  }

  protected toggleSort(): void {
    this.sort.update((sort) => (sort === 'ascending' ? 'descending' : 'ascending'));
    this.highlighted.set(0);
  }

  protected isPinned(fileName: string): boolean {
    return this.pinned().includes(fileName);
  }

  protected togglePin(fileName: string): void {
    if (fileName) {
      this.pinToggle.emit(fileName);
    }
  }

  protected choose(fileName: string): void {
    this.selectedChange.emit(fileName);
    this.isOpen.set(false);
  }

  protected edit(option: EnvironmentChoice): void {
    this.isOpen.set(false);
    this.editRequested.emit(option);
  }

  protected create(): void {
    this.isOpen.set(false);
    requestEntryCreation({ kind: 'environment', scope: this.scope() });
  }

  protected manage(): void {
    this.isOpen.set(false);
    void this.router.navigate(['/workspace/environments']);
  }

  protected handleKeydown(event: KeyboardEvent): void {
    const options = this.options();
    const count = options.length;
    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault();
        if (!this.isOpen()) {
          this.toggle();
        } else if (count > 0) {
          this.highlighted.update((index) => (index + 1) % count);
        }
        break;
      case 'ArrowUp':
        event.preventDefault();
        if (this.isOpen() && count > 0) {
          this.highlighted.update((index) => (index - 1 + count) % count);
        }
        break;
      case 'Enter':
        if (this.isOpen()) {
          event.preventDefault();
          const option = options[this.highlighted()];
          if (option) {
            this.choose(option.FileName);
          }
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
