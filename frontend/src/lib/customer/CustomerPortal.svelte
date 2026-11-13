<script>
  import { currentRoute } from '../router.js';
  import Portal from '../layout/Portal.svelte';
  import CustomerBoundary from './CustomerBoundary.svelte';
</script>
<CustomerBoundary kind="portal" slug={$currentRoute.params.slug}><Portal /></CustomerBoundary>
