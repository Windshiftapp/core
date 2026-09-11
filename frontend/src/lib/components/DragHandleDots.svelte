<script>
  let {
    class: className = 'w-4 h-4',
    color = 'var(--ds-text-subtlest)',
  } = $props();
</script>

<svg class={className} style="color: {color};" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
  <circle cx="9" cy="6" r="1.5" />
  <circle cx="15" cy="6" r="1.5" />
  <circle cx="9" cy="12" r="1.5" />
  <circle cx="15" cy="12" r="1.5" />
  <circle cx="9" cy="18" r="1.5" />
  <circle cx="15" cy="18" r="1.5" />
</svg>
