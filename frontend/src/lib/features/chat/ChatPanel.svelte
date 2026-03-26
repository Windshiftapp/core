<script>
  import { fly } from 'svelte/transition';
  import { IconMessage, IconX, IconLoader2, IconSend, IconChevronDown, IconRefresh } from '@tabler/icons-svelte-runes';
  import { useEventListener } from 'runed';
  import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
  import { chatStore } from '../../stores/chatStore.svelte.js';
  import { navigate } from '../../router.js';
  import Select from '../../components/Select.svelte';

  let {
    isOpen = $bindable(false),
    onclose = () => {},
  } = $props();

  let inputText = $state('');
  let messagesContainer = $state(null);
  let textareaEl = $state(null);
  let previousFocusEl = $state(null);

  // Resize state
  let panelWidth = $state(400);
  let panelHeight = $state(500);
  let isResizing = $state(false);
  let resizeType = $state(null); // 'left' | 'top' | 'corner'
  let resizeStartX = $state(0);
  let resizeStartY = $state(0);
  let resizeStartWidth = $state(0);
  let resizeStartHeight = $state(0);

  // Position state (px from viewport edges)
  let panelRight = $state(16);
  let panelBottom = $state(16);

  // Drag state
  let isDragging = $state(false);
  let dragStartX = $state(0);
  let dragStartY = $state(0);
  let dragStartRight = $state(0);
  let dragStartBottom = $state(0);

  const isInteracting = $derived(isResizing || isDragging);

  function startResize(event, type) {
    isResizing = true;
    resizeType = type;
    resizeStartX = event.clientX;
    resizeStartY = event.clientY;
    resizeStartWidth = panelWidth;
    resizeStartHeight = panelHeight;
    event.preventDefault();
  }

  function handleResizeMove(event) {
    if (resizeType === 'left' || resizeType === 'corner') {
      panelWidth = Math.max(320, Math.min(700, resizeStartWidth + (resizeStartX - event.clientX)));
    }
    if (resizeType === 'top' || resizeType === 'corner') {
      const maxH = window.innerHeight - 32;
      panelHeight = Math.max(300, Math.min(maxH, resizeStartHeight + (resizeStartY - event.clientY)));
    }
  }

  function handleResizeUp() {
    isResizing = false;
    resizeType = null;
  }

  useEventListener(() => isResizing ? document : undefined, 'mousemove', handleResizeMove);
  useEventListener(() => isResizing ? document : undefined, 'mouseup', handleResizeUp);

  function startDrag(event) {
    if (event.target.closest('button, select')) return;
    isDragging = true;
    dragStartX = event.clientX;
    dragStartY = event.clientY;
    dragStartRight = panelRight;
    dragStartBottom = panelBottom;
    event.preventDefault();
  }

  function handleDragMove(event) {
    const deltaX = event.clientX - dragStartX;
    const deltaY = event.clientY - dragStartY;
    panelRight = Math.max(0, Math.min(window.innerWidth - panelWidth, dragStartRight - deltaX));
    panelBottom = Math.max(0, Math.min(window.innerHeight - panelHeight, dragStartBottom - deltaY));
  }

  function handleDragUp() {
    isDragging = false;
  }

  useEventListener(() => isDragging ? document : undefined, 'mousemove', handleDragMove);
  useEventListener(() => isDragging ? document : undefined, 'mouseup', handleDragUp);

  // Auto-scroll on new messages
  $effect(() => {
    const len = chatStore.messages.length;
    if (len && messagesContainer) {
      // Tick delay so DOM updates first
      requestAnimationFrame(() => {
        messagesContainer.scrollTop = messagesContainer.scrollHeight;
      });
    }
  });

  // Auto-focus textarea on open
  $effect(() => {
    if (isOpen && textareaEl) {
      previousFocusEl = document.activeElement;
      requestAnimationFrame(() => textareaEl?.focus());
    }
  });

  // Load LLM connections when panel opens
  $effect(() => {
    if (isOpen) {
      chatStore.loadConnections();
    }
  });

  function handleClose() {
    isOpen = false;
    onclose();
    // Restore focus
    if (previousFocusEl && typeof previousFocusEl.focus === 'function') {
      previousFocusEl.focus();
    }
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') {
      e.stopPropagation();
      handleClose();
    }
  }

  function handleTextareaKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  }

  function send() {
    const text = inputText.trim();
    if (!text || chatStore.loading) return;
    inputText = '';
    chatStore.sendMessage(text);
    // Reset textarea height
    if (textareaEl) {
      textareaEl.style.height = 'auto';
    }
  }

  function autoResize(e) {
    const el = e.target;
    el.style.height = 'auto';
    el.style.height = Math.min(el.scrollHeight, 120) + 'px';
  }

  function handleMessageClick(e) {
    const link = e.target.closest('a');
    if (!link) return;
    const key = link.textContent.trim();
    if (!/^[A-Z]{2,10}-\d+$/.test(key)) return;
    e.preventDefault();
    const item = chatStore.itemKeyMap[key];
    if (item) {
      navigate(`/workspaces/${item.workspaceId}/items/${item.id}`);
    }
  }

  function preprocessItemKeys(text) {
    if (!text) return '';
    return text.replace(/\b([A-Z]{2,10}-\d+)\b/g, '[$1](#)');
  }

  const activeConnection = $derived.by(() => {
    const conns = chatStore.connections;
    if (!conns.length) return null;
    if (chatStore.connectionId) {
      return conns.find(c => c.id === chatStore.connectionId) || null;
    }
    return conns.find(c => c.is_default) || conns[0] || null;
  });

</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
{#if isOpen}
  <div
    class="fixed z-40 flex flex-col shadow-2xl rounded-lg overflow-hidden"
    class:select-none={isInteracting}
    style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);
           width: {panelWidth}px; height: {panelHeight}px; max-height: calc(100vh - 32px);
           right: {panelRight}px; bottom: {panelBottom}px;"
    transition:fly={{ y: 100, duration: 250 }}
    onkeydown={handleKeydown}
  >
    <!-- Resize handles -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="absolute left-0 top-0 bottom-0 w-1.5 cursor-ew-resize z-10" onmousedown={(e) => startResize(e, 'left')}></div>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="absolute top-0 left-0 right-0 h-1.5 cursor-ns-resize z-10" onmousedown={(e) => startResize(e, 'top')}></div>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="absolute top-0 left-0 w-3 h-3 cursor-nwse-resize z-20" onmousedown={(e) => startResize(e, 'corner')}></div>

    <!-- Header -->
    <div
      class="flex items-center justify-between px-4 py-3 border-b cursor-grab"
      class:cursor-grabbing={isDragging}
      style="border-color: var(--ds-border);"
      onmousedown={startDrag}
    >
      <div class="flex items-center gap-2">
        <IconMessage size={20} stroke={1.5} style="color: var(--ds-text-subtle);" />
        <h2 class="text-sm font-semibold" style="color: var(--ds-text);">AI Chat</h2>
      </div>
      <div class="flex items-center gap-2">
        {#if chatStore.connections.length > 1}
          <Select
            value={chatStore.connectionId}
            onchange={(v) => { chatStore.connectionId = parseInt(v) || 0; }}
            size="small"
            options={[{ value: 0, label: 'Default' }, ...chatStore.connections.map(c => ({ value: c.id, label: c.name }))]}
          />
        {/if}
        <button
          onclick={handleClose}
          class="p-1 rounded transition-colors"
          style="color: var(--ds-text-subtle);"
          onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
          onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
          aria-label="Close chat"
        >
          <IconX size={20} stroke={1.5} />
        </button>
      </div>
    </div>

    {#if activeConnection}
      <div class="px-4 py-1.5 border-b text-xs" style="border-color: var(--ds-border); color: var(--ds-text-subtlest);">
        {activeConnection.model}
        {#if chatStore.connections.length > 1}
          <span style="color: var(--ds-text-disabled);">via {activeConnection.name}</span>
        {/if}
      </div>
    {/if}

    <!-- Messages -->
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div
      bind:this={messagesContainer}
      class="flex-1 overflow-y-auto px-4 py-3 space-y-3"
      onclick={handleMessageClick}
    >
      {#if chatStore.messages.length === 0}
        <div class="flex items-center justify-center h-full">
          <div class="text-center">
            <div class="mx-auto mb-3 w-fit"><IconMessage size={40} stroke={1.5} style="color: var(--ds-icon-subtle);" /></div>
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              Ask anything about your workspaces and items.
            </p>
          </div>
        </div>
      {:else}
        {#each chatStore.messages as msg}
          {#if msg.role === 'user'}
            <div class="flex justify-end">
              <div class="max-w-[85%] rounded-lg px-3 py-2 text-sm" style="background-color: var(--ds-interactive); color: var(--ds-text-inverse);">
                {msg.content}
              </div>
            </div>
          {:else}
            <div class="flex justify-start">
              <div class="max-w-full rounded-lg px-3 py-2 text-sm" style="background-color: var(--ds-surface-sunken); color: var(--ds-text);">
                {#if msg.error}
                  <p class="text-sm" style="color: var(--ds-text-danger);">{msg.error}</p>
                  {#if msg === chatStore.messages[chatStore.messages.length - 1] && !chatStore.loading}
                    <button
                      onclick={() => chatStore.retryLastMessage()}
                      class="mt-1.5 flex items-center gap-1 text-xs px-2 py-1 rounded transition-colors"
                      style="color: var(--ds-text-subtle); background-color: var(--ds-surface);"
                      onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
                      onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
                    >
                      <IconRefresh size={12} stroke={1.5} />
                      Retry
                    </button>
                  {/if}
                {:else}
                  <div class="chat-message-content">
                    <MilkdownEditor content={preprocessItemKeys(msg.content)} readonly={true} showToolbar={false} compact={true} />
                  </div>
                {/if}
              </div>
            </div>
          {/if}
        {/each}

        {#if chatStore.loading}
          <div class="flex justify-start">
            <div class="flex items-center gap-2 rounded-lg px-3 py-2 text-sm" style="background-color: var(--ds-surface-sunken); color: var(--ds-text-subtle);">
              <IconLoader2 size={16} stroke={1.5} class="animate-spin" />
              Thinking...
            </div>
          </div>
        {/if}
      {/if}
    </div>

    <!-- Input -->
    <div class="border-t px-4 py-3" style="border-color: var(--ds-border);">
      <div class="flex items-end gap-2">
        <textarea
          bind:this={textareaEl}
          bind:value={inputText}
          oninput={autoResize}
          onkeydown={handleTextareaKeydown}
          placeholder="Ask a question..."
          rows="1"
          class="flex-1 resize-none rounded-lg px-3 py-2 text-sm border focus:outline-none focus:ring-1"
          style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text); --tw-ring-color: var(--ds-border-focused);"
          disabled={chatStore.loading}
        ></textarea>
        <button
          onclick={send}
          disabled={chatStore.loading || !inputText.trim()}
          class="p-2 rounded-lg transition-colors flex-shrink-0"
          class:opacity-50={chatStore.loading || !inputText.trim()}
          style="background-color: var(--ds-interactive); color: var(--ds-text-inverse);"
          aria-label="Send message"
        >
          <IconSend size={16} stroke={1.5} />
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .chat-message-content :global(a) {
    color: var(--ds-link);
    text-decoration: underline;
    cursor: pointer;
  }
  .chat-message-content :global(a:hover) {
    text-decoration-thickness: 2px;
  }
  .chat-message-content :global(.milkdown) {
    padding: 0 !important;
    min-height: unset !important;
    border: none !important;
  }
  .chat-message-content :global(.editor) {
    padding: 0 !important;
  }
</style>
