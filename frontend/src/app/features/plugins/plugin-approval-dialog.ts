import { Component, ElementRef, computed, effect, input, output, viewChild } from '@angular/core';
import { Icon } from '@shared/icon';
import { contributionSummary, describePermission, shortCommit } from './plugin-labels';
import { PluginReview } from './plugins-store';

const CONFIRM_LABELS = { install: 'Install', update: 'Update', approve: 'Enable' } as const;
const TITLES = {
  install: 'Install this plugin?',
  update: 'Update this plugin?',
  approve: 'Enable this plugin on this computer?',
} as const;

/**
 * Shows exactly what will run before a plugin is installed, updated or
 * enabled: its repository, the commit pinned in awake.yaml, the permissions
 * it asks for and what it adds to Awake.
 */
@Component({
  selector: 'app-plugin-approval-dialog',
  imports: [Icon],
  template: `
    <dialog #dialog class="approval" (cancel)="$event.preventDefault(); cancelled.emit()">
      @if (review(); as current) {
        <div class="body">
          <span class="badge" aria-hidden="true"><app-icon name="plug" [size]="16" /></span>
          <div class="text">
            <h2>{{ title() }}</h2>
            <p class="name">
              {{ current.preview.manifest.name }}
              <span class="version">{{ current.preview.manifest.pluginVersion }}</span>
            </p>
            @if (current.preview.manifest.description) {
              <p class="description">{{ current.preview.manifest.description }}</p>
            }

            <dl class="origin">
              <dt>Repository</dt>
              <dd class="mono">{{ current.preview.repository }}</dd>
              <dt>Commit</dt>
              <dd class="mono">
                @if (current.preview.installedCommit) {
                  {{ short(current.preview.installedCommit) }} →
                }
                {{ short(current.preview.commit) }}
                @if (current.preview.ref) {
                  <span class="ref">({{ current.preview.ref }})</span>
                }
              </dd>
            </dl>

            <h3>Permissions</h3>
            @if (permissions().length === 0) {
              <p class="muted">None: the plugin only works on the requests it is given.</p>
            } @else {
              <ul class="permissions">
                @for (permission of permissions(); track permission) {
                  <li>
                    <app-icon name="key" [size]="13" />
                    {{ describe(permission) }}
                  </li>
                }
              </ul>
            }

            @if (contributions().length > 0) {
              <h3>Adds to Awake</h3>
              <ul class="contributions">
                @for (line of contributions(); track line) {
                  <li>{{ line }}</li>
                }
              </ul>
            }

            <p class="warning">
              @if (current.kind === 'approve') {
                A teammate added this plugin to awake.yaml. It runs with your credentials once
                enabled, so check that you trust its repository.
              } @else {
                The commit is pinned in awake.yaml: your teammates will be asked to enable it too.
              }
            </p>
          </div>
        </div>
        <footer>
          <button type="button" class="btn btn-ghost" (click)="cancelled.emit()">Cancel</button>
          <button
            type="button"
            class="btn btn-primary"
            [disabled]="busy()"
            (click)="confirmed.emit()"
          >
            {{ busy() ? 'Working…' : confirmLabel() }}
          </button>
        </footer>
      }
    </dialog>
  `,
  styles: `
    .approval {
      width: min(520px, calc(100vw - 48px));
      padding: 0;
      border: none;
      border-radius: var(--radius-xl);
      background: var(--color-surface);
      color: var(--color-text);
      box-shadow: var(--shadow-raised);
    }
    .approval::backdrop {
      background: var(--color-backdrop);
      backdrop-filter: blur(3px);
    }
    .body {
      display: flex;
      gap: 14px;
      padding: 22px 22px 18px;
    }
    .badge {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      flex: 0 0 auto;
      width: 34px;
      height: 34px;
      border-radius: var(--radius-md);
      background: var(--color-accent-soft);
      color: var(--color-accent);
    }
    .text {
      min-width: 0;
      flex: 1;
    }
    h2 {
      margin: 4px 0 6px;
      font-size: var(--text-md);
      color: var(--color-text-strong);
    }
    h3 {
      margin: 16px 0 6px;
      font-size: var(--text-sm);
      font-weight: 600;
      color: var(--color-text-muted);
    }
    p {
      margin: 0;
      line-height: 1.5;
    }
    .name {
      font-weight: 600;
      color: var(--color-text-strong);
    }
    .version,
    .ref,
    .muted {
      font-weight: 400;
      color: var(--color-text-muted);
    }
    .description {
      margin-top: 4px;
      color: var(--color-text-muted);
    }
    .origin {
      display: grid;
      grid-template-columns: auto 1fr;
      gap: 4px 12px;
      margin: 14px 0 0;
      font-size: var(--text-sm);
    }
    dt {
      color: var(--color-text-muted);
    }
    dd {
      margin: 0;
      min-width: 0;
      overflow-wrap: anywhere;
    }
    .mono {
      font-family: var(--font-mono);
    }
    ul {
      margin: 0;
      padding: 0;
      list-style: none;
      display: flex;
      flex-direction: column;
      gap: 4px;
      font-size: var(--text-sm);
    }
    .permissions li {
      display: flex;
      align-items: center;
      gap: 8px;
      color: var(--color-warning, var(--color-text));
    }
    .contributions li::before {
      content: '· ';
      color: var(--color-text-muted);
    }
    .warning {
      margin-top: 16px;
      font-size: var(--text-sm);
      color: var(--color-text-subtle);
    }
    footer {
      display: flex;
      justify-content: flex-end;
      gap: 8px;
      padding: 12px 22px 18px;
    }
  `,
})
export class PluginApprovalDialog {
  readonly review = input<PluginReview | null>(null);
  readonly busy = input(false);
  readonly confirmed = output<void>();
  readonly cancelled = output<void>();

  private readonly dialog = viewChild.required<ElementRef<HTMLDialogElement>>('dialog');

  protected readonly title = computed(() => TITLES[this.review()?.kind ?? 'install']);
  protected readonly confirmLabel = computed(
    () => CONFIRM_LABELS[this.review()?.kind ?? 'install'],
  );
  protected readonly permissions = computed(
    () => this.review()?.preview.manifest.permissions ?? [],
  );
  protected readonly contributions = computed(() => {
    const review = this.review();
    return review ? contributionSummary(review.preview.manifest) : [];
  });
  protected readonly short = shortCommit;
  protected readonly describe = describePermission;

  constructor() {
    effect(() => {
      const dialog = this.dialog().nativeElement;
      if (this.review() && !dialog.open) {
        dialog.showModal();
      } else if (!this.review() && dialog.open) {
        dialog.close();
      }
    });
  }
}
