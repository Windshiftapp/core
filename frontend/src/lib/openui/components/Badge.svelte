<script>
  import Badge from '../../components/Badge.svelte';

  /** @type {{ props: { text: string, tone?: 'neutral'|'info'|'success'|'warning'|'danger' } }} */
  let { props } = $props();
</script>

<Badge variant={props.tone ?? 'neutral'}>{props.text}</Badge>
