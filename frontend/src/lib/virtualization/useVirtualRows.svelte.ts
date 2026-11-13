import { onDestroy, untrack } from 'svelte';
import type { ScrollRoot, VirtualRow } from './types';
import { VirtualRowsController } from './virtualRowsController';

export function useVirtualRows<T>(options: {
  getRows: () => readonly VirtualRow<T>[];
  getIdentity: () => string;
  getScrollElement: () => ScrollRoot | null;
  getListElement: () => HTMLElement | null;
  getEnabled?: () => boolean;
  getLayoutRevision?: () => string;
  getOverscan?: () => number;
  getFixedSize?: () => number | undefined;
}) {
  let revision = $state(0);
  const controller = new VirtualRowsController<T>({
    onChange: () => {
      revision = untrack(() => revision) + 1;
    },
  });
  $effect(() => {
    const configuration = {
      rows: options.getRows(),
      identity: options.getIdentity(),
      enabled: options.getEnabled?.() ?? true,
      layout: options.getLayoutRevision?.() ?? '',
      fixedSize: options.getFixedSize?.(),
      overscan: options.getOverscan?.(),
    };
    const root = options.getScrollElement();
    const list = options.getListElement();
    untrack(() => {
      controller.connect(root, list);
      controller.configure(configuration);
    });
  });
  onDestroy(() => controller.destroy());
  $effect(() => {
    const root = options.getScrollElement();
    if (!root) return;
    const before = () => controller.setPrinting(true);
    const after = () => controller.setPrinting(false);
    const cancel = () => controller.cancelScroll();
    window.addEventListener('beforeprint', before);
    window.addEventListener('afterprint', after);
    root.addEventListener('wheel', cancel, { passive: true });
    root.addEventListener('touchstart', cancel, { passive: true });
    return () => {
      window.removeEventListener('beforeprint', before);
      window.removeEventListener('afterprint', after);
      root.removeEventListener('wheel', cancel);
      root.removeEventListener('touchstart', cancel);
    };
  });
  return {
    controller,
    get tokens() {
      void revision;
      return controller.tokens;
    },
    get virtual() {
      void revision;
      return controller.virtual;
    },
    get visibleKeys() {
      void revision;
      return controller.visibleKeys;
    },
  };
}
