export function clickOutside(node, { capture = true } = {}) {
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
    // The click handler may replace the trigger before the event bubbles here.
    if (!event.composedPath().includes(node) && !containsControlledPopup(event.target)) {
      node.dispatchEvent(new CustomEvent('clickOutside'));
    }
  }

  // Capture is the default so a container sees the click before handlers on
  // descendants can stop it. A container that wraps editors which own their
  // own outside-click handling should pass `{ capture: false }`: the editor's
  // capture listener then commits before the container's bubble listener tears
  // the editor down, instead of removing that listener mid-dispatch.
  document.addEventListener('click', handleClick, capture);
  return {
    destroy() {
      document.removeEventListener('click', handleClick, capture);
    },
  };
}
