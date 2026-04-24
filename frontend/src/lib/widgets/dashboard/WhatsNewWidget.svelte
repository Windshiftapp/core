<script>
  import { Bell, Inbox } from 'lucide-svelte';
  import { homepageStore } from '../../stores';
  import { navigate } from '../../router.js';

  let notifications = $derived(homepageStore.notifications);
  let loading = $derived(homepageStore.loading);

  function open(notification) {
    if (notification?.link) navigate(notification.link);
  }
</script>

{#if loading && notifications.length === 0}
  <div class="space-y-2 animate-pulse">
    {#each Array(3) as _}
      <div class="h-10 rounded" style="background-color: var(--ds-background-neutral);"></div>
    {/each}
  </div>
{:else if notifications.length === 0}
  <div class="flex flex-col items-center text-center py-6" style="color: var(--ds-text-subtle);">
    <Inbox class="w-6 h-6 mb-2 opacity-60" />
    <p class="text-sm">You're all caught up</p>
  </div>
{:else}
  <ul class="flex flex-col divide-y" style="border-color: var(--ds-border);">
    {#each notifications as n (n.id)}
      <li>
        <button
          class="w-full text-left px-1 py-2 flex items-start gap-2 transition-colors"
          style="color: var(--ds-text);"
          onmouseenter={(e) =>
            (e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)')}
          onmouseleave={(e) => (e.currentTarget.style.backgroundColor = '')}
          onclick={() => open(n)}
        >
          <Bell
            class="w-4 h-4 mt-0.5 flex-shrink-0"
            style={n.read ? 'color: var(--ds-text-subtlest);' : 'color: var(--ds-icon-accent);'}
          />
          <div class="min-w-0 flex-1">
            <p
              class="text-xs truncate"
              style={n.read ? 'color: var(--ds-text-subtle);' : 'font-weight: 500;'}
            >
              {n.message || n.title || 'Notification'}
            </p>
            {#if n.timestamp}
              <p class="text-[0.65rem] mt-0.5" style="color: var(--ds-text-subtlest);">
                {homepageStore.formatRelativeTime(n.timestamp)}
              </p>
            {/if}
          </div>
        </button>
      </li>
    {/each}
  </ul>
{/if}
