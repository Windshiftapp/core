<script>
  import { boardPublications } from '../api/boardPublications.js';
  import { isSystemAdmin } from '../stores/permissions.svelte.js';
  import AlertBox from '../components/AlertBox.svelte';
  import Button from '../components/Button.svelte';
  import CopyButton from '../components/CopyButton.svelte';
  import BoardPublicationLocations from '../features/collections/BoardPublicationLocations.svelte';
  import { publicBaseURL } from '../runtime/contextPath.js';
  import DataTable from '../components/DataTable.svelte';
  import SelectField from '../components/SelectField.svelte';
  import TextField from '../components/TextField.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import BoardSharingEditor from '../features/collections/BoardSharingEditor.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';

  let query = $state('');
  let publicationState = $state('all');
  let accessMode = $state('');
  let workspace = $state('');
  let owner = $state('');
  let organisation = $state('');
  let page = $state(1);
  let rows = $state([]);
  let total = $state(0);
  let loading = $state(true);
  let error = $state('');
  let selected = $state(null);
  let refresh = $state(0);

  $effect(() => {
    const filters = { q: query, state: publicationState, access_mode: accessMode, workspace_id: workspace && workspace !== 'global' ? workspace : null, global: workspace === 'global' ? true : null, owner_id: owner || null, organisation_id: organisation || null, page, page_size: 25 };
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
    { key: 'actions', label: '', slot: 'actions' },
  ];
</script>

<section class="space-y-5" data-testid="public-boards-management">
  <PageHeader title="Public boards" description={$isSystemAdmin ? 'Review and manage board publications across all owners.' : 'Manage publication and customer access for boards you own.'} />
  <div class="grid gap-3 sm:grid-cols-3">
    <TextField id="public-boards-search" dataTestid="public-boards-search" label="Search name or slug" value={query} oninput={event => { query = event.currentTarget.value; page = 1; }} />
    <SelectField id="public-boards-state" label="Publication state" value={publicationState} onchange={value => { publicationState = value; page = 1; }} options={[{ value: 'all', label: 'All' }, { value: 'enabled', label: 'Enabled' }, { value: 'disabled', label: 'Disabled' }, { value: 'revoked', label: 'Revoked' }]} />
    <SelectField id="public-boards-access-mode" label="Audience" value={accessMode} onchange={value => { accessMode = value; page = 1; }} options={[{ value: '', label: 'All audiences' }, { value: 'public', label: 'Unrestricted' }, { value: 'organisations', label: 'Windshift organisations' }]} />
    <TextField id="public-boards-workspace" label="Workspace ID or global" value={workspace} oninput={event => { workspace = event.currentTarget.value; page = 1; }} />
    <TextField id="public-boards-owner" label="Owner ID" type="number" value={owner} oninput={event => { owner = event.currentTarget.value; page = 1; }} />
    <TextField id="public-boards-organisation" label="Allowed organisation ID" type="number" value={organisation} oninput={event => { organisation = event.currentTarget.value; page = 1; }} />
  </div>
  {#if error}<AlertBox variant="error" message={error} />{/if}
  <DataTable {columns} data={rows} keyField="collection_id" {loading} pagination totalItems={total} pageSize={25} bind:currentPage={page} emptyMessage="No previously published boards" rowAttrs={item => ({ 'data-testid': `public-board-row-${item.collection_id}` })}>
    {#snippet locations(item)}<BoardPublicationLocations publication={item} />{/snippet}
    {#snippet actions(item)}
      <div class="flex flex-col items-start gap-2">
        <Button dataTestid={`public-board-manage-${item.collection_id}`} onclick={() => selected = item.collection_id}>Manage sharing</Button>
        {#if item.enabled && item.standalone_enabled && item.slug && !item.revoked_at && item.instance_enabled}
          <Button href={`${publicBaseURL()}/board/${item.slug}`} target="_blank" dataTestid={`public-board-open-${item.collection_id}`}>Open shared board</Button>
          <CopyButton getText={() => `${publicBaseURL()}/board/${item.slug}`} label="Copy link" />
        {/if}
      </div>
    {/snippet}
  </DataTable>
</section>

<Modal isOpen={selected !== null} onclose={() => selected = null} maxWidth="max-w-xl" closeOnBackdropClick={false} dataTestid="public-board-management-dialog">
  <ModalHeader title="Share board" onclose={() => selected = null} />
  {#if selected !== null}
    <div class="px-6 py-5"><BoardSharingEditor collectionId={selected} oncancel={() => selected = null} onsaved={(_, close) => { refresh++; if (close) selected = null; }} /></div>
  {/if}
</Modal>
