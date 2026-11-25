import { NgTemplateOutlet } from '@angular/common';
import { Component, computed, input } from '@angular/core';
import { Browser } from '@wailsio/runtime';
import { parseMarkdown } from './markdown';

/**
 * Renders Markdown through bindings: every block and inline format becomes an
 * element, raw HTML stays text, and links open in the system browser.
 */
@Component({
  selector: 'app-markdown-view',
  imports: [NgTemplateOutlet],
  template: `
    <ng-template #inline let-segments>
      @for (segment of segments; track $index) {
        @switch (segment.kind) {
          @case ('code') {
            <code>{{ segment.text }}</code>
          }
          @case ('strong') {
            <strong>{{ segment.text }}</strong>
          }
          @case ('emphasis') {
            <em>{{ segment.text }}</em>
          }
          @case ('link') {
            <a
              [href]="segment.href"
              [title]="segment.href"
              (click)="openLink($event, segment.href)"
              >{{ segment.text }}</a
            >
          }
          @default {
            {{ segment.text }}
          }
        }
      }
    </ng-template>

    @for (block of blocks(); track $index) {
      @switch (block.kind) {
        @case ('heading') {
          <div
            class="heading"
            [class]="'level-' + block.level"
            role="heading"
            [attr.aria-level]="block.level"
          >
            <ng-container *ngTemplateOutlet="inline; context: { $implicit: block.segments }" />
          </div>
        }
        @case ('paragraph') {
          <p><ng-container *ngTemplateOutlet="inline; context: { $implicit: block.segments }" /></p>
        }
        @case ('list') {
          @if (block.ordered) {
            <ol [attr.start]="block.start">
              @for (item of block.items; track $index) {
                <li><ng-container *ngTemplateOutlet="inline; context: { $implicit: item }" /></li>
              }
            </ol>
          } @else {
            <ul>
              @for (item of block.items; track $index) {
                <li><ng-container *ngTemplateOutlet="inline; context: { $implicit: item }" /></li>
              }
            </ul>
          }
        }
        @case ('code') {
          <pre><code>{{ block.text }}</code></pre>
        }
        @case ('quote') {
          <blockquote>
            @for (paragraph of block.paragraphs; track $index) {
              <p><ng-container *ngTemplateOutlet="inline; context: { $implicit: paragraph }" /></p>
            }
          </blockquote>
        }
        @case ('rule') {
          <hr />
        }
        @case ('table') {
          <div class="table-scroll">
            <table>
              <thead>
                <tr>
                  @for (cell of block.header; track $index) {
                    <th [style.text-align]="block.alignments[$index]">
                      <ng-container *ngTemplateOutlet="inline; context: { $implicit: cell }" />
                    </th>
                  }
                </tr>
              </thead>
              <tbody>
                @for (row of block.rows; track $index) {
                  <tr>
                    @for (cell of row; track $index) {
                      <td [style.text-align]="block.alignments[$index]">
                        <ng-container *ngTemplateOutlet="inline; context: { $implicit: cell }" />
                      </td>
                    }
                  </tr>
                }
              </tbody>
            </table>
          </div>
        }
      }
    }
  `,
  styleUrl: './markdown-view.css',
})
export class MarkdownView {
  readonly source = input.required<string>();

  protected readonly blocks = computed(() => parseMarkdown(this.source()));

  protected openLink(event: Event, href: string): void {
    event.preventDefault();
    event.stopPropagation();
    void Browser.OpenURL(href);
  }
}
