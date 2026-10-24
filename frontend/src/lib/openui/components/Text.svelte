<script>
  /** @type {{ props: { text: string, tone?: 'default'|'muted' } }} */
  let { props } = $props();

  const style = $derived(
    props.tone === 'muted' ? 'color: var(--ds-text-subtle);' : 'color: var(--ds-text);'
  );
</script>

<p class="text-sm" {style} data-testid="openui-text">{props.text}</p>
