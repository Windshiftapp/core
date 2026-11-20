import { describe, expect, it } from 'vitest';
import { buildConversationOptions } from './conversationOptions.js';

const session = { id: 1, title: 'Guide Bot', agent_profile_id: 1, workspace_id: 1 };
const base = {
  sessions: [session],
  agents: [],
  workspaceId: 0,
  canChatIn: () => true,
  workspaceName: () => 'Alpha WS',
};

describe('buildConversationOptions', () => {
  it('adds existing agent chats with the workspace name off workspace routes', () => {
    expect(buildConversationOptions(base)).toEqual([
      { value: 'general', label: 'General' },
      { value: 'session:1', label: 'Guide Bot · Alpha WS' },
    ]);
  });

  it('omits the workspace name inside a workspace', () => {
    expect(buildConversationOptions({ ...base, workspaceId: 1 })[1].label).toBe('Guide Bot');
  });

  it('drops sessions in workspaces where the user cannot edit', () => {
    expect(buildConversationOptions({ ...base, canChatIn: () => false })).toEqual([
      { value: 'general', label: 'General' },
    ]);
  });

  it('falls back to the bare title when the workspace name is unknown', () => {
    expect(buildConversationOptions({ ...base, workspaceName: () => undefined })[1].label).toBe(
      'Guide Bot'
    );
  });

  it('adds new-chat entries for available agents', () => {
    const opts = buildConversationOptions({
      ...base,
      workspaceId: 1,
      agents: [{ id: 1, name: 'Guide Bot', handle: 'guidebot' }],
    });
    expect(opts.at(-1)).toEqual({ value: 'new:1', label: 'New chat · Guide Bot' });
  });
});
