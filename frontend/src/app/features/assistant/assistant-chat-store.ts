import { Injectable, signal } from '@angular/core';

export type ChatRole = 'user' | 'assistant';

/** One message shown in the assistant conversation. */
export interface ChatEntry {
  role: ChatRole;
  content: string;
  /** Label of the agent that wrote an assistant message. */
  agent?: string;
}

/**
 * Keeps the assistant conversation in memory so leaving the page to review a
 * change in Git does not lose it. The conversation is scoped to a workspace.
 */
@Injectable({ providedIn: 'root' })
export class AssistantChatStore {
  private workspaceRoot = '';
  private readonly entries = signal<ChatEntry[]>([]);
  private readonly selectedAgent = signal('claude');
  private readonly selectedModel = signal('');

  readonly messages = this.entries.asReadonly();
  readonly agent = this.selectedAgent.asReadonly();
  /** OpenCode model chosen by the user; empty keeps the configured one. */
  readonly model = this.selectedModel.asReadonly();

  /** Starts a new conversation when another workspace is opened. */
  useWorkspace(workspaceRoot: string): void {
    if (workspaceRoot !== this.workspaceRoot) {
      this.workspaceRoot = workspaceRoot;
      this.entries.set([]);
    }
  }

  selectAgent(agent: string): void {
    this.selectedAgent.set(agent);
  }

  selectModel(model: string): void {
    this.selectedModel.set(model);
  }

  add(entry: ChatEntry): void {
    this.entries.update((entries) => [...entries, entry]);
  }

  clear(): void {
    this.entries.set([]);
  }
}
