<script>
  import { customerAccess } from '../api/customerAccess.js';
  import { portalAuth } from '../api/portal.js';
  import CustomerLogin from './CustomerLogin.svelte';
  import Button from '../components/Button.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  let { kind, slug, children } = $props();
  let status = $state(null);
  let loading = $state(true);
  let error = $state('');

  async function refresh() {
    loading = true;
    error = '';
    try { status = await customerAccess.status(kind, slug); }
    catch (err) { status = null; error = err.message; }
    finally { loading = false; }
  }
  $effect(() => {
    const type = kind;
    const key = slug;
    let active = true;
    loading = true;
    error = '';
    status = null;
    (async () => {
      try {
        const token = new URLSearchParams(window.location.hash.slice(1)).get('customer_token');
        if (type === 'board' && token) {
          history.replaceState(null, '', window.location.pathname + window.location.search);
          await customerAccess.login(type, key, 'verify', { token });
        }
        const portalToken = new URLSearchParams(window.location.hash.slice(1)).get('token') || new URLSearchParams(window.location.search).get('token');
        if (type === 'portal' && portalToken) {
          const access = await customerAccess.status(type, key);
          if (access.restricted && !access.allowed) {
            await portalAuth.verifyMagicLink(key, portalToken);
            history.replaceState(null, '', window.location.pathname);
          }
        }
        const result = await customerAccess.status(type, key);
        if (active) status = result;
      } catch (err) { if (active) error = err.message; }
      finally { if (active) loading = false; }
    })();
    return () => { active = false; };
  });
</script>

{#if loading}
  <div class="p-8" role="status">Loading…</div>
{:else if error}
  <div class="max-w-md mx-auto p-8 space-y-4"><AlertBox variant="error" message={error} /><Button onclick={refresh} dataTestid="customer-access-retry">Try again</Button></div>
{:else if status?.allowed}
  {#if kind === 'board' && status.authenticated}
    <div class="p-2 text-right"><Button variant="subtle" dataTestid="customer-board-logout" onclick={async () => { await customerAccess.login(kind, slug, 'logout'); await refresh(); }}>Sign out</Button></div>
  {/if}
  {@render children()}
{:else if status}
  <main class="max-w-md mx-auto p-8"><CustomerLogin {kind} {slug} {status} onlogin={refresh} /></main>
{/if}
