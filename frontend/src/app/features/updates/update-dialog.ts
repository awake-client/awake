import { Component, ElementRef, computed, effect, inject, viewChild } from '@angular/core';
import { Browser } from '@wailsio/runtime';
import { TabsStore } from '@core/tabs-store';
import { UpdateStore } from '@core/update-store';
import { Icon, IconName } from '@shared/icon';
import { InlineSegment, formatBytes, parseInlineMarkdown } from '@shared/inline-markdown';
import { formatVersion } from '@shared/version-label';

const PERCENT = 100;

/**
 * Asks before installing an update, shows what changed since the running
 * version, follows the download and offers the restart.
 */
@Component({
  selector: 'app-update-dialog',
  imports: [Icon],
  templateUrl: './update-dialog.html',
  styleUrl: './update-dialog.css',
})
export class UpdateDialog {
  protected readonly updates = inject(UpdateStore);
  private readonly tabs = inject(TabsStore);
  private readonly dialog = viewChild.required<ElementRef<HTMLDialogElement>>('dialog');

  protected readonly currentLabel = computed(() => formatVersion(this.updates.currentVersion()));
  protected readonly releaseLabel = computed(() => formatVersion(this.updates.release()?.version));
  protected readonly publishedOn = computed(() => {
    const published = this.updates.release()?.publishedAt;
    return published
      ? new Date(published).toLocaleDateString(undefined, { dateStyle: 'medium' })
      : '';
  });
  protected readonly downloadSize = computed(() => {
    const size = this.updates.release()?.size ?? 0;
    return size > 0 ? formatBytes(size) : '';
  });
  protected readonly changes = computed(() => this.updates.release()?.changes ?? []);
  protected readonly showsChanges = computed(() => {
    const phase = this.updates.phase();
    return (
      this.changes().length > 0 &&
      phase !== 'checking' &&
      phase !== 'up-to-date' &&
      phase !== 'check-failed'
    );
  });
  protected readonly percent = computed(() => {
    const progress = this.updates.progress();
    if (!progress || progress.total <= 0) {
      return null;
    }
    return Math.min(PERCENT, Math.round((progress.written / progress.total) * PERCENT));
  });
  protected readonly progressLabel = computed(() => {
    const progress = this.updates.progress();
    if (!progress || progress.total <= 0) {
      return '';
    }
    return `${formatBytes(progress.written)} of ${formatBytes(progress.total)}`;
  });
  protected readonly unsavedCount = computed(
    () => this.tabs.tabs().filter((tab) => tab.dirty).length,
  );
  protected readonly icon = computed<IconName>(() => {
    switch (this.updates.phase()) {
      case 'up-to-date':
      case 'ready':
        return 'check';
      case 'check-failed':
      case 'install-failed':
        return 'alert';
      case 'checking':
        return 'refresh';
      default:
        return 'arrow-up-circle';
    }
  });

  constructor() {
    effect(() => {
      const dialog = this.dialog().nativeElement;
      if (this.updates.dialogOpen()) {
        if (!dialog.open) {
          dialog.showModal();
        }
      } else if (dialog.open) {
        dialog.close();
      }
    });
  }

  protected segments(text: string): InlineSegment[] {
    return parseInlineMarkdown(text);
  }

  protected openLink(event: Event, href: string): void {
    event.preventDefault();
    void Browser.OpenURL(href);
  }

  protected closeOnBackdrop(event: MouseEvent): void {
    if (event.target === this.dialog().nativeElement) {
      this.updates.closeDialog();
    }
  }
}
