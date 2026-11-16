export function clickOutside(node) {
  function containsControlledPopup(target) {
    const controls = [node, ...node.querySelectorAll('[aria-controls], [aria-owns]')];
    return controls.some((control) =>
      `${control.getAttribute('aria-controls') ?? ''} ${control.getAttribute('aria-owns') ?? ''}`
        .split(/\s+/)
        .filter(Boolean)
        .some((id) => node.ownerDocument.getElementById(id)?.contains(target))
    );
  }

  function handleClick(event) {
    if (!node.contains(event.target) && !containsControlledPopup(event.target)) {
      node.dispatchEvent(new CustomEvent('clickOutside'));
    }
  }

  document.addEventListener('click', handleClick, true);
  return {
    destroy() {
      document.removeEventListener('click', handleClick, true);
    },
  };
}
