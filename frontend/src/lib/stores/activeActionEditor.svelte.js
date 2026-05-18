// activeActionEditor exposes which action (if any) is currently open in the
// in-app editor. The chat panel reads it so the backend can append a
// surface-specific hint to the system prompt; the chatStore live-reload
// path reads it indirectly via the action editor's own subscription.
//
// The store is intentionally minimal — a single integer plus get/set —
// because the only listener today is the chat panel reading it on send.

let activeActionId = $state(0);

export const activeActionEditor = {
  get id() {
    return activeActionId;
  },
  set(id) {
    activeActionId = id || 0;
  },
};
