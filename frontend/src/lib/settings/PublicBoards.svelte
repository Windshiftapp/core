<script>
  import Ban from '@lucide/svelte/icons/ban';
  import Copy from '@lucide/svelte/icons/copy';
  import ExternalLink from '@lucide/svelte/icons/external-link';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import Settings from '@lucide/svelte/icons/settings';
  import { boardPublications } from '../api/boardPublications.js';
  import { isSystemAdmin } from '../stores/permissions.svelte.js';
  import { errorToast, successToast, surfaceError } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { copyToClipboard } from '../utils/clipboard.js';
  import AlertBox from '../components/AlertBox.svelte';
  import BoardPublicationLocations from '../features/collections/BoardPublicationLocations.svelte';
  import { publicBaseURL } from '../runtime/contextPath.js';
  import DataTable from '../components/DataTable.svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import SelectField from '../components/SelectField.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import BoardSharingEditor from '../features/collections/BoardSharingEditor.svelte';
  import ConfirmWithReasonDialog from '../dialogs/ConfirmWithReasonDialog.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';

  let query = $state('');
  let publicationState = $state('all');
  let accessMode = $state('');
  let page = $state(1);
  let rows = $state([]);
  let total = $state(0);
  let loading = $state(true);
  let error = $state('');
  let selected = $state(null);
  let revokeTarget = $state(null);
  let showRevoke = $state(false);
  let refresh = $state(0);

  $effect(() => {
    const filters = { q: query, state: publicationState, access_mode: accessMode, page, page_size: 25 };
    const controller = new AbortController();
    const admin = $isSystemAdmin;
    refresh;
    loading = true;
    error = '';
    boardPublications.list(filters, { admin, signal: controller.signal }).then(result => {
      if (controller.signal.aborted) return;
      rows = result.data;
      total = result.pagination.total_items;
    }).catch(err => { if (!controller.signal.aborted) error = err.message; })
      .finally(() => { if (!controller.signal.aborted) loading = false; });
    return () => controller.abort();
  });

  const columns = [
    { key: 'name', label: 'Board' },
    { key: 'owner_name', label: 'Owner', render: item => !item.owner_id ? 'Missing owner' : `${item.owner_name || `User ${item.owner_id}`}${item.owner_active === false ? ' (inactive)' : ''}` },
    { key: 'workspace_name', label: 'Workspace', render: item => item.workspace_id ? item.workspace_name || `Workspace ${item.workspace_id}` : 'Global' },
    { key: 'enabled', label: 'Publication', render: item => item.revoked_at ? 'Revoked' : !item.enabled ? 'Disabled' : !item.instance_enabled ? 'Instance paused' : 'Enabled' },
    { key: 'policy.mode', label: 'Board audience', render: item => item.policy.mode === 'public' ? 'Anyone with the link' : item.policy.organisation_ids.length ? item.policy.organisation_ids.map(id => item.organisations?.find(org => org.id === id)?.name || `Organisation ${id}`).join(', ') : 'Nobody (empty allowlist)' },
    { key: 'locations', label: 'Used by / effective access', slot: 'locations' },
    { key: 'changed_at', label: 'Sharing changed', render: item => item.changed_at ? `${item.changed_at} · ${item.changed_by_name || 'Deleted actor'}` : 'History unavailable' },
    { key: 'actions', label: t('common.actions') },
  ];

  function boardURL(item) {
    return `${publicBaseURL()}/board/${item.slug}`;
  }

  async function copyLink(item) {
    const ok = await copyToClipboard(boardURL(item));
    if (ok) successToast(`Copied the link to ${item.name}`);
    else errorToast('Could not copy the board link');
  }

  function requestRevoke(item) {
    revokeTarget = item;
    showRevoke = true;
  }

  async function confirmRevoke(reason) {
    const item = revokeTarget;
    if (!item) return;
    try {
      await boardPublications.revoke(item.collection_id, item.revision, reason);
      successToast(`Revoked the public link for ${item.name}`);
      refresh++;
    } catch (err) {
      surfaceError(err, { context: 'public boards revoke' });
    } finally {
      revokeTarget = null;
    }
  }

  async function allowRepublish(item) {
    try {
      await boardPublications.allowRepublish(item.collection_id, item.revision);
      successToast(`Allowed re-publishing for ${item.name}`);
      refresh++;
    } catch (err) {
      surfaceError(err, { context: 'public boards allow republish' });
    }
  }

  function buildActionItems(item) {
    /** @type {any[]} */
    const items = [
      {
        id: 'manage',
        title: 'Manage sharing',
        icon: Settings,
        testid: `public-board-manage-${item.collection_id}`,
        onClick: () => (selected = item.collection_id)
      }
    ];
    if (item.enabled && item.standalone_enabled && item.slug && !item.revoked_at && item.instance_enabled) {
      items.push(
        {
          id: 'open',
          title: 'Open shared board',
          icon: ExternalLink,
          testid: `public-board-open-${item.collection_id}`,
          href: boardURL(item),
          target: '_blank'
        },
        {
          id: 'copy',
          title: 'Copy link',
          icon: Copy,
          testid: `public-board-copy-${item.collection_id}`,
          onClick: () => copyLink(item)
        }
      );
    }
    if (item.can_administer) {
      items.push({ type: 'divider' });
      if (item.revoked_at) {
        items.push({
          id: 'allow-republish',
          title: 'Allow re-publish',
          icon: RotateCcw,
          testid: 'public-board-allow-republish',
          onClick: () => allowRepublish(item)
        });
      } else {
        items.push({
          id: 'revoke',
          title: 'Revoke and lock publication',
          icon: Ban,
          color: 'var(--ds-text-danger)',
          testid: 'public-board-revoke',
          onClick: () => requestRevoke(item)
        });
      }
    }
    return items;
  }
</script>

<section class="space-y-5" data-testid="public-boards-management">
  <PageHeader title="Public boards" description={$isSystemAdmin ? 'Review and manage board publications across all owners.' : 'Manage publication and customer access for boards you own.'} />
  <div class="flex flex-wrap items-end gap-3" data-testid="public-boards-filters">
    <SearchInput bind:value={query} on_input={() => (page = 1)} placeholder="Search name or slug" class="min-w-[220px] flex-1 max-w-xs" dataTestid="public-boards-search" />
    <SelectField label="Publication state" value={publicationState} onchange={value => { publicationState = value; page = 1; }} options={[{ value: 'all', label: 'All' }, { value: 'enabled', label: 'Enabled' }, { value: 'disabled', label: 'Disabled' }, { value: 'revoked', label: 'Revoked' }]} dataTestid="public-boards-state" class="w-44" />
    <SelectField label="Audience" value={accessMode} onchange={value => { accessMode = value; page = 1; }} options={[{ value: '', label: 'All audiences' }, { value: 'public', label: 'Unrestricted' }, { value: 'organisations', label: 'Windshift organisations' }]} dataTestid="public-boards-access-mode" class="w-56" />
  </div>
  {#if error}<AlertBox variant="error" message={error} />{/if}
  <DataTable {columns} data={rows} keyField="collection_id" {loading} pagination totalItems={total} pageSize={25} bind:currentPage={page} emptyMessage="No previously published boards" actionItems={buildActionItems} actionTriggerTestid={item => `public-board-actions-${item.collection_id}`} rowAttrs={item => ({ 'data-testid': `public-board-row-${item.collection_id}` })}>
    {#snippet locations(item)}<BoardPublicationLocations publication={item} />{/snippet}
  </DataTable>
</section>

<Modal isOpen={selected !== null} onclose={() => selected = null} maxWidth="max-w-xl" closeOnBackdropClick={false} dataTestid="public-board-management-dialog">
  <ModalHeader title="Share board" onclose={() => selected = null} />
  {#if selected !== null}
    <div class="px-6 py-5"><BoardSharingEditor collectionId={selected} oncancel={() => selected = null} onsaved={(_, close) => { refresh++; if (close) selected = null; }} /></div>
  {/if}
</Modal>

<ConfirmWithReasonDialog
  bind:show={showRevoke}
  variant="danger"
  title="Revoke and lock publication"
  message={`Revoke the public link for ${revokeTarget?.name ?? 'this board'}? Owners cannot re-publish until an administrator clears the lock.`}
  reasonLabel="Administrative revoke reason"
  reasonPlaceholder="Why is this publication being revoked?"
  confirmText="Revoke and lock"
  onconfirm={confirmRevoke}
  oncancel={() => (revokeTarget = null)}
/>
