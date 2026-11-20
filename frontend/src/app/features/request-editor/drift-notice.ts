import { Component, computed, input, output, signal } from '@angular/core';
import { Icon } from '@shared/icon';
import { DriftIssue, MAXIMUM_DRIFT_ISSUES } from './override-drift';

/** Issues shown before the list folds behind "Show N more". */
const FOLDED_ISSUES = 4;

/**
 * Warns that a local override no longer matches the shared version, and lists
 * where: keys or parameters gone, added or holding another kind of value.
 */
@Component({
  selector: 'app-drift-notice',
  imports: [Icon],
  template: `
    <div class="drift" role="status">
      <div class="drift-header">
        <app-icon name="alert" [size]="14" />
        <span class="drift-title">{{ title() }}</span>
        <span class="drift-spacer"></span>
        <button type="button" class="drift-action" (click)="compare.emit()">
          Show shared version
        </button>
      </div>
      <ul>
        @for (issue of shownIssues(); track $index) {
          <li [attr.data-kind]="issue.kind">
            @if (issue.path) {
              <code>{{ issue.path }}</code>
            }
            <span>{{ issue.message }}</span>
          </li>
        }
      </ul>
      @if (hiddenCount() > 0) {
        <button type="button" class="drift-more" (click)="expanded.set(true)">
          Show {{ hiddenCount() }} more
        </button>
      }
    </div>
  `,
  styles: `
    :host {
      display: block;
      flex: 0 0 auto;
    }
    .drift {
      margin-bottom: 12px;
      padding: 8px 12px 10px;
      border: 1px solid color-mix(in srgb, var(--color-warning) 35%, transparent);
      border-radius: var(--radius-md);
      background: color-mix(in srgb, var(--color-warning) 8%, transparent);
      font-size: var(--text-sm);
    }
    .drift-header {
      display: flex;
      align-items: center;
      gap: 8px;
      color: var(--color-warning);
    }
    .drift-title {
      font-weight: 600;
      color: var(--color-text-strong);
    }
    .drift-spacer {
      flex: 1;
    }
    .drift-action,
    .drift-more {
      padding: 0;
      border: none;
      background: transparent;
      color: var(--color-accent);
      font-size: var(--text-sm);
      cursor: pointer;
    }
    .drift-action:hover,
    .drift-more:hover {
      text-decoration: underline;
    }
    ul {
      margin: 6px 0 0;
      padding: 0 0 0 22px;
      list-style: none;
    }
    li {
      display: flex;
      flex-wrap: wrap;
      align-items: baseline;
      gap: 6px;
      padding: 2px 0;
      color: var(--color-text-muted);
      overflow-wrap: anywhere;
    }
    code {
      padding: 0 4px;
      border-radius: var(--radius-xs);
      background: var(--color-active);
      color: var(--color-text-strong);
      font-family: var(--font-mono);
      font-size: var(--text-xs);
    }
    .drift-more {
      margin: 4px 0 0 22px;
    }
  `,
})
export class DriftNotice {
  readonly issues = input.required<readonly DriftIssue[]>();
  /** What drifted, as the title names it: "parameters" or "body". */
  readonly subject = input.required<string>();
  /** Asks to show the shared version, to compare with the override. */
  readonly compare = output<void>();

  protected readonly expanded = signal(false);

  protected readonly title = computed(() => {
    const count = this.issues().length;
    const capped = count >= MAXIMUM_DRIFT_ISSUES ? `${count}+` : `${count}`;
    const differences = count === 1 ? 'difference' : 'differences';
    return `Out of sync with the shared ${this.subject()}: ${capped} ${differences}`;
  });
  protected readonly shownIssues = computed(() =>
    this.expanded() ? this.issues() : this.issues().slice(0, FOLDED_ISSUES),
  );
  protected readonly hiddenCount = computed(() =>
    Math.max(0, this.issues().length - this.shownIssues().length),
  );
}
