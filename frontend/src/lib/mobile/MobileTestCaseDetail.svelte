<script>
  import { ExternalLink, Loader } from '@lucide/svelte';
  import { navigate } from '../router.js';
  import { t } from '../stores/i18n.svelte.js';
  import Lozenge from '../components/Lozenge.svelte';
  import MobileHeader from './MobileHeader.svelte';
  import { loadMobileTestCaseSummary } from './mobileTestCaseData.js';

  let { workspaceId, testId } = $props();

  let testCase = $state(null);
  let loading = $state(true);
  let errored = $state(false);
  let loadToken = 0;

  function back() {
    if (window.history.length > 1) window.history.back();
    else navigate('/m/requirements');
  }

  function openFullTestCase() {
    navigate(`/workspaces/${workspaceId}/tests/cases/${testId}`);
  }

  async function load(token) {
    loading = true;
    errored = false;
    try {
      testCase = await loadMobileTestCaseSummary(workspaceId, testId);
      if (token !== loadToken) return;
    } catch (err) {
      console.error('Failed to load test case:', err);
      if (token === loadToken) errored = true;
    } finally {
      if (token === loadToken) loading = false;
    }
  }

  $effect(() => {
    const ws = workspaceId;
    const id = testId;
    if (ws == null || id == null) return;
    const token = ++loadToken;
    testCase = null;
    load(token);
  });
</script>

<MobileHeader title={testCase?.title ?? t('requirements.mobile.testCaseTitle')} onback={back} />

{#if loading}
  <div class="center" data-testid="mobile-test-case-loading"><Loader class="spin" size={22} /></div>
{:else if errored || !testCase}
  <div class="msg" data-testid="mobile-test-case-error">
    <p>{t('requirements.mobile.testLoadError')}</p>
    <button class="retry" onclick={() => load(++loadToken)} disabled={loading} type="button">
      {t('common.retry')}
    </button>
  </div>
{:else}
  <div class="detail" data-testid="mobile-test-case-detail">
    <h2 class="title">{testCase.title}</h2>
    {#if testCase.status}
      <Lozenge color="green">{testCase.status}</Lozenge>
    {/if}
    {#if testCase.format}
      <p class="meta">{t('requirements.mobile.testFormat', { format: testCase.format })}</p>
    {/if}
    {#if testCase.priority}
      <p class="meta">{t('requirements.mobile.testPriority', { priority: testCase.priority })}</p>
    {/if}
    <button class="desktop-link" onclick={openFullTestCase} data-testid="mobile-test-case-open-full" type="button">
      <ExternalLink size={16} />
      {t('requirements.mobile.openFullTestCase')}
    </button>
  </div>
{/if}

<style>
  .center { display: flex; justify-content: center; padding: 3rem; color: var(--ds-text-subtle); }
  :global(.spin) { animation: spin 1s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }

  .msg { padding: 3rem 1.25rem; text-align: center; color: var(--ds-text-subtle); }
  .msg p { margin: 0; }
  .retry {
    min-height: 40px;
    margin-top: 0.75rem;
    padding: 0.45rem 1rem;
    border: 1px solid var(--ds-interactive);
    border-radius: var(--radius-md, 6px);
    background: var(--ds-interactive);
    color: var(--ds-text-inverse, #fff);
    font: inherit;
    font-weight: var(--font-semibold, 600);
    cursor: pointer;
  }

  .detail { padding: 0.875rem 0.875rem 2rem; }
  .title { margin: 0 0 0.75rem; font-size: 1.125rem; font-weight: 600; }
  .meta { margin: 0.5rem 0 0; font-size: 0.875rem; color: var(--ds-text-subtle); }

  .desktop-link {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    margin-top: 1.25rem;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--ds-text-link, var(--ds-interactive));
    font-size: 0.875rem;
    cursor: pointer;
  }
</style>
