<script>
  import DataTable from '../../components/DataTable.svelte';
  import Button from '../../components/Button.svelte';
  import BackButton from '../../components/BackButton.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import IconArchive from '@tabler/icons-svelte-runes/icons/archive';
  import { pages } from '../../api/pages.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { formatDateSimple } from '../../utils/dateFormatter.js';
  import { pagesTreeRefresh } from './pagesTreeRefresh.svelte.js';

  /** Full-page admin archive list with sorting and unarchive actions. The
   * backend enforces admin access and returns 404 to unauthorized deep links. */
  let { workspaceId } = $props();

  let rows = $state(/** @type {any[]} */ ([]));
  let loading = $state(true);
  let pendingId = $state(/** @type {number | null} */ (null));

  $effect(() => {
    if (workspaceId) {
      void load();
    }
  });

  async function load() {
    loading = true;
    try {
      const data = await pages.listArchived(workspaceId);
      rows = Array.isArray(data) ? data : [];
    } catch (err) {
      errorToast(err?.message || t('pages.archivedLoadError'));
      rows = [];
    } finally {
      loading = false;
    }
  }

  async function handleUnarchive(row) {
    const ok = await confirm({
      title: t('pages.archivedUnarchiveTitle', { title: row.title }),
      message: t('pages.archivedUnarchiveMessage'),
      confirmText: t('pages.archivedUnarchiveConfirm'),
      variant: 'primary',
    });
    if (!ok) return;
    pendingId = row.id;
    try {
      await pages.unarchive(workspaceId, row.id);
      successToast(t('pages.archivedUnarchiveOK', { title: row.title }));
      rows = rows.filter((r) => r.id !== row.id);
      // Restore the page to the sidebar tree.
      // for any sibling tab that has the tree loaded.
      pagesTreeRefresh.bump();
    } catch (err) {
      errorToast(err?.message || t('pages.archivedUnarchiveError'));
    } finally {
      pendingId = null;
    }
  }

  function goBack() {
    navigate(`/workspaces/${workspaceId}/pages`);
  }

  const columns = $derived([
    { key: 'title', label: t('pages.archivedColTitle'), sortable: true },
    {
      key: 'archived_at',
      label: t('pages.archivedColArchivedAt'),
      sortable: true,
      render: (row) => formatDateSimple(row.archived_at),
    },
    {
      key: 'archived_by_name',
      label: t('pages.archivedColArchivedBy'),
      sortable: true,
      render: (row) => row.archived_by_name || '—',
    },
    { key: 'unarchive', label: '', slot: 'unarchive', width: '8rem', align: 'text-right' },
  ]);
</script>

<main class="archived-page" data-testid="archived-pages-view">
  <PageHeader
    icon={IconArchive}
    title={t('pages.archivedHeading')}
    subtitle={t('pages.archivedSubtitle')}
  >
    {#snippet actions()}
      <BackButton onclick={goBack} testid="archived-pages-back" label={t('pages.archivedBack')} />
    {/snippet}
  </PageHeader>

  <DataTable
    {columns}
    data={rows}
    keyField="id"
    {loading}
    rowAttrs={(row) => ({ 'data-testid': `archived-page-row-${row.id}` })}
    emptyMessage={t('pages.archivedEmpty')}
    emptyIcon={IconArchive}
  >
    {#snippet unarchive(row)}
      <Button
        variant="secondary"
        size="small"
        disabled={pendingId === row.id}
        onclick={() => handleUnarchive(row)}
        dataTestid="archived-page-unarchive"
        dataPageId={row.id}
      >
        {t('pages.archivedUnarchive')}
      </Button>
    {/snippet}
  </DataTable>
</main>

<style>
  .archived-page {
    padding: 1.5rem 2rem 2rem;
    max-width: 72rem;
    margin: 0 auto;
    width: 100%;
  }
</style>
