import { Component, effect, inject, signal } from '@angular/core';
import * as MCPSettingsService from '@bindings/github.com/awake-client/awake/src/features/mcp/settingsservice';
import * as PlatformService from '@bindings/github.com/awake-client/awake/src/features/platform/service';
import { WorkspaceStore } from '@core/workspace-store';
import { Icon } from '@shared/icon';

/**
 * Shows how to give an external coding agent, such as Claude Code, access to
 * the requests of this workspace through the local MCP server.
 */
@Component({
  selector: 'app-mcp-setup',
  imports: [Icon],
  template: `
    <div class="mcp" [class.open]="isOpen() && enabled()">
      <div class="header">
        <app-icon name="terminal" [size]="14" />
        <span class="title">
          <strong>MCP server</strong>
          <small>
            {{
              enabled()
                ? 'Claude Code and other agents can use this workspace.'
                : 'Off: agents that start awake mcp for this workspace are refused.'
            }}
          </small>
        </span>
        @if (enabled()) {
          <button
            type="button"
            class="btn btn-sm btn-ghost setup"
            [attr.aria-expanded]="isOpen()"
            (click)="isOpen.set(!isOpen())"
          >
            Set up
            <app-icon class="chevron" name="chevron-down" [size]="13" />
          </button>
        }
        <button
          type="button"
          role="switch"
          class="switch"
          [class.on]="enabled()"
          [attr.aria-checked]="enabled()"
          [title]="
            enabled()
              ? 'Turn the MCP server off for this workspace'
              : 'Turn the MCP server on for this workspace'
          "
          aria-label="Allow agents to use this workspace through MCP"
          [disabled]="saving()"
          (click)="setEnabled(!enabled())"
        >
          <span class="knob"></span>
        </button>
      </div>

      @if (error(); as message) {
        <p class="hint error">{{ message }}</p>
      }

      @if (enabled() && isOpen()) {
        <div class="body">
          <p class="hint">
            Agents can list, read, send, create and update the requests of this workspace. Secrets
            and credentials never leave Awake. The server runs only when the agent starts it, over
            stdio, without opening a port.
          </p>

          <div class="access" role="radiogroup" aria-label="Access">
            <button
              type="button"
              role="radio"
              [class.selected]="!readOnly()"
              [attr.aria-checked]="!readOnly()"
              (click)="readOnly.set(false)"
            >
              Read and write
            </button>
            <button
              type="button"
              role="radio"
              [class.selected]="readOnly()"
              [attr.aria-checked]="readOnly()"
              (click)="readOnly.set(true)"
            >
              Read and send only
            </button>
          </div>

          @if (command(); as text) {
            <div class="command">
              <code>{{ text }}</code>
              <button type="button" class="btn btn-sm" (click)="copy(text)">
                <app-icon [name]="copied() ? 'check' : 'copy'" [size]="13" />
                {{ copied() ? 'Copied' : 'Copy' }}
              </button>
            </div>
            <p class="hint">Run it once in a terminal, then restart Claude Code.</p>
          }
          <p class="hint">
            Saved on this computer only. Turning the switch off stops a running agent at its next
            call.
          </p>
        </div>
      }
    </div>
  `,
  styles: `
    .mcp {
      border: 1px solid var(--color-border);
      border-radius: var(--radius-md);
      background: var(--color-surface);
      font-size: var(--text-sm);
    }
    .header {
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 9px 11px;
      color: var(--color-text-muted);
    }
    .title {
      display: flex;
      flex: 1;
      flex-direction: column;
      gap: 1px;
      min-width: 0;
    }
    .title strong {
      color: var(--color-text-strong);
      font-weight: 600;
    }
    .title small {
      color: var(--color-text-subtle);
    }
    .setup {
      display: inline-flex;
      align-items: center;
      gap: 4px;
    }
    .chevron {
      transition: transform 120ms ease;
    }
    .mcp.open .chevron {
      transform: rotate(180deg);
    }
    .mcp.open .header {
      border-bottom: 1px solid var(--color-border);
    }
    .body {
      padding-bottom: 2px;
    }
    .hint {
      margin: 10px 12px;
      line-height: 1.5;
      color: var(--color-text-muted);
    }
    .switch {
      position: relative;
      flex: 0 0 auto;
      width: 34px;
      height: 20px;
      padding: 0;
      border: none;
      border-radius: 999px;
      background: var(--color-border-strong);
      cursor: pointer;
      transition: background 120ms ease;
    }
    .switch.on {
      background: var(--color-accent);
    }
    .switch:disabled {
      opacity: 0.6;
      cursor: default;
    }
    .knob {
      position: absolute;
      top: 2px;
      left: 2px;
      width: 16px;
      height: 16px;
      border-radius: 50%;
      background: #fff;
      box-shadow: 0 1px 2px rgba(0, 0, 0, 0.2);
      transition: transform 120ms ease;
    }
    .switch.on .knob {
      transform: translateX(14px);
    }
    .hint code {
      font-family: var(--font-mono);
    }
    .hint.error {
      color: var(--color-danger);
    }
    .access {
      display: inline-flex;
      gap: 2px;
      margin: 0 12px;
      padding: 3px;
      border-radius: var(--radius-lg);
      background: var(--color-active);
    }
    .access button {
      height: 24px;
      padding: 0 10px;
      border: none;
      border-radius: var(--radius-md);
      background: transparent;
      color: var(--color-text-muted);
      font-size: var(--text-sm);
      font-weight: 500;
      cursor: pointer;
    }
    .access button.selected {
      background: var(--color-surface-raised);
      color: var(--color-text-strong);
      box-shadow: var(--shadow-card);
    }
    .command {
      display: flex;
      align-items: flex-start;
      gap: 8px;
      margin: 10px 12px 0;
      padding: 8px 8px 8px 10px;
      border: 1px solid var(--color-border);
      border-radius: var(--radius-sm);
      background: var(--color-background);
    }
    .command code {
      flex: 1;
      min-width: 0;
      font-family: var(--font-mono);
      line-height: 1.5;
      color: var(--color-text-strong);
      overflow-wrap: anywhere;
      user-select: text;
    }
  `,
})
export class McpSetup {
  private readonly workspaceStore = inject(WorkspaceStore);

  protected readonly isOpen = signal(false);
  protected readonly readOnly = signal(false);
  protected readonly command = signal('');
  protected readonly error = signal<string | null>(null);
  protected readonly copied = signal(false);
  protected readonly enabled = signal(true);
  protected readonly saving = signal(false);
  private copiedTimer: ReturnType<typeof setTimeout> | undefined;

  constructor() {
    effect(() => {
      const root = this.workspaceStore.rootPath();
      if (root) {
        MCPSettingsService.Enabled(root)
          .then((enabled) => this.enabled.set(enabled))
          .catch(() => this.enabled.set(true));
      }
    });

    effect(() => {
      const root = this.workspaceStore.rootPath();
      const readOnly = this.readOnly();
      if (!root) {
        this.command.set('');
        return;
      }
      PlatformService.MCPCommand(root, readOnly)
        .then((command) => {
          this.command.set(command);
          this.error.set(null);
        })
        .catch((error: unknown) =>
          this.error.set(error instanceof Error ? error.message : String(error)),
        );
    });
  }

  protected async setEnabled(enabled: boolean): Promise<void> {
    const root = this.workspaceStore.rootPath();
    if (!root || this.saving()) {
      return;
    }
    this.saving.set(true);
    try {
      await MCPSettingsService.SetEnabled(root, enabled);
      this.enabled.set(enabled);
      this.error.set(null);
    } catch (error) {
      this.error.set(error instanceof Error ? error.message : String(error));
    } finally {
      this.saving.set(false);
    }
  }

  protected async copy(text: string): Promise<void> {
    await navigator.clipboard.writeText(text);
    this.copied.set(true);
    clearTimeout(this.copiedTimer);
    this.copiedTimer = setTimeout(() => this.copied.set(false), 1500);
  }
}
