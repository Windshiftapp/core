// Automatic activation: moving focus selects the next enabled local tab.
export function moveTabFocus(event, tabs, activeId, select) {
  if (event.altKey || event.ctrlKey || event.metaKey) return;
  const enabled = tabs.filter((tab) => !tab.disabled);
  if (!enabled.length) return;
  let index = enabled.findIndex((tab) => tab.id === activeId);
  const rtl = getComputedStyle(event.currentTarget).direction === 'rtl';
  if (event.key === 'Home') index = 0;
  else if (event.key === 'End') index = enabled.length - 1;
  else if (event.key === 'ArrowRight')
    index = (index + (rtl ? -1 : 1) + enabled.length) % enabled.length;
  else if (event.key === 'ArrowLeft')
    index = (index + (rtl ? 1 : -1) + enabled.length) % enabled.length;
  else return;
  event.preventDefault();
  const tab = enabled[index];
  select(tab.id);
  const controls = event.currentTarget.closest('[role="tablist"]').querySelectorAll('[role="tab"]');
  const target = [...controls].find((control) => control.dataset.tabId === tab.id);
  target?.focus({ preventScroll: true });
  target?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
}
