import { getContext, setContext } from 'svelte';
import { on } from 'svelte/events';

const contextKey = Symbol('modal-scope');
const scopes = [];
const isolated = new Map();
const announcements = new Set();
let observer;
let restoring = false;
let lastActive;
const candidateSelector =
  'a[href], area[href], button, input, select, textarea, iframe, [contenteditable="true"], [tabindex]';

export function isFocusEligible(element) {
  if (!(element instanceof HTMLElement) || !element.isConnected) return false;
  if (element.matches(':disabled, [disabled], input[type="hidden"]')) return false;
  if (element.closest('[inert], [hidden], [aria-hidden="true"]')) return false;
  for (let node = element; node instanceof HTMLElement; node = node.parentElement) {
    const style = getComputedStyle(node);
    if (
      style.display === 'none' ||
      style.visibility === 'hidden' ||
      style.visibility === 'collapse'
    )
      return false;
  }
  return true;
}

export function tabStops(root) {
  return [...root.querySelectorAll(candidateSelector)]
    .filter((element) => element.tabIndex >= 0 && isFocusEligible(element))
    .sort((a, b) => (a.tabIndex || Infinity) - (b.tabIndex || Infinity));
}

function activeScope() {
  return scopes
    .filter((scope) => scope.root?.isConnected)
    .sort((a, b) => {
      const layers = (scope) => {
        const result = [];
        for (
          let element = scope.root;
          element && element !== document.body;
          element = element.parentElement
        ) {
          const style = getComputedStyle(element);
          const zIndex = Number.parseInt(style.zIndex, 10);
          if (!Number.isNaN(zIndex)) result.unshift(zIndex);
        }
        return result;
      };
      const left = layers(a);
      const right = layers(b);
      for (let index = 0; index < Math.max(left.length, right.length); index++) {
        const difference = (left[index] || 0) - (right[index] || 0);
        if (difference) return difference;
      }
      return scopes.indexOf(a) - scopes.indexOf(b);
    })
    .at(-1);
}

function roots(scope) {
  return [scope.root, ...scope.popups.keys(), ...announcements].filter((root) => root?.isConnected);
}

function contains(scope, target) {
  return roots(scope).some((root) => root.contains(target));
}

function restoreIsolation() {
  for (const [element, previous] of isolated) {
    for (const name of ['inert', 'aria-hidden']) {
      if (previous[name] === null) element.removeAttribute(name);
      else element.setAttribute(name, previous[name]);
    }
    if (previous.visibility !== undefined) {
      if (previous.visibility)
        element.style.setProperty('visibility', previous.visibility, previous.priority);
      else element.style.removeProperty('visibility');
    }
  }
  isolated.clear();
}

function isolate() {
  restoreIsolation();
  const scope = activeScope();
  lastActive = scope;
  if (!scope) {
    observer?.takeRecords();
    return;
  }
  const allowed = roots(scope);
  function hide(element, visually = false) {
    if (!isolated.has(element)) {
      isolated.set(element, {
        inert: element.getAttribute('inert'),
        'aria-hidden': element.getAttribute('aria-hidden'),
      });
      element.setAttribute('inert', '');
      element.setAttribute('aria-hidden', 'true');
    }
    if (visually && isolated.get(element).visibility === undefined) {
      const previous = isolated.get(element);
      previous.visibility = element.style.getPropertyValue('visibility');
      previous.priority = element.style.getPropertyPriority('visibility');
      element.style.setProperty('visibility', 'hidden');
    }
  }
  function visit(parent) {
    for (const element of parent.children) {
      if (allowed.includes(element)) continue;
      if (allowed.some((root) => element.contains(root))) visit(element);
      else if (!element.matches('script, style, link')) {
        hide(element);
      }
    }
  }
  visit(document.body);
  for (const other of scopes) {
    if (other !== scope && other.root && !allowed.some((root) => other.root.contains(root)))
      hide(other.root);
    if (other !== scope)
      for (const root of other.popups.keys()) {
        if (!allowed.some((allowedRoot) => root.contains(allowedRoot))) hide(root, true);
      }
  }
  const layer = Number.parseInt(getComputedStyle(scope.root).zIndex, 10) || 0;
  for (const root of scope.popups.keys()) {
    if (root.isConnected && (Number.parseInt(getComputedStyle(root).zIndex, 10) || 0) <= layer)
      root.style.zIndex = String(layer + 1);
  }
  observer?.takeRecords();
}

function focus(element) {
  element?.focus({ preventScroll: true });
}

function focusFallback(scope) {
  focus(
    (isFocusEligible(scope.lastFocus) && scope.lastFocus) || tabStops(scope.root)[0] || scope.root
  );
}

function popupFor(scope, target) {
  return [...scope.popups.entries()]
    .reverse()
    .find(([root, options]) => root.contains(target) || options.trigger?.()?.contains(target));
}

function nextFromTrigger(scope, trigger, backwards) {
  const stops = tabStops(scope.root);
  const index = stops.indexOf(trigger);
  return stops[(index + (backwards ? -1 : 1) + stops.length) % stops.length] || scope.root;
}

function scopeTabStops(scope) {
  return [scope.root, ...announcements].filter((root) => root?.isConnected).flatMap(tabStops);
}

function handleTab(event) {
  const scope = activeScope();
  if (!scope || event.key !== 'Tab' || event.ctrlKey || event.metaKey || event.altKey) return;
  const active = document.activeElement;
  const popup = popupFor(scope, active);
  event.preventDefault();
  if (popup) {
    const [root, options] = popup;
    const stops = options.keepTabStops ? tabStops(root) : [];
    const index = stops.indexOf(active);
    const next = stops[index + (event.shiftKey ? -1 : 1)];
    if (next && (index >= 0 || !event.shiftKey)) {
      focus(next);
      return;
    }
    const trigger = options.trigger?.();
    options.close?.();
    focus(nextFromTrigger(scope, trigger, event.shiftKey));
    return;
  }
  const stops = scopeTabStops(scope);
  const index = stops.indexOf(active);
  const next =
    index < 0
      ? event.shiftKey
        ? stops.at(-1)
        : stops[0]
      : stops[(index + (event.shiftKey ? -1 : 1) + stops.length) % stops.length];
  focus(next || scope.root);
}

function handleKeydown(event) {
  const scope = activeScope();
  if (!scope) return;
  const popup = popupFor(scope, event.target);
  if (popup) {
    if (event.key === 'Escape' && !event.defaultPrevented) {
      event.preventDefault();
      popup[1].close?.();
      focus(popup[1].trigger?.());
    }
  } else if (
    (contains(scope, event.target) || event.key === 'Escape') &&
    ![...announcements].some((root) => root.contains(event.target))
  ) {
    scope.onKeydown?.(event);
  }
  // Window-level shortcuts must not dispatch the same modal gesture again.
  event.stopPropagation();
}

function handleFocus(event) {
  const scope = activeScope();
  if (!scope || restoring) return;
  if (contains(scope, event.target)) scope.lastFocus = event.target;
  else {
    restoring = true;
    focusFallback(scope);
    restoring = false;
  }
}

function start() {
  document.addEventListener('keydown', handleTab, true);
  document.addEventListener('keydown', handleKeydown);
  document.addEventListener('focusin', handleFocus, true);
  observer = new MutationObserver(isolate);
  observer.observe(document.body, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ['class', 'style'],
  });
}

function stop() {
  observer?.disconnect();
  observer = undefined;
  document.removeEventListener('keydown', handleTab, true);
  document.removeEventListener('keydown', handleKeydown);
  document.removeEventListener('focusin', handleFocus, true);
}

export function createOverlayScope() {
  const parent = getContext(contextKey);
  const scope = {
    parent,
    root: null,
    popups: new Map(),
    headings: new Set(),
    trigger: null,
    lastFocus: null,
    onKeydown: null,
  };
  setContext(contextKey, scope);
  return scope;
}

export function createDialogHeading() {
  const scope = getContext(contextKey);
  return (heading) => {
    scope?.headings.add(heading);
    return {
      destroy() {
        scope?.headings.delete(heading);
      },
    };
  };
}

// Legacy full-screen editors share ownership without changing their layout.
export function createOverlayDialog(onKeydown) {
  const scope = createOverlayScope();
  return (root) => {
    const destroy = mountOverlay(scope, root, onKeydown);
    queueMicrotask(() => {
      if (overlayOwnsFocus(root) && !overlayHasFocusedControl(root)) focus(root);
    });
    return { destroy };
  };
}

// Transient feedback remains readable and actionable above the modal stack.
export function overlayAnnouncement(root) {
  announcements.add(root);
  isolate();
  return {
    destroy() {
      announcements.delete(root);
      isolate();
    },
  };
}

export function mountOverlay(scope, root, onKeydown) {
  scope.root = root;
  scope.trigger = document.activeElement;
  scope.onKeydown = onKeydown;
  const removeKeydown = on(root, 'keydown', handleKeydown);
  if (scopes.length === 0) start();
  scopes.push(scope);
  isolate();
  return () => {
    removeKeydown();
    const wasActive = lastActive === scope || activeScope() === scope;
    scopes.splice(scopes.indexOf(scope), 1);
    scope.root = null;
    scope.lastFocus = null;
    isolate();
    if (!scopes.length) stop();
    if (!wasActive) return;
    // Allow simultaneous parent/child teardown and replacement dialogs to settle.
    queueMicrotask(() => {
      const resumed = activeScope();
      if (resumed && resumed !== scope.parent && !contains(resumed, scope.trigger)) return;
      if (
        resumed &&
        contains(resumed, document.activeElement) &&
        document.activeElement !== document.body
      )
        return;
      if (
        isFocusEligible(scope.trigger) &&
        scope.trigger.matches(candidateSelector) &&
        (!resumed || contains(resumed, scope.trigger))
      )
        focus(scope.trigger);
      else if (resumed) focusFallback(resumed);
      else if (!document.activeElement || document.activeElement === document.body) {
        focus(tabStops(document.body)[0]);
      }
    });
  };
}

export function overlayOwnsFocus(root) {
  const scope = activeScope();
  return scope?.root === root;
}

export function overlayHasFocusedControl(root) {
  const scope = activeScope();
  return (
    scope?.root === root &&
    document.activeElement !== root &&
    contains(scope, document.activeElement)
  );
}

// Capture component ownership before a positioning action moves the popup DOM.
export function createOwnedPopup() {
  const scope = getContext(contextKey);
  const action = (root, options = {}) => {
    if (!scope) return {};
    const triggerAtOpen = document.activeElement;
    const previousLayer = root.style.getPropertyValue('z-index');
    const previousPriority = root.style.getPropertyPriority('z-index');
    const resolved = {
      trigger: () => {
        const explicit = options.trigger?.();
        if (explicit) return explicit;
        return (
          roots(scope)
            .flatMap((root) => [...root.querySelectorAll('[aria-controls], [aria-owns]')])
            .find((element) =>
              `${element.getAttribute('aria-controls')} ${element.getAttribute('aria-owns')}`
                .split(' ')
                .includes(root.id)
            ) || triggerAtOpen
        );
      },
      close: () => options.close?.(),
      keepTabStops: options.keepTabStops ?? true,
    };
    scope.popups.set(root, resolved);
    const removeKeydown = on(root, 'keydown', handleKeydown);
    isolate();
    return {
      update(next) {
        options = next;
        resolved.keepTabStops = next.keepTabStops ?? true;
      },
      destroy() {
        removeKeydown();
        const hadFocus = root.contains(document.activeElement) || root.contains(scope.lastFocus);
        scope.popups.delete(root);
        if (previousLayer) root.style.setProperty('z-index', previousLayer, previousPriority);
        else root.style.removeProperty('z-index');
        isolate();
        if (hadFocus)
          queueMicrotask(() => {
            if (activeScope() === scope && document.activeElement === document.body)
              focus(resolved.trigger());
          });
      },
    };
  };
  action.withinModal = Boolean(scope);
  return action;
}
