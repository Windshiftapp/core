import { getContext, untrack } from 'svelte';
import type { RowRetention } from './types';
import { VIRTUAL_ROW_CONTEXT } from './types';

export function useRowRetention(active: () => boolean, reason: string) {
  const retention = getContext<RowRetention | undefined>(VIRTUAL_ROW_CONTEXT);
  $effect(() => {
    if (!active() || !retention) return;
    // Acquiring a pin invalidates the rendered range. Do not subscribe this
    // effect to row descriptors read by the imperative context callback.
    return untrack(() => retention.acquire(reason));
  });
}
