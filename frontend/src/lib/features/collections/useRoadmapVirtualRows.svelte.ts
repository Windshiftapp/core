import { useVirtualRows } from '../../virtualization/useVirtualRows.svelte';
import { collectionItemRowKey } from './collectionVirtualRows';

export function useRoadmapVirtualRows(options: {
  getItems: () => any[];
  getRoot: () => HTMLElement | null;
  getList: () => HTMLElement | null;
  getIdentity: () => string;
  getEnabled: () => boolean;
}) {
  const rows = $derived(
    options
      .getItems()
      .map((value) => ({ key: collectionItemRowKey(value), value, estimatedSize: 40 }))
  );
  return useVirtualRows({
    getRows: () => rows,
    getScrollElement: options.getRoot,
    getListElement: options.getList,
    getIdentity: options.getIdentity,
    getEnabled: options.getEnabled,
    getFixedSize: () => 40,
  });
}
