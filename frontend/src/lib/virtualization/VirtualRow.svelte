<script lang="ts">
  import { setContext, untrack } from 'svelte';
  import type { Snippet } from 'svelte';
  import { VIRTUAL_ROW_CONTEXT } from './types';
  import type { RowPosition, VirtualRow } from './types';
  import type { VirtualRowsController } from './virtualRowsController';

  let { descriptor, position, controller, row, pane = 'main' } = $props<{
    descriptor: VirtualRow<any>;
    position: RowPosition;
    controller: VirtualRowsController<any>;
    pane?: string;
    row: Snippet<[any, { index: number; attach: (node: HTMLElement) => { destroy(): void } }]>;
  }>();

  // The keyed renderer remounts on identity/key changes, not snapshot updates.
  const retention = untrack(() => ({ controller, key: descriptor.key }));
  setContext(VIRTUAL_ROW_CONTEXT, { acquire: (reason: string) => retention.controller.acquirePin(retention.key, reason) });

  function attach(node: HTMLElement) {
    const registration = controller.attach(node, descriptor.key, position.index, pane);
    let release: (() => void) | null = null;
    const focus = () => { release ??= controller.acquirePin(descriptor.key, 'focus'); };
    const blur = () => {
      queueMicrotask(() => {
        if (!node.contains(node.ownerDocument.activeElement)) { release?.(); release = null; }
      });
    };
    const keydown = (event: KeyboardEvent) => { void controller.moveFocus(event, descriptor.key); };
    node.addEventListener('focusin', focus);
    node.addEventListener('focusout', blur);
    node.addEventListener('keydown', keydown);
    // Svelte owns this effect with the action, including conditional node removal.
    $effect(() => { registration.update({ key: descriptor.key, index: position.index }); });
    return {
      destroy() {
        node.removeEventListener('focusin', focus);
        node.removeEventListener('focusout', blur);
        node.removeEventListener('keydown', keydown);
        release?.();
        registration.destroy();
      },
    };
  }
</script>

{@render row(descriptor.value, { attach, index: position.index })}
