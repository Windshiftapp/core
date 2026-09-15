<script>
  import { ExternalLink, Loader } from '@lucide/svelte';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { successToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { workspacesStore } from '../stores';
  import { formatRelativeCompact } from '../utils/dateFormatter.js';
  import { renderMarkdown } from '../utils/render-markdown.js';
  import SafeMarkdown from '../components/SafeMarkdown.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { requirementStatusLozenge } from '../features/requirements/requirementStatuses.js';
  import MobileHeader from './MobileHeader.svelte';

  let { workspaceId, requirementNumber } = $props();

  let detail = $state(null);
  let page = $state(null);
  let assignableUsers = $state([]);
  let loading = $state(true);
  let errored = $state(false);
  let loadToken = 0;

  const workspaceName = $derived.by(() => {
    const store = $workspacesStore;
    const regular = store?.regularWorkspaces?.find((ws) => ws.id === workspaceId);
    if (regular) return regular.name;
    if (store?.personalWorkspace?.id === workspaceId) return store.personalWorkspace.name ?? '';
    return '';
  });

  const contentHtml = $derived(page ? renderMarkdown(page.content) : '');

  const ownerLabel = $derived.by(() => {
    if (!detail?.owner_id) return t('requirements.mobile.ownerUnset');
    const user = assignableUsers.find((u) => u.id === detail.owner_id);
    if (!user) return `#${detail.owner_id}`;
    return [user.first_name, user.last_name].filter(Boolean).join(' ') || user.username || user.email;
  });

  function back() {
    if (window.history.length > 1) window.history.back();
    else navigate('/m/requirements');
  }

  function openInPages() {
    if (!detail?.page_id) return;
    navigate(`/m/pages/${workspaceId}/${detail.page_id}`);
  }

  async function copyKey() {
    if (!detail?.key) return;
    try {
      await navigator.clipboard.writeText(detail.key);
      successToast(t('requirements.keyCopied'));
    } catch {
      // ignore clipboard errors
    }
  }

  async function load(token) {
    loading = true;
    errored = false;
    try {
      const [detailRes, usersRes] = await Promise.allSettled([
        api.requirements.get(workspaceId, requirementNumber),
        api.getAssignableUsers(workspaceId),
      ]);
      if (token !== loadToken) return;
      if (detailRes.status === 'rejected') throw detailRes.reason;
      detail = detailRes.value;
      assignableUsers = usersRes.status === 'fulfilled' ? (usersRes.value ?? []) : [];

      const pageRes = await api.pages.getPage(workspaceId, detail.page_id);
      if (token !== loadToken) return;
      page = pageRes;
    } catch (err) {
      console.error('Failed to load requirement:', err);
      if (token === loadToken) errored = true;
    } finally {
      if (token === loadToken) loading = false;
    }
  }

  $effect(() => {
    const ws = workspaceId;
    const num = requirementNumber;
    if (ws == null || num == null) return;
    const token = ++loadToken;
    detail = null;
    page = null;
    assignableUsers = [];
    load(token);
  });
</script>

<MobileHeader title={detail?.page_title ?? detail?.key ?? t('requirements.navTitle')} onback={back} />

{#if loading}
  <div class="center" data-testid="mobile-requirement-loading"><Loader class="spin" size={22} /></div>
{:else if errored || !detail}
  <div class="msg" data-testid="mobile-requirement-error">
    <p>{t('requirements.mobile.detailLoadError')}</p>
    <button class="retry" onclick={() => load(++loadToken)} disabled={loading} type="button">
      {t('common.retry')}
    </button>
  </div>
{:else}
  <div class="detail" data-testid="mobile-requirement-detail">
    <button class="key-btn" onclick={copyKey} type="button" title={t('requirements.copyKey')}>
      <Lozenge color="blue">{detail.key}</Lozenge>
    </button>

    <p class="meta" data-testid="mobile-requirement-meta">
      {#if workspaceName}{workspaceName} · {/if}
      {t('requirements.fieldType')}: {t(`requirements.type.${detail.requirement_type}`)} ·
      {t('requirements.fieldStatus')}:
      <Lozenge color={requirementStatusLozenge(detail.status)}>
        {t(`requirements.status.${detail.status}`)}
      </Lozenge>
      {#if detail.updated_at}
        · {formatRelativeCompact(new Date(detail.updated_at))}
      {/if}
    </p>

    <p class="owner" data-testid="mobile-requirement-owner">
      {t('requirements.fieldOwner')}: {ownerLabel}
    </p>

    <button class="pages-link" onclick={openInPages} data-testid="mobile-requirement-open-pages" type="button">
      <ExternalLink size={16} />
      {t('requirements.openInPages')}
    </button>

    <section class="traceability" data-testid="mobile-requirement-traceability">
      <h2 class="section-title">{t('requirements.mobile.traceabilitySummary')}</h2>
      <div class="trace-stats">
        <span>{t('requirements.traceability.linkedItems')}: {detail.linked_item_count ?? 0}</span>
        <span>{t('requirements.traceability.linkedTests')}: {detail.linked_test_count ?? 0}</span>
        {#if (detail.linked_test_count ?? 0) > 0}
          <Lozenge color="green">{t('requirements.traceability.covered')}</Lozenge>
        {:else}
          <Lozenge color="red">{t('requirements.traceability.uncovered')}</Lozenge>
        {/if}
      </div>
    </section>

    {#if page?.content}
      <SafeMarkdown html={contentHtml} testid="mobile-requirement-content" />
    {:else}
      <p class="empty">{t('requirements.emptyDescription')}</p>
    {/if}
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
  .retry:disabled { opacity: 0.6; }

  .detail { padding: 0.875rem 0.875rem 2rem; }

  .key-btn {
    border: none;
    background: transparent;
    padding: 0;
    margin-bottom: 0.5rem;
    cursor: pointer;
  }

  .meta,
  .owner {
    font-size: 0.8125rem;
    color: var(--ds-text-subtle);
    margin: 0 0 0.5rem;
    line-height: 1.45;
  }

  .pages-link {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    margin-bottom: 1rem;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--ds-text-link, var(--ds-interactive));
    font-size: 0.875rem;
    cursor: pointer;
  }

  .traceability {
    margin-bottom: 1.25rem;
    padding: 0.75rem;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    background: var(--ds-surface-raised);
  }
  .section-title {
    font-size: 0.875rem;
    font-weight: var(--font-semibold, 600);
    color: var(--ds-text);
    margin: 0 0 0.5rem;
  }
  .trace-stats {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.8125rem;
    color: var(--ds-text-subtle);
  }

  .empty { color: var(--ds-text-subtle); font-size: 0.875rem; }
</style>
