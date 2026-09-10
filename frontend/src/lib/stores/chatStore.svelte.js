import { api } from '../api.js';
import { agentRuns } from './agentRuns.svelte.js';

let open = $state(false);
let messages = $state([]);
let loading = $state(false);
let error = $state('');
let connectionId = $state(0);
let connections = $state([]);
let connectionsLoaded = $state(false);
let itemKeyMap = $state({});
let sessionId = $state(0);
let sessionType = $state('general');
let sessionWorkspaceId = $state(0);
let agentProfileId = $state(0);
let sessions = $state([]);
let availableAgents = $state([]);
let conversationLoading = $state(false);
let workspaceOptionsKey = '';
let historyLoaded = $state(false);
let historyPromise = null;
let historyAfterMessageId = $state(0);

function messageID(messagesToInspect) {
  return messagesToInspect.reduce(
    (highest, message) => Math.max(highest, Number(message.id) || 0),
    0
  );
}

function applyGeneralSession(session, storedMessages, afterMessageId = 0) {
  sessionId = session?.id || 0;
  sessionType = 'general';
  sessionWorkspaceId = 0;
  agentProfileId = 0;
  messages = (Array.isArray(storedMessages) ? storedMessages : []).map((message) => ({
    id: message.id,
    role: message.role,
    content: message.content,
  }));
  historyAfterMessageId = afterMessageId;
}

async function loadGeneralHistory() {
  if (historyLoaded && sessionType === 'general') return;
  if (historyPromise) return historyPromise;
  historyPromise = (async () => {
    try {
      const session = await api.ai.getGeneralSession();
      let stored = [];
      if (session?.id) {
        stored = await api.ai.getSessionMessages(session.id);
      }
      applyGeneralSession(session, stored);
      historyLoaded = true;
    } catch (err) {
      console.error('Failed to load agent conversation:', err);
    } finally {
      historyPromise = null;
    }
  })();
  return historyPromise;
}

async function startNewGeneralChat() {
  if (conversationLoading) return;
  conversationLoading = true;
  try {
    const session = await api.ai.getGeneralSession();
    const stored = session?.id ? await api.ai.getSessionMessages(session.id) : [];
    applyGeneralSession(session, stored, messageID(Array.isArray(stored) ? stored : []));
    messages = [];
    error = '';
    itemKeyMap = {};
    historyLoaded = true;
  } catch (err) {
    error = err.message || 'Conversation could not be started';
  } finally {
    conversationLoading = false;
  }
}

async function loadSession(session) {
  if (!session?.id) return;
  conversationLoading = true;
  historyLoaded = false;
  try {
    const stored = await api.ai.getSessionMessages(session.id);
    sessionId = session.id;
    sessionType = session.session_type || 'standard';
    sessionWorkspaceId = session.workspace_id || 0;
    agentProfileId = session.agent_profile_id || 0;
    historyAfterMessageId = 0;
    messages = (Array.isArray(stored) ? stored : []).map((message) => ({
      id: message.id,
      role: message.role,
      content: message.content,
    }));
    error = '';
    itemKeyMap = {};
    historyLoaded = true;
  } catch (err) {
    error = err.message || 'Conversation could not be loaded';
  } finally {
    conversationLoading = false;
  }
}

async function prepareWorkspaceOptions(workspaceId, allowStandard) {
  const normalizedWorkspaceId = Number(workspaceId) || 0;
  const key = allowStandard && normalizedWorkspaceId ? String(normalizedWorkspaceId) : 'general';
  if (workspaceOptionsKey === key) return;
  workspaceOptionsKey = key;

  if (!allowStandard || !normalizedWorkspaceId) {
    sessions = [];
    availableAgents = [];
    if (sessionType === 'standard') {
      await loadGeneralHistory();
    }
    return;
  }

  try {
    const [sessionResult, agentResult] = await Promise.all([
      api.ai.listSessions(),
      api.ai.listAvailableStandardAgents(normalizedWorkspaceId),
    ]);
    sessions = (Array.isArray(sessionResult) ? sessionResult : []).filter(
      (session) =>
        session.session_type === 'standard' &&
        Number(session.workspace_id) === normalizedWorkspaceId &&
        !session.archived_at
    );
    availableAgents = Array.isArray(agentResult) ? agentResult : [];
  } catch (err) {
    console.error('Failed to load workspace agent conversations:', err);
    sessions = [];
    availableAgents = [];
  }
}

async function selectConversation(value, workspaceId) {
  if (value === 'general') {
    historyLoaded = false;
    return loadGeneralHistory();
  }
  if (String(value).startsWith('session:')) {
    const id = Number(String(value).slice('session:'.length));
    const session = sessions.find((candidate) => candidate.id === id);
    if (session) return loadSession(session);
    return;
  }
  if (String(value).startsWith('new:')) {
    const profileId = Number(String(value).slice('new:'.length));
    return startStandardConversation(workspaceId, profileId);
  }
}

async function startStandardConversation(workspaceId, profileId = agentProfileId) {
  const normalizedWorkspaceId = Number(workspaceId) || 0;
  const normalizedProfileId = Number(profileId) || 0;
  if (!normalizedWorkspaceId || !normalizedProfileId || conversationLoading) return;
  conversationLoading = true;
  try {
    const agent = availableAgents.find((candidate) => candidate.id === normalizedProfileId);
    const session = await api.ai.createStandardSession(normalizedWorkspaceId, {
      agent_profile_id: normalizedProfileId,
      ...(agent?.name ? { title: agent.name } : {}),
    });
    sessions = [session, ...sessions.filter((candidate) => candidate.id !== session.id)];
    await loadSession(session);
  } catch (err) {
    error = err.message || 'Conversation could not be created';
  } finally {
    conversationLoading = false;
  }
}

async function archiveCurrentSession() {
  if (sessionType !== 'standard' || !sessionId || conversationLoading) return;
  conversationLoading = true;
  const archivedID = sessionId;
  try {
    await api.ai.archiveSession(archivedID);
    sessions = sessions.filter((session) => session.id !== archivedID);
    historyLoaded = false;
    await loadGeneralHistory();
  } catch (err) {
    error = err.message || 'Conversation could not be archived';
  } finally {
    conversationLoading = false;
  }
}

async function loadConnections() {
  if (connectionsLoaded) return;
  try {
    const result = await api.llmProviders.getEnabled();
    connections = Array.isArray(result) ? result : [];
    connectionsLoaded = true;
  } catch (err) {
    console.error('Failed to load LLM connections:', err);
  }
}

function toggle() {
  open = !open;
  if (open && !connectionsLoaded) {
    loadConnections();
  }
  if (open && !historyLoaded) {
    loadGeneralHistory();
  }
}

function show() {
  open = true;
  if (!connectionsLoaded) {
    loadConnections();
  }
  if (!historyLoaded) {
    loadGeneralHistory();
  }
}

function hide() {
  open = false;
}

async function sendMessage(text, context) {
  if (!text.trim() || loading) return;
  if (!historyLoaded) {
    await loadGeneralHistory();
  }

  const userMsg = { role: 'user', content: text };
  messages = [...messages, userMsg];
  loading = true;
  error = '';

  try {
    const result =
      sessionType === 'general'
        ? await api.ai.chat(
            text,
            connectionId || undefined,
            sessionId || undefined,
            context,
            historyAfterMessageId
          )
        : await api.ai.chat(text, undefined, sessionId || undefined, context);
    sessionId = result.session_id || sessionId;
    const assistantMsg = {
      id: result.message_id,
      role: 'assistant',
      content: result.answer || '',
      toolCalls: result.tool_calls || [],
      iterations: result.iterations || 0,
      maxIterations: result.max_iterations || 0,
      stopReason: result.stop_reason || '',
      needsReview: result.needs_review || false,
      reviewReasons: result.review_reasons || [],
    };
    messages = [...messages, assistantMsg];
    extractItemKeys(assistantMsg.toolCalls);
    agentRuns.emit();
  } catch (err) {
    error = err.message || 'Failed to get a response';
    const errorMsg = {
      role: 'assistant',
      content: '',
      error: error,
    };
    messages = [...messages, errorMsg];
  } finally {
    loading = false;
  }
}

function extractItemKeys(toolCalls) {
  if (!Array.isArray(toolCalls)) return;
  const newEntries = {};
  for (const tc of toolCalls) {
    if (!tc.result) continue;
    let parsed;
    try {
      parsed = JSON.parse(tc.result);
    } catch {
      continue;
    }
    // Collect items from list/search results or single-item detail
    const items = parsed.items || (parsed.key ? [parsed] : []);
    for (const item of items) {
      if (item.key && item.id && item.workspace_id) {
        newEntries[item.key] = { id: item.id, workspaceId: item.workspace_id };
      }
    }
  }
  if (Object.keys(newEntries).length > 0) {
    itemKeyMap = { ...itemKeyMap, ...newEntries };
  }
}

function retryLastMessage() {
  if (loading) return;
  // Remove the last assistant error message
  const lastMsg = messages[messages.length - 1];
  if (!lastMsg?.error) return;
  const withoutError = messages.slice(0, -1);
  // Find the last user message to re-send
  let userText = '';
  for (let i = withoutError.length - 1; i >= 0; i--) {
    if (withoutError[i].role === 'user') {
      userText = withoutError[i].content;
      break;
    }
  }
  if (!userText) return;
  // Remove both the error and the user message, then re-send
  messages = withoutError.slice(0, -1);
  sendMessage(userText);
}

function clearHistory() {
  if (sessionType === 'general') {
    historyAfterMessageId = Math.max(historyAfterMessageId, messageID(messages));
    historyLoaded = true;
  } else {
    historyLoaded = false;
  }
  messages = [];
  error = '';
  itemKeyMap = {};
}

export const chatStore = {
  get open() {
    return open;
  },
  get messages() {
    return messages;
  },
  get loading() {
    return loading;
  },
  get error() {
    return error;
  },
  get connectionId() {
    return connectionId;
  },
  set connectionId(val) {
    connectionId = val;
  },
  get connections() {
    return connections;
  },
  get itemKeyMap() {
    return itemKeyMap;
  },
  get sessionId() {
    return sessionId;
  },
  get sessionType() {
    return sessionType;
  },
  get sessionWorkspaceId() {
    return sessionWorkspaceId;
  },
  get agentProfileId() {
    return agentProfileId;
  },
  get sessions() {
    return sessions;
  },
  get availableAgents() {
    return availableAgents;
  },
  get conversationLoading() {
    return conversationLoading;
  },
  toggle,
  show,
  hide,
  sendMessage,
  retryLastMessage,
  clearHistory,
  loadConnections,
  loadGeneralHistory,
  startNewGeneralChat,
  prepareWorkspaceOptions,
  selectConversation,
  startStandardConversation,
  archiveCurrentSession,
};
