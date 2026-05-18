// actionMutations is an in-memory pub/sub bus for "an action was just
// mutated by an AI tool call in this tab". The chatStore emits after every
// chat response whose tool calls include create_action or update_action;
// the action editor subscribes and refetches when the emitted id matches
// the action it currently has open.
//
// Modeled on stores/notifications.js's subscribeToNewNotifications pattern.
// No cross-tab broadcasting — server push is out of scope.

const subscribers = new Set();

export const actionMutations = {
  emit(actionId) {
    const id = Number(actionId);
    if (!id) return;
    for (const fn of subscribers) {
      try {
        fn(id);
      } catch (err) {
        console.error('actionMutations subscriber threw:', err);
      }
    }
  },
  subscribe(fn) {
    subscribers.add(fn);
    return () => subscribers.delete(fn);
  },
};

// Exposed on window so Playwright specs can simulate the chat-side
// trigger without standing up an LLM. Benign in production: the bus is
// in-memory and the action editor's subscriber refetches via the
// authenticated API, which still enforces server-side permissions.
if (typeof window !== 'undefined') {
  // eslint-disable-next-line no-underscore-dangle
  window.__actionMutations = actionMutations;
}
