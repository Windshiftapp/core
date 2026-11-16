<script>
  import { untrack } from 'svelte';
  import { portalBoards } from '../api/portalBoards.js';
  import { customerAccess } from '../api/customerAccess.js';
  import { currentRoute } from '../router.js';
  import LazyRootView from '../components/LazyRootView.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Spinner from '../components/Spinner.svelte';
  import CustomerLogin from './CustomerLogin.svelte';
  const loadClassic = () => import('../layout/Portal.svelte');
  const loadCompact = () => import('./CompactPortal.svelte');
  let experience = $state(null);
  let error = $state('');
  let loginStatus = $state(null);
  let identityRevision = $state(0);
  const portalSlug = $derived($currentRoute.params.slug);
  const verifying = $derived(/^\/portal\/[^/]+\/verify$/.test($currentRoute.path));
  $effect(() => {
    const slug = portalSlug;
    identityRevision;
    const controller = new AbortController();
    experience = null;
    error = '';
    loginStatus = null;
    if (verifying) return () => controller.abort();
    untrack(() => portalBoards.experience(slug, { signal: controller.signal })).then(value => { if (!controller.signal.aborted) experience = value; })
      .catch(async err => {
        if (controller.signal.aborted) return;
        if (err.status === 401) {
          try {
            const status = await customerAccess.status('portal', slug);
            if (!controller.signal.aborted) loginStatus = status;
          } catch (statusError) {
            if (!controller.signal.aborted) error = statusError.message;
          }
        } else error = err.message;
      });
    return () => controller.abort();
  });
</script>
{#if verifying}<LazyRootView loader={loadClassic} label="Portal" />{:else if error}<AlertBox variant="error" message={error} />{:else if loginStatus}<CustomerLogin kind="portal" slug={portalSlug} status={loginStatus} onlogin={() => identityRevision++} />{:else if !experience}<Spinner />{:else if experience.presentation === 'compact'}<LazyRootView loader={loadCompact} label="Portal" componentProps={{ experience }} />{:else}<LazyRootView loader={loadClassic} label="Portal" componentProps={{ experience }} />{/if}
