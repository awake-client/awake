import { Component, ElementRef, computed, effect, inject, signal, viewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import * as AssistantService from '@bindings/github.com/awake-client/awake/src/features/assistant/service';
import {
  AgentStatus,
  ChatMessage,
} from '@bindings/github.com/awake-client/awake/src/features/assistant/models';
import { notifyCollectionsChanged } from '@core/collection-events';
import { WorkspaceStore } from '@core/workspace-store';
import { Icon, IconName } from '@shared/icon';
import { AssistantChatStore } from './assistant-chat-store';

const SUGGESTIONS = [
  'Rename the unclear requests in this workspace.',
  'Extract localhost URLs into a development environment.',
  'Review my collections and suggest useful environments.',
];

const AGENT_ICONS: Record<string, IconName> = {
  claude: 'sparkles',
  opencode: 'terminal',
};

/**
 * A simple chat over the workspace. Each message is sent to the selected
 * command line agent, which can read and edit the YAML files directly.
 */
@Component({
  selector: 'app-assistant-panel',
  imports: [FormsModule, Icon],
  templateUrl: './assistant-panel.html',
  styleUrl: './assistant-panel.css',
})
export class AssistantPanel {
  private readonly workspaceStore = inject(WorkspaceStore);
  private readonly chat = inject(AssistantChatStore);
  private readonly messageList = viewChild<ElementRef<HTMLElement>>('messageList');

  protected readonly agents = signal<AgentStatus[]>([]);
  protected readonly isLoadingAgents = signal(true);
  protected readonly models = signal<string[]>([]);
  protected readonly isLoadingModels = signal(false);
  private modelsRequested = false;
  protected readonly isSending = signal(false);
  protected readonly draft = signal('');
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly createdAgentFile = signal(false);
  protected readonly suggestions = SUGGESTIONS;

  protected readonly messages = this.chat.messages;
  protected readonly agent = this.chat.agent;
  protected readonly model = this.chat.model;
  protected readonly workspaceRoot = computed(() => this.workspaceStore.rootPath() ?? '');
  protected readonly selectedAgent = computed(() =>
    this.agents().find((agent) => agent.Name === this.agent()),
  );
  protected readonly hasInstalledAgent = computed(() =>
    this.agents().some((agent) => agent.Installed),
  );
  protected readonly canSend = computed(
    () =>
      this.selectedAgent()?.Installed === true &&
      !this.isSending() &&
      this.draft().trim().length > 0,
  );

  constructor() {
    const rootPath = this.workspaceStore.rootPath();
    if (rootPath) {
      this.chat.useWorkspace(rootPath);
    }

    void this.loadAgents();

    // The conversation belongs to the open workspace.
    effect(() => {
      const currentRoot = this.workspaceStore.rootPath();
      if (currentRoot) {
        this.chat.useWorkspace(currentRoot);
      }
    });

    effect(() => {
      this.messages();
      this.isSending();
      requestAnimationFrame(() => this.scrollToBottom());
    });

    // OpenCode needs a model that supports tool use; offer the ones it knows.
    effect(() => {
      if (this.agent() === 'opencode' && !this.modelsRequested) {
        this.modelsRequested = true;
        void this.loadModels();
      }
    });
  }

  protected selectAgent(name: string): void {
    this.chat.selectAgent(name);
  }

  protected selectModel(model: string): void {
    this.chat.selectModel(model);
  }

  protected agentIcon(name: string): IconName {
    return AGENT_ICONS[name] ?? 'terminal';
  }

  protected agentLabel(name: string): string {
    return this.agents().find((agent) => agent.Name === name)?.Label ?? name;
  }

  protected useSuggestion(suggestion: string): void {
    this.draft.set(suggestion);
  }

  protected clear(): void {
    this.chat.clear();
    this.errorMessage.set(null);
    this.createdAgentFile.set(false);
  }

  protected handleKeydown(event: KeyboardEvent): void {
    if (event.key !== 'Enter' || event.shiftKey || event.isComposing) {
      return;
    }
    event.preventDefault();
    void this.send();
  }

  protected async send(): Promise<void> {
    const rootPath = this.workspaceStore.rootPath();
    const message = this.draft().trim();
    if (!rootPath || !message || !this.canSend()) {
      return;
    }

    const agent = this.agent();
    const history: ChatMessage[] = this.messages().map((entry) => ({
      Role: entry.role,
      Content: entry.content,
    }));

    this.isSending.set(true);
    this.errorMessage.set(null);
    this.draft.set('');
    this.chat.add({ role: 'user', content: message });

    try {
      const reply = await AssistantService.Chat(
        rootPath,
        agent,
        agent === 'opencode' ? this.model() : '',
        message,
        history,
      );
      this.chat.add({
        role: 'assistant',
        content: reply.Content,
        agent: this.agentLabel(reply.Agent),
      });
      if (reply.CreatedAgentFile) {
        this.createdAgentFile.set(true);
      }
    } catch (error) {
      this.errorMessage.set(describeError(error));
    } finally {
      this.isSending.set(false);
      // The agent edits files directly; refresh every view that reads them,
      // even after a failure that may have applied part of the changes.
      notifyCollectionsChanged();
      window.dispatchEvent(new Event('awake:environments-changed'));
    }
  }

  private async loadAgents(): Promise<void> {
    this.isLoadingAgents.set(true);
    try {
      const agents = (await AssistantService.Agents()) ?? [];
      this.agents.set(agents);

      const current = agents.find((agent) => agent.Name === this.agent());
      if (!current?.Installed) {
        const installed = agents.find((agent) => agent.Installed);
        if (installed) {
          this.chat.selectAgent(installed.Name);
        }
      }
    } catch (error) {
      this.agents.set([]);
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoadingAgents.set(false);
    }
  }

  private async loadModels(): Promise<void> {
    this.isLoadingModels.set(true);
    try {
      this.models.set((await AssistantService.OpenCodeModels()) ?? []);
    } catch (error) {
      this.models.set([]);
      this.errorMessage.set(describeError(error));
    } finally {
      this.isLoadingModels.set(false);
    }
  }

  private scrollToBottom(): void {
    const element = this.messageList()?.nativeElement;
    if (element) {
      element.scrollTop = element.scrollHeight;
    }
  }
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
