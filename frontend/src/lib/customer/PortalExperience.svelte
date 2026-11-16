<script>
  import { untrack } from 'svelte';
  import { portalBoards } from '../api/portalBoards.js';
  import { currentRoute } from '../router.js';
  import LazyRootView from '../components/LazyRootView.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Spinner from '../components/Spinner.svelte';
  const loadClassic = () => import('../layout/Portal.svelte');
  const loadCompact = () => import('./CompactPortal.svelte');
  let experience = $state(null);
  let error = $state('');
  const portalSlug = $derived($currentRoute.params.slug);
  const verifying = $derived($currentRoute.path.endsWith('/verify'));
  $effect(() => {
    const slug = portalSlug;
    verifying;
    const controller = new AbortController();
    experience = null;
    error = '';
    untrack(() => portalBoards.experience(slug, { signal: controller.signal })).then(value => { if (!controller.signal.aborted) experience = value; })
      .catch(err => { if (!controller.signal.aborted) error = err.message; });
    return () => controller.abort();
  });
</script>
{#if error}<AlertBox variant="error" message={error} />{:else if !experience}<Spinner />{:else if experience.presentation === 'compact' && !$currentRoute.path.endsWith('/verify')}<LazyRootView loader={loadCompact} label="Portal" componentProps={{ experience }} />{:else}<LazyRootView loader={loadClassic} label="Portal" componentProps={{ experience }} />{/if}
