<script>
  import PublicBoard from '../pages/PublicBoard.svelte';
  import CustomerBoundary from './CustomerBoundary.svelte';
  let { slug } = $props();
</script>
<CustomerBoundary kind="board" {slug}><PublicBoard {slug} /></CustomerBoundary>
