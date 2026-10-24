<script>
  import EmptyState from '../../components/EmptyState.svelte';

  /** @type {{ props: { message: string } }} */
  let { props } = $props();
</script>

<EmptyState title={props.message} />
