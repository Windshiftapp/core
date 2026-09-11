<script>
  import { User } from '@lucide/svelte';

  let {
    user = null,
    fallbackName = '',
    fallbackAvatar = false,
    interactive = false,
    testId = undefined,
  } = $props();

  const userName = $derived(
    user
      ? `${user.first_name || ''} ${user.last_name || ''}`.trim() || user.username || fallbackName
      : fallbackName,
  );
  const initials = $derived(
    user
      ? (user.first_name?.[0] || '') + (user.last_name?.[0] || '') || user.username?.[0]?.toUpperCase() || '?'
      : fallbackName.split(' ').map((part) => part[0]).join('').toUpperCase().slice(0, 2) || '?',
  );
</script>

<div class="flex items-center gap-2 {interactive ? 'cursor-pointer' : ''}" data-testid={testId}>
  {#if user || fallbackAvatar}
    <div class="w-5 h-5 rounded-full bg-blue-500 flex items-center justify-center text-white text-[10px] font-medium">
      {initials}
    </div>
  {:else}
    <User class="w-4 h-4" style="color: var(--ds-text-subtle);" />
  {/if}
  <span class="text-sm truncate" style="color: var(--ds-text);">{userName}</span>
</div>
