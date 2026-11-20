import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../api/ai.js', () => ({
  ai: {
    listSessions: vi.fn(),
    listAvailableStandardAgents: vi.fn(),
    getGeneralSession: vi.fn(),
    getSessionMessages: vi.fn(),
    chat: vi.fn(),
  },
  llmProviders: { getEnabled: vi.fn() },
}));

vi.mock('./agentRuns.svelte.js', () => ({
  agentRuns: { emit: vi.fn() },
}));

const sessionFixture = [
  { id: 9, session_type: 'general' },
  { id: 1, session_type: 'standard', workspace_id: 1 },
  { id: 2, session_type: 'standard', workspace_id: 2 },
  { id: 3, session_type: 'standard', workspace_id: 1, archived_at: '2026-11-01T00:00:00Z' },
];

let ai;
let chatStore;

beforeEach(async () => {
  vi.resetModules();
  ({ ai } = await import('../api/ai.js'));
  vi.mocked(ai.listSessions).mockReset().mockResolvedValue(sessionFixture);
  vi.mocked(ai.listAvailableStandardAgents).mockReset().mockResolvedValue([]);
  vi.mocked(ai.getGeneralSession).mockReset().mockResolvedValue({ id: 9 });
  vi.mocked(ai.getSessionMessages).mockReset().mockResolvedValue([]);
  vi.mocked(ai.chat).mockReset().mockResolvedValue({ answer: 'hello', session_id: 9 });
  ({ chatStore } = await import('./chatStore.svelte.js'));
});

describe('chatStore.resumeConversation', () => {
  const anyWorkspace = () => true;

  it('starts a fresh general chat on the first open', async () => {
    await chatStore.resumeConversation(0, false, anyWorkspace);
    expect(ai.getGeneralSession).toHaveBeenCalledTimes(1);
    expect(chatStore.sessionType).toBe('general');
    expect(chatStore.sessions.map((s) => s.id)).toEqual([1, 2]);
  });

  it('keeps the general chat messages when reopened', async () => {
    await chatStore.resumeConversation(0, false, anyWorkspace);
    await chatStore.sendMessage('hi');
    await chatStore.resumeConversation(0, false, anyWorkspace);
    expect(ai.getGeneralSession).toHaveBeenCalledTimes(1);
    expect(chatStore.messages.map((m) => m.content)).toEqual(['hi', 'hello']);
  });

  it('keeps the selected agent chat when reopened', async () => {
    await chatStore.resumeConversation(0, false, anyWorkspace);
    await chatStore.selectConversation('session:1', 0);
    await chatStore.resumeConversation(0, false, anyWorkspace);
    expect(chatStore.sessionType).toBe('standard');
    expect(chatStore.sessionId).toBe(1);
    expect(ai.getGeneralSession).toHaveBeenCalledTimes(1);
  });

  it('keeps an agent chat when reopened inside its own workspace', async () => {
    await chatStore.resumeConversation(0, false, anyWorkspace);
    await chatStore.selectConversation('session:1', 0);
    await chatStore.resumeConversation(1, true, anyWorkspace);
    expect(chatStore.sessionId).toBe(1);
    expect(chatStore.sessionType).toBe('standard');
  });

  it('falls back to general when reopened in another workspace', async () => {
    await chatStore.resumeConversation(0, false, anyWorkspace);
    await chatStore.selectConversation('session:2', 0);
    await chatStore.resumeConversation(1, true, anyWorkspace);
    expect(chatStore.sessionType).toBe('general');
    expect(chatStore.sessionId).toBe(9);
  });

  it('falls back to general when the user can no longer chat in that workspace', async () => {
    await chatStore.resumeConversation(0, false, anyWorkspace);
    await chatStore.selectConversation('session:1', 0);
    await chatStore.resumeConversation(0, false, (id) => id !== 1);
    expect(chatStore.sessionType).toBe('general');
  });
});

describe('chatStore.prepareWorkspaceOptions', () => {
  it('lists standard sessions from every workspace when no workspace is in context', async () => {
    await chatStore.prepareWorkspaceOptions(0, false);
    expect(chatStore.sessions.map((s) => s.id)).toEqual([1, 2]);
    expect(chatStore.availableAgents).toEqual([]);
    expect(ai.listAvailableStandardAgents).not.toHaveBeenCalled();
  });

  it('scopes sessions and agents to the current workspace', async () => {
    vi.mocked(ai.listAvailableStandardAgents).mockResolvedValue([{ id: 1, name: 'Guide Bot' }]);
    await chatStore.prepareWorkspaceOptions(1, true);
    expect(chatStore.sessions.map((s) => s.id)).toEqual([1]);
    expect(chatStore.availableAgents).toEqual([{ id: 1, name: 'Guide Bot' }]);
  });

  it('reloads the cross-workspace list after visiting a workspace', async () => {
    await chatStore.prepareWorkspaceOptions(0, false);
    await chatStore.prepareWorkspaceOptions(1, true);
    await chatStore.prepareWorkspaceOptions(0, false);
    expect(ai.listSessions).toHaveBeenCalledTimes(3);
    expect(chatStore.sessions.map((s) => s.id)).toEqual([1, 2]);
  });

  it('retries the session list on the next open after a failed load', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(ai.listSessions).mockRejectedValueOnce(new Error('network'));
    await chatStore.prepareWorkspaceOptions(0, false);
    expect(chatStore.sessions).toEqual([]);
    await chatStore.prepareWorkspaceOptions(0, false);
    expect(ai.listSessions).toHaveBeenCalledTimes(2);
    expect(chatStore.sessions.map((s) => s.id)).toEqual([1, 2]);
  });

  it('keeps sessions empty in a workspace the user cannot edit', async () => {
    await chatStore.prepareWorkspaceOptions(1, false);
    expect(chatStore.sessions).toEqual([]);
    expect(ai.listSessions).not.toHaveBeenCalled();
  });
});
