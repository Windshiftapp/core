<script>
  import { onMount } from 'svelte';

  let { children, height = 0, class: className = '', style = '' } = $props();

  let visible = $state(false);
  let el = $state(null);

  onMount(() => {
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          visible = true;
          observer.disconnect();
        }
      },
      { rootMargin: '200px' }
    );
    observer.observe(el);
    return () => observer.disconnect();
  });
</script>

<div
  bind:this={el}
  class={className}
  style="{height ? `min-height: ${height}px;` : ''}{style}"
>
  {#if visible}
    {@render children()}
  {/if}
</div>
