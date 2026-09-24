<script>
  let {
    backgroundStyle = '',
    contextVars = '',
    class: className = '',
    // min-h-screen suits views that scroll inside main; views that are
    // themselves viewport-height scrollers pass min-h-full instead.
    rootClass = 'min-h-screen',
    // Views whose horizontal scrolling happens in a nested container (fixed
    // toolbar + scrolling canvas) need a viewport-width root instead of a
    // content-width one.
    fullWidth = false,
    contentClass = 'p-6',
    contentStyle = '',
    rootStyle = '',
    testid = undefined,
    children
  } = $props();
</script>

<div
  class={`${fullWidth ? 'w-full' : 'w-fit min-w-full'} ${rootClass} ${className}`}
  style={`${backgroundStyle} ${contextVars} background-attachment: fixed; ${rootStyle}`}
  data-testid={testid}
>
  <div class={contentClass} style={contentStyle}>
    {#if children}
      {@render children()}
    {/if}
  </div>
</div>
