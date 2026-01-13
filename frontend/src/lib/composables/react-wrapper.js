import { createRoot } from 'react-dom/client';

/**
 * Mounts a React component into a DOM node
 * @param {HTMLElement} node - The DOM node to mount into
 * @param {React.Component} Component - The React component to render
 * @param {Object} props - Props to pass to the component
 * @returns {Object} Object with destroy method to unmount the component
 */
export function mountReactComponent(node, Component, props = {}) {
  const root = createRoot(node);
  root.render(Component(props));

  return {
    destroy() {
      root.unmount();
    },
    update(newProps) {
      root.render(Component(newProps));
    }
  };
}
