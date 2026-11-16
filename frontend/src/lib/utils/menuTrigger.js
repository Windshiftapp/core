// Menu triggers mark themselves with data-menu-trigger. Clickable rows and
// cards call this to ignore clicks that belong to a menu instead of the row,
// which lets the trigger click bubble to melt's outside-click handling.
export function isMenuTriggerEvent(event) {
  return event.target instanceof Element && event.target.closest('[data-menu-trigger]') !== null;
}
