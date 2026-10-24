<script>
  /** @type {{ props: { text: string, level?: '1'|'2'|'3' } }} */
  let { props } = $props();

  const classes = $derived({
    1: 'text-lg font-semibold',
    2: 'text-base font-semibold',
    3: 'text-sm font-medium',
  });
  const className = $derived(classes[props.level ?? '2'] ?? classes['2']);
</script>

<h3 class="{className}" style="color: var(--ds-text);" data-testid="openui-title">{props.text}</h3>
