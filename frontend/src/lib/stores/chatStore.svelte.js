import { api } from '../api.js';

let open = $state(false);
let messages = $state([]);
let loading = $state(false);
let error = $state('');
let connectionId = $state(0);
let connections = $state([]);
let connectionsLoaded = $state(false);
let itemKeyMap = $state({});

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
}

function show() {
  open = true;
  if (!connectionsLoaded) {
    loadConnections();
  }
}

function hide() {
  open = false;
}

async function sendMessage(text) {
  if (!text.trim() || loading) return;

  const userMsg = { role: 'user', content: text };
  messages = [...messages, userMsg];
  loading = true;
  error = '';

  try {
    const history = messages
      .filter((m) => !m.error)
      .map((m) => ({ role: m.role, content: m.content }));
    const result = await api.ai.chat(text, connectionId || undefined, history);
    const assistantMsg = {
      role: 'assistant',
      content: result.answer || '',
      toolCalls: result.tool_calls || [],
      iterations: result.iterations || 0,
    };
    messages = [...messages, assistantMsg];
    extractItemKeys(assistantMsg.toolCalls);
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
  if (!lastMsg || !lastMsg.error) return;
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
  toggle,
  show,
  hide,
  sendMessage,
  retryLastMessage,
  clearHistory,
  loadConnections,
};
