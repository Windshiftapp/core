<script>
  import User from '@lucide/svelte/icons/user';
  import Avatar from '../../components/Avatar.svelte';

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
</script>

<div class="flex items-center gap-2 {interactive ? 'cursor-pointer' : ''}" data-testid={testId}>
  {#if user || fallbackAvatar}
    <Avatar size="2xs" variant="blue" name={userName} />
  {:else}
    <User class="w-4 h-4" style="color: var(--ds-text-subtle);" />
  {/if}
  <span class="text-sm truncate" style="color: var(--ds-text);">{userName}</span>
</div>
