<script>
  import { currentRoute } from '../router.js';
  import PortalExperience from './PortalExperience.svelte';
  import CustomerBoundary from './CustomerBoundary.svelte';
</script>
<CustomerBoundary kind="portal" slug={$currentRoute.params.slug}><PortalExperience /></CustomerBoundary>
