<script>
  import { onMount, tick } from 'svelte';
  import { ChevronLeft, Send, Sparkles, Loader, Trash2 } from '@lucide/svelte';
  import { chatStore } from '../stores/chatStore.svelte.js';
  import { buildChatContext } from '../features/chat/chatContext.js';
  import { currentRoute, navigate } from '../router.js';
  import { renderMarkdown } from '../utils/render-markdown.js';
  import Textarea from '../components/Textarea.svelte';

  let text = $state('');
  let scrollEl = $state(null);

  const messages = $derived(chatStore.messages);
  const loading = $derived(chatStore.loading);
  const conversationLoading = $derived(chatStore.conversationLoading);

  function back() {
    if (window.history.length > 1) window.history.back();
    else navigate('/m');
  }

  async function send() {
    const t = text.trim();
    if (!t || loading || conversationLoading) return;
    text = '';
    // Same global chatStore the desktop drives; context is derived from the
    // current route (general assistant on the mobile surface).
    chatStore.sendMessage(t, buildChatContext($currentRoute));
  }

  function onKeydown(e) {
    // Enter sends; Shift+Enter for a newline.
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  }

  // Keep the latest message in view.
  $effect(() => {
    void messages.length;
    void loading;
    tick().then(() => {
      if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
    });
  });

  onMount(() => {
    chatStore.loadConnections();
    void chatStore.startNewGeneralChat();
  });
</script>

<div class="chat">
<header class="chat-header" data-testid="mobile-chat-header">
  <button class="back" onclick={back} aria-label="Back" type="button"><ChevronLeft size={24} /></button>
  <span class="title"><Sparkles size={17} /> Assistant</span>
  {#if messages.length > 0}
    <button class="clear" onclick={() => chatStore.clearHistory()} aria-label="Clear conversation" type="button"><Trash2 size={18} /></button>
  {:else}
    <span class="spacer"></span>
  {/if}
</header>

<div class="messages" bind:this={scrollEl} data-testid="mobile-chat-messages">
  {#if messages.length === 0}
    <div class="empty" data-testid="chat-empty">
      <Sparkles size={28} />
      <p>Ask about your work</p>
      <span>The assistant can search items, summarize, and help you plan.</span>
    </div>
  {:else}
    {#each messages as msg, i (i)}
      {#if msg.role === 'user'}
        <div class="msg user" data-testid="chat-msg-user">{msg.content}</div>
      {:else}
        <div class="msg assistant" data-testid="chat-msg-assistant">
          {#if msg.error}
            <p class="err">{msg.error}</p>
          {:else}
            <div class="html-content">{@html renderMarkdown(msg.content)}</div>
            {#if msg.needsReview}
              <p class="review">⚠ This answer may need review.</p>
            {/if}
          {/if}
        </div>
      {/if}
    {/each}
  {/if}
  {#if loading}
    <div class="msg assistant thinking" data-testid="chat-thinking"><Loader class="spin" size={16} /> Thinking…</div>
  {/if}
</div>

<form class="composer" onsubmit={(e) => { e.preventDefault(); send(); }}>
  <Textarea
    bind:value={text}
    onkeydown={onKeydown}
    placeholder="Message the assistant…"
    rows={1}
    data-testid="chat-input"
    class="mobile-chat-input"
  />
  <button class="send" disabled={!text.trim() || loading || conversationLoading} data-testid="chat-send" aria-label="Send" type="submit">
    <Send size={18} />
  </button>
</form>
</div>

<style>
  /* Fill the shell's content area: header + scrollable messages + pinned
     composer, so the input sits at the bottom regardless of message count. */
  .chat { height: 100%; display: flex; flex-direction: column; min-height: 0; }

  .chat-header {
    flex-shrink: 0;
    display: flex; align-items: center; gap: 0.5rem;
    min-height: 52px; padding: 0.5rem 0.75rem;
    padding-top: calc(env(safe-area-inset-top, 0px) + 0.5rem);
    background-color: var(--ds-surface); border-bottom: 1px solid var(--ds-border);
  }
  .back, .clear {
    display: inline-flex; align-items: center; justify-content: center;
    width: 36px; height: 36px; border: none; background: transparent; color: var(--ds-text); cursor: pointer; flex-shrink: 0;
  }
  .back { margin-left: -6px; }
  .title { flex: 1; display: inline-flex; align-items: center; gap: 0.4rem; font-size: 1.0625rem; font-weight: var(--font-semibold, 600); color: var(--ds-text); }
  .spacer { width: 36px; flex-shrink: 0; }

  .messages {
    flex: 1 1 auto; min-height: 0; overflow-y: auto; -webkit-overflow-scrolling: touch;
    display: flex; flex-direction: column; gap: 0.6rem; padding: 0.85rem;
  }

  .msg { max-width: 85%; padding: 0.6rem 0.8rem; border-radius: var(--radius-lg, 8px); font-size: 0.9375rem; line-height: 1.5; overflow-wrap: anywhere; }
  .user { align-self: flex-end; background-color: var(--ds-interactive); color: var(--ds-text-inverse, #fff); border-bottom-right-radius: var(--radius-sm, 4px); white-space: pre-wrap; }
  .assistant { align-self: flex-start; background-color: var(--ds-surface-raised); color: var(--ds-text); border: 1px solid var(--ds-border); border-bottom-left-radius: var(--radius-sm, 4px); }
  .assistant :global(.html-content) { font-size: 0.9375rem; }
  .err { margin: 0; color: var(--ds-text-danger, var(--ds-danger)); }
  .review { margin: 0.4rem 0 0; font-size: 0.75rem; color: var(--ds-text-warning, var(--ds-text-subtle)); }
  .thinking { display: inline-flex; align-items: center; gap: 0.4rem; color: var(--ds-text-subtle); }
  :global(.spin) { animation: spin 1s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }

  .empty { display: flex; flex-direction: column; align-items: center; gap: 0.4rem; padding: 3rem 1rem; color: var(--ds-text-subtle); text-align: center; }
  .empty p { margin: 0.25rem 0 0; font-weight: var(--font-medium, 500); color: var(--ds-text); }
  .empty span { font-size: 0.8125rem; }

  .composer {
    flex-shrink: 0;
    display: flex; align-items: flex-end; gap: 0.5rem;
    padding: 0.6rem 0.75rem; padding-bottom: calc(env(safe-area-inset-bottom, 0px) + 0.6rem);
    background-color: var(--ds-surface); border-top: 1px solid var(--ds-border);
  }
  .composer :global(.mobile-chat-input) {
    flex: 1; min-width: 0; max-height: 8rem; resize: none;
    padding: 0.55rem 0.7rem; border: 1px solid var(--ds-border); border-radius: var(--radius-lg, 8px);
    background-color: var(--ds-background-input, var(--ds-surface)); color: var(--ds-text);
    font-size: 1rem; font-family: inherit; line-height: 1.4;
  }
  .send {
    flex-shrink: 0; width: 40px; height: 40px; display: inline-flex; align-items: center; justify-content: center;
    border: none; border-radius: var(--radius-full, 9999px);
    background-color: var(--ds-interactive); color: var(--ds-text-inverse, #fff); cursor: pointer;
  }
  .send:disabled { opacity: 0.5; }
</style>
