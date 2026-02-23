export function portal(node, target = document.body) {
  target.appendChild(node);
  return {
    destroy() {
      node.remove();
    }
  };
}
