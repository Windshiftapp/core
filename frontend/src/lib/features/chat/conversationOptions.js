/**
 * Build the AI chat conversation picker options.
 *
 * Off workspace routes (workspaceId 0) the store hands over the user's agent
 * chats from every workspace, so each label names its workspace. Sessions are
 * kept only where the user may chat: the backend requires item edit on the
 * session's workspace and would reject the turn otherwise. New-chat entries
 * come from `agents`, which the store fills only inside a workspace.
 *
 * @param {{
 *   sessions: Array<{ id: number, title?: string, agent_profile_id?: number, workspace_id: number }>,
 *   agents: Array<{ id: number, name?: string, handle?: string }>,
 *   workspaceId: number,
 *   canChatIn: (workspaceId: number) => boolean,
 *   workspaceName: (workspaceId: number) => string | undefined,
 * }} input
 * @returns {Array<{ value: string, label: string }>}
 */
export function buildConversationOptions({
  sessions,
  agents,
  workspaceId,
  canChatIn,
  workspaceName,
}) {
  const options = [{ value: 'general', label: 'General' }];
  for (const session of sessions) {
    if (!canChatIn(session.workspace_id)) continue;
    const agent = agents.find((candidate) => candidate.id === session.agent_profile_id);
    let label = session.title || agent?.name || 'Standard agent chat';
    if (!workspaceId) {
      const name = workspaceName(session.workspace_id);
      if (name) label = `${label} · ${name}`;
    }
    options.push({ value: `session:${session.id}`, label });
  }
  for (const agent of agents) {
    options.push({
      value: `new:${agent.id}`,
      label: `New chat · ${agent.name || `@${agent.handle}`}`,
    });
  }
  return options;
}
