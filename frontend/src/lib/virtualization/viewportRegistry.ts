import type { ScrollRoot } from './types';

interface Snapshot {
  offset: number;
  width: number;
  height: number;
  scrolling: boolean;
}

interface Registration {
  subscribers: Set<(snapshot: Snapshot) => void>;
  read: (scrolling?: boolean) => Snapshot;
  notify: (scrolling?: boolean) => void;
  destroy: () => void;
}

const roots = new Map<ScrollRoot, Registration>();

export function isWindow(root: ScrollRoot): root is Window {
  return 'window' in root && root.window === root;
}

export function rootOffset(root: ScrollRoot) {
  return isWindow(root) ? root.scrollY : root.scrollTop;
}

export function subscribeViewport(root: ScrollRoot, callback: (snapshot: Snapshot) => void) {
  let registration = roots.get(root);
  if (!registration) {
    const subscribers = new Set<(snapshot: Snapshot) => void>();
    const read = (scrolling = false): Snapshot => ({
      offset: rootOffset(root),
      width: isWindow(root) ? root.innerWidth : root.clientWidth,
      height: isWindow(root) ? root.innerHeight : root.clientHeight,
      scrolling,
    });
    const notify = (scrolling = false) => {
      const snapshot = read(scrolling);
      for (const subscriber of subscribers) subscriber(snapshot);
    };
    let settled: ReturnType<typeof setTimeout> | undefined;
    const scroll = () => {
      notify(true);
      clearTimeout(settled);
      settled = setTimeout(() => notify(false), 150);
    };
    const resize = () => notify();
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(resize);
    observer?.observe(isWindow(root) ? root.document.documentElement : root);
    root.addEventListener('scroll', scroll, { passive: true });
    const targetWindow = isWindow(root) ? root : root.ownerDocument.defaultView;
    targetWindow?.addEventListener('resize', resize);
    registration = {
      subscribers,
      read,
      notify,
      destroy() {
        clearTimeout(settled);
        observer?.disconnect();
        root.removeEventListener('scroll', scroll);
        targetWindow?.removeEventListener('resize', resize);
      },
    };
    roots.set(root, registration);
  }
  registration.subscribers.add(callback);
  callback(registration.read());
  let released = false;
  return () => {
    if (released) return;
    released = true;
    registration.subscribers.delete(callback);
    if (!registration.subscribers.size) {
      registration.destroy();
      roots.delete(root);
    }
  };
}

export function refreshViewport(root: ScrollRoot) {
  roots.get(root)?.notify();
}

export function listOrigin(root: ScrollRoot, list: HTMLElement) {
  const top = isWindow(root) ? 0 : root.getBoundingClientRect().top + root.clientTop;
  return list.getBoundingClientRect().top - top + rootOffset(root);
}
