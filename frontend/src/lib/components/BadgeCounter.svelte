<script>
  let {
    count = 0,
    max = 99,
    class: className = ''
  } = $props();

  const displayCount = $derived(count > max ? `${max}+` : count);
</script>

{#if count > 0}
  <span class="absolute -top-1 -right-1 bg-red-500 text-white text-xs font-bold rounded-full min-w-[18px] h-[18px] flex items-center justify-center {className}">
    {displayCount}
  </span>
{/if}
