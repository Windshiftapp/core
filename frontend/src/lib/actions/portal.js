export function portal(node, target = document.body) {
  // A falsy target opts out of portaling so callers can toggle it via a prop
  // without conditionally using the action.
  if (!target) {
    return {};
  }
  target.appendChild(node);
  return {
    destroy() {
      node.remove();
    },
  };
}
