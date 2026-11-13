<script lang="ts">
  import { getContext, untrack } from 'svelte';
  import type { Snippet } from 'svelte';
  import { useVirtualRows } from '../virtualization/useVirtualRows.svelte';
  import type { ScrollRoot } from '../virtualization/types';
  import { VIRTUAL_SCROLL_CONTEXT } from '../virtualization/types';
  import VirtualRow from '../virtualization/VirtualRow.svelte';

  let {
    items = [], getKey, estimateSize = () => 48, identity = '', enabled = true,
    layoutRevision = '', fixedSize, scrollElement, listElement = null, row, spacer = null,
    onrangechange = null, dragSources = false, dragRevision = '', sharedModel = null, pane = 'main',
  } = $props<{
    items: any[];
    getKey: (item: any, index: number) => string;
    estimateSize?: (item: any, index: number) => number;
    identity?: string;
    enabled?: boolean;
    layoutRevision?: string;
    fixedSize?: number;
    scrollElement?: ScrollRoot | null;
    listElement?: HTMLElement | null;
    row: Snippet<[any, { index: number; attach: (node: HTMLElement) => { destroy(): void } }]>;
    spacer?: Snippet<[number]> | null;
    onrangechange?: ((keys: string[]) => void) | null;
    dragSources?: boolean;
    dragRevision?: string;
    sharedModel?: ReturnType<typeof useVirtualRows<any>> | null;
    pane?: string;
  }>();

  const shellRoot = getContext<{ element: HTMLElement | null; revision?: string } | undefined>(VIRTUAL_SCROLL_CONTEXT);
  const rows = $derived(items.map((value, index) => ({ key: getKey(value, index), value, estimatedSize: estimateSize(value, index) })));
  // A paired surface supplies its model for this renderer's entire lifetime.
  const virtual = untrack(() => sharedModel) ?? useVirtualRows({
    getRows: () => rows,
    getIdentity: () => identity,
    getEnabled: () => enabled,
    getScrollElement: () => scrollElement === undefined ? shellRoot?.element ?? null : scrollElement,
    getListElement: () => listElement,
    getLayoutRevision: () => `${layoutRevision}|${shellRoot?.revision ?? ''}`,
    getFixedSize: () => fixedSize,
  });

  $effect(() => { onrangechange?.(virtual.visibleKeys); });

  let dragSession: { invalidate(): void; destroy(): void } | null = null;
  $effect(() => {
    if (!dragSources) return;
    const root = scrollElement === undefined ? shellRoot?.element : scrollElement;
    if (!root || !('ownerDocument' in root)) return;
    let disposed = false;
    void import('../virtualization/virtualDragSession').then(({ createVirtualDragSession }) => {
      if (!disposed) dragSession = createVirtualDragSession(virtual.controller, root);
    });
    return () => { disposed = true; dragSession?.destroy(); dragSession = null; };
  });
  $effect(() => { void identity; void dragRevision; dragSession?.invalidate(); });

</script>

{#each virtual.tokens as token (token.key)}
  {#if token.kind === 'gap'}
    {#if spacer}
      {@render spacer(token.size)}
    {:else}
      <div aria-hidden="true" data-virtual-gap style="height: {token.size}px; overflow-anchor: none; pointer-events: none;"></div>
    {/if}
  {:else}
    <VirtualRow descriptor={virtual.controller.rows[token.row.index]} position={token.row} controller={virtual.controller} {row} {pane} />
  {/if}
{/each}
