<script>
  import { onMount } from 'svelte';
  import { jiraImport } from './JiraImportStore.svelte.js';
  import { toHotkeyString, getShortcutDisplay } from '../utils/keyboardShortcuts.js';
  import JiraImportWizard from './JiraImportWizard.svelte';
  import Button from '../components/Button.svelte';
  import Spinner from '../components/Spinner.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Input from '../components/Input.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import {
    Cloud, Plus, Trash2, ExternalLink, Link, Clock,
    CheckCircle, XCircle, Loader, PlayCircle, AlertTriangle
  } from '@lucide/svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import { addToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { formatAuthenticatedDateTime as formatDateTimeLocale } from '../utils/authenticatedDateFormatter.js';
  import { confirm } from '../composables/useConfirm.js';
  import { safeHref } from '../utils/sanitize';

  // State
  let showWizard = $state(false);
  let selectedConnectionId = $state(null);
  let deleteJob = $state(null);
  let deleteConfirmation = $state('');
  let isDeletingImportedData = $state(false);

  // Derived state from store
  let savedConnections = $derived(jiraImport.savedConnections);
  let importJobs = $derived(jiraImport.importJobs);
  let canConfirmDeleteImportData = $derived(deleteJob && deleteConfirmation.trim() === deleteJob.id);

  onMount(() => {
    jiraImport.loadSavedConnections();
    jiraImport.loadImportJobs();
  });

  function openWizard(connectionId = null) {
    selectedConnectionId = connectionId;
    jiraImport.reset();
    if (connectionId) {
      const conn = savedConnections.items.find(c => c.id === connectionId);
      if (conn) {
        jiraImport.useSavedConnection(conn);
      }
    }
    showWizard = true;
  }

  function closeWizard() {
    showWizard = false;
    selectedConnectionId = null;
    // Refresh the lists after wizard closes
    jiraImport.loadSavedConnections();
    jiraImport.loadImportJobs();
  }

  async function deleteConnection(connectionId) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('systemImport.deleteConnectionMessage'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;
    const result = await jiraImport.deleteSavedConnection(connectionId);
    if (result.success) {
      addToast({ message: t('systemImport.connectionDeleted'), variant: 'success' });
    } else {
      addToast({ message: result.error, variant: 'error' });
    }
  }

  function formatDate(dateString) {
    if (!dateString) return '-';
    return formatDateTimeLocale(dateString);
  }

  function getStatusColor(status) {
    switch (status) {
      case 'completed': return 'var(--ds-text-success)';
      case 'completed_with_errors': return 'var(--ds-text-warning)';
      case 'running': return 'var(--ds-text-accent-blue)';
      case 'failed': return 'var(--ds-text-danger)';
      case 'data_deleted': return 'var(--ds-text-subtle)';
      case 'queued': return 'var(--ds-text-subtle)';
      default: return 'var(--ds-text-subtle)';
    }
  }

  function getStatusIcon(status) {
    switch (status) {
      case 'completed': return CheckCircle;
      case 'completed_with_errors': return AlertTriangle;
      case 'running': return Loader;
      case 'failed': return XCircle;
      case 'data_deleted': return Trash2;
      case 'queued': return Clock;
      default: return Clock;
    }
  }

  function statusLabel(status) {
    const known = new Set(['completed', 'running', 'failed', 'data_deleted', 'queued']);
    return known.has(status) ? t(`systemImport.statuses.${status}`) : status?.replaceAll('_', ' ');
  }

  function phaseLabel(phase) {
    const known = new Set(['initializing', 'importing_project', 'completed', 'running']);
    return known.has(phase) ? t(`systemImport.phases.${phase}`) : phase?.replaceAll('_', ' ');
  }

  function scopeLabel(scope) {
    return scope === 'work_items' ? t('systemImport.scopes.workItems') : scope?.replaceAll('_', ' ');
  }

  function canDeleteImportedData(job) {
    return job && !['queued', 'running', 'data_deleted'].includes(job.status);
  }

  function openDeleteImportedData(job) {
    deleteJob = job;
    deleteConfirmation = '';
  }

  function closeDeleteImportedData() {
    if (isDeletingImportedData) return;
    deleteJob = null;
    deleteConfirmation = '';
  }

  async function deleteImportedData() {
    if (!deleteJob || !canConfirmDeleteImportData) return;
    isDeletingImportedData = true;
    const result = await jiraImport.deleteImportedData(deleteJob.id, {
      confirm_job_id: deleteConfirmation.trim(),
      confirm_workspace_count: deleteJob.imported_workspace_count || 0,
      confirm_delete_imported_data: true
    });
    isDeletingImportedData = false;
    if (result.success) {
      addToast({ message: t('systemImport.dataDeleted'), variant: 'success' });
      closeDeleteImportedData();
    } else {
      addToast({ message: result.error, variant: 'error' });
    }
  }
</script>

<div class="space-y-8">
  <!-- Page Header -->
  <PageHeader title={t('systemImport.title')} subtitle={t('systemImport.subtitle')} icon={Cloud}>
    {#snippet actions()}
      <!-- shortcut-guard-exempt: the configured systemImport.add hotkey and its visible hint are declared on this button -->
      <Button dataTestid="jira-import-new" variant="primary" onclick={() => openWizard()} keyboardHint={getShortcutDisplay('systemImport', 'add')} hotkeyConfig={{ key: toHotkeyString('systemImport', 'add'), guard: () => !showWizard }}>
        <Plus size={16} class="mr-2" />
        {t('systemImport.newImport')}
      </Button>
    {/snippet}
  </PageHeader>

  <!-- Saved Connections Section -->
  <div data-testid="jira-import-connections" class="rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
    <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <div class="flex items-center gap-2">
        <Link size={18} style="color: var(--ds-text-subtle);" />
        <h2 class="text-lg font-medium" style="color: var(--ds-text);">{t('systemImport.savedConnections')}</h2>
      </div>
      <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
        {t('systemImport.savedConnectionsDescription')}
      </p>
    </div>

    <div class="p-6">
      {#if savedConnections.isLoading}
        <div class="flex items-center justify-center py-8">
          <Spinner size="md" />
        </div>
      {:else if savedConnections.error}
        <AlertBox variant="error" message={savedConnections.error} />
      {:else if savedConnections.items.length === 0}
        <div class="text-center py-8">
          <Cloud class="w-12 h-12 mx-auto opacity-50" style="color: var(--ds-text-subtle);" />
          <p class="mt-4 text-sm" style="color: var(--ds-text-subtle);">
            {t('systemImport.noConnections')}
          </p>
        </div>
      {:else}
        <div class="space-y-3">
          {#each savedConnections.items as connection}
            <div data-testid={`jira-import-connection-${connection.id}`} class="p-4 rounded-lg border flex items-center justify-between"
                 style="border-color: var(--ds-border); background: var(--ds-surface);">
              <div class="flex items-center gap-4">
                <div class="w-10 h-10 rounded-lg flex items-center justify-center"
                     style="background: var(--ds-background-accent-blue-subtler);">
                  <Cloud class="w-5 h-5" style="color: var(--ds-text-accent-blue);" />
                </div>
                <div>
                  <div class="flex items-center gap-2">
                    <span class="font-medium" style="color: var(--ds-text);">
                      {connection.instance_name || 'Jira Cloud'}
                    </span>
                    <a href={safeHref(connection.instance_url)}
                       target="_blank"
                       rel="noopener noreferrer"
                       class="hover:opacity-70">
                      <ExternalLink size={14} style="color: var(--ds-text-subtle);" />
                    </a>
                  </div>
                  <div class="flex items-center gap-3 mt-1">
                    <span class="text-xs" style="color: var(--ds-text-subtle);">
                      {connection.email}
                    </span>
                    {#if connection.last_used_at}
                      <span class="text-xs" style="color: var(--ds-text-subtle);">
                        {t('systemImport.lastUsed')}: {formatDate(connection.last_used_at)}
                      </span>
                    {/if}
                  </div>
                </div>
              </div>
              <div class="flex items-center gap-2">
                <Button dataTestid="jira-import-connection-start" variant="secondary" size="small" onclick={() => openWizard(connection.id)}>
                  <PlayCircle size={14} class="mr-1" />
                  {t('jiraImport.buttons.startImport')}
                </Button>
                <Button variant="danger-ghost" size="small" icon={Trash2} title={t('systemImport.deleteConnection')} onclick={() => deleteConnection(connection.id)}></Button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>

  <!-- Import History Section -->
  <div data-testid="jira-import-history" class="rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
    <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <div class="flex items-center gap-2">
        <Clock size={18} style="color: var(--ds-text-subtle);" />
        <h2 class="text-lg font-medium" style="color: var(--ds-text);">{t('systemImport.importHistory')}</h2>
      </div>
      <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
        {t('systemImport.importHistoryDescription')}
      </p>
    </div>

    <div class="p-6">
      {#if importJobs.isLoading}
        <div class="flex items-center justify-center py-8">
          <Spinner size="md" />
        </div>
      {:else if importJobs.error}
        <AlertBox variant="error" message={importJobs.error} />
      {:else if importJobs.items.length === 0}
        <div class="text-center py-8">
          <Clock class="w-12 h-12 mx-auto opacity-50" style="color: var(--ds-text-subtle);" />
          <p class="mt-4 text-sm" style="color: var(--ds-text-subtle);">
            {t('systemImport.noImports')}
          </p>
        </div>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full">
            <thead>
              <tr style="border-bottom: 1px solid var(--ds-border);">
                <th class="text-left py-3 px-4 text-xs font-semibold tracking-wide"
                    style="color: var(--ds-text);">{t('common.status')}</th>
                <th class="text-left py-3 px-4 text-xs font-semibold tracking-wide"
                    style="color: var(--ds-text);">{t('systemImport.instance')}</th>
                <th class="text-left py-3 px-4 text-xs font-semibold tracking-wide"
                    style="color: var(--ds-text);">{t('systemImport.scope')}</th>
                <th class="text-left py-3 px-4 text-xs font-semibold tracking-wide"
                    style="color: var(--ds-text);">{t('systemImport.started')}</th>
                <th class="text-left py-3 px-4 text-xs font-semibold tracking-wide"
                    style="color: var(--ds-text);">{t('systemImport.completed')}</th>
                <th class="text-left py-3 px-4 text-xs font-semibold tracking-wide"
                    style="color: var(--ds-text);">{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {#each importJobs.items as job}
                {@const StatusIcon = getStatusIcon(job.status)}
                <tr
                  data-testid="jira-import-history-row"
                  data-job-id={job.id}
                  data-imported-comments={job.progress?.imported_comments || 0}
                  data-imported-attachments={job.progress?.imported_attachments || 0}
                  style="border-bottom: 1px solid var(--ds-border);"
                >
                  <td class="py-3 px-4">
                    <div class="flex items-center gap-2">
                      <StatusIcon size={16} style="color: {getStatusColor(job.status)};"
                                  class={job.status === 'running' ? 'animate-spin' : ''} />
                      <span data-testid="jira-import-history-status" class="text-sm capitalize" style="color: {getStatusColor(job.status)};">
                        {statusLabel(job.status)}
                      </span>
                    </div>
                    {#if job.phase && job.status === 'running'}
                      <span class="text-xs mt-1 block" style="color: var(--ds-text-subtle);">
                        {phaseLabel(job.phase)}
                      </span>
                    {/if}
                    {#if job.error_message}
                      <span class="text-xs mt-1 block" style="color: var(--ds-text-danger);">
                        {job.error_message}
                      </span>
                    {/if}
                  </td>
                  <td class="py-3 px-4">
                    <span class="text-sm" style="color: var(--ds-text);">
                      {job.instance_name || job.instance_url || '-'}
                    </span>
                  </td>
                  <td class="py-3 px-4">
                    <div class="flex flex-col gap-1">
                      <span class="text-xs px-2 py-1 rounded capitalize w-fit"
                            style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                        {scopeLabel(job.scope)}
                      </span>
                      {#if job.project_keys?.length}
                        <span class="text-xs" style="color: var(--ds-text-subtle);">
                          {t('systemImport.projects')}: {job.project_keys.join(', ')}
                        </span>
                      {/if}
                      <span data-testid="jira-import-history-counts" class="text-xs" style="color: var(--ds-text-subtle);">
                        {t('systemImport.workspaceCount', { count: job.imported_workspace_count || 0 })},
                        {t('systemImport.itemCount', { count: job.imported_item_count || 0 })}
                      </span>
                    </div>
                  </td>
                  <td class="py-3 px-4">
                    <span class="text-sm" style="color: var(--ds-text-subtle);">
                      {formatDate(job.started_at)}
                    </span>
                  </td>
                  <td class="py-3 px-4">
                    <span class="text-sm" style="color: var(--ds-text-subtle);">
                      {formatDate(job.completed_at)}
                    </span>
                  </td>
                  <td class="py-3 px-4">
                    {#if canDeleteImportedData(job)}
                      <Button variant="danger-ghost" size="small" icon={Trash2} onclick={() => openDeleteImportedData(job)}>
                        {t('systemImport.deleteData')}
                      </Button>
                    {:else if job.status === 'data_deleted'}
                      <span class="text-xs" style="color: var(--ds-text-subtle);">{t('common.deleted')}</span>
                    {:else}
                      <span class="text-xs" style="color: var(--ds-text-subtle);">{t('systemImport.unavailable')}</span>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  </div>
</div>

{#if deleteJob}
  <Modal
    isOpen={!!deleteJob}
    preventClose={isDeletingImportedData}
    maxWidth="max-w-2xl"
    onclose={closeDeleteImportedData}
    onSubmit={deleteImportedData}
    submitDisabled={!canConfirmDeleteImportData || isDeletingImportedData}
  >
    <ModalHeader title={t('systemImport.deleteImportedTitle')} showCloseButton={!isDeletingImportedData} />
    <div class="px-6 py-5 space-y-4">
      <div class="flex gap-3 rounded-lg border p-4" style="border-color: var(--ds-border-warning); background: var(--ds-background-warning-subtle);">
        <AlertTriangle class="w-5 h-5 flex-shrink-0" style="color: var(--ds-text-warning);" />
        <div class="space-y-2">
          <p class="font-medium" style="color: var(--ds-text);">{t('systemImport.deleteWarningTitle')}</p>
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            {t('systemImport.deleteWarningDescription')}
          </p>
        </div>
      </div>

      <div class="rounded-lg border p-4 space-y-3" style="border-color: var(--ds-border); background: var(--ds-surface);">
        <div class="grid grid-cols-2 gap-4 text-sm">
          <div>
            <p class="text-xs font-medium uppercase" style="color: var(--ds-text-subtle);">{t('systemImport.jobId')}</p>
            <p class="font-mono" style="color: var(--ds-text);">{deleteJob.id}</p>
          </div>
          <div>
            <p class="text-xs font-medium uppercase" style="color: var(--ds-text-subtle);">{t('systemImport.importedScope')}</p>
            <p style="color: var(--ds-text);">
              {t('systemImport.workspaceCount', { count: deleteJob.imported_workspace_count || 0 })},
              {t('systemImport.itemCount', { count: deleteJob.imported_item_count || 0 })}
            </p>
          </div>
        </div>

        {#if deleteJob.imported_workspaces?.length}
          <div>
            <p class="text-xs font-medium uppercase mb-2" style="color: var(--ds-text-subtle);">{t('systemImport.workspacesToRemove')}</p>
            <div class="flex flex-wrap gap-2">
              {#each deleteJob.imported_workspaces as workspace}
                <span class="text-xs px-2 py-1 rounded border" style="border-color: var(--ds-border); color: var(--ds-text); background: var(--ds-surface-raised);">
                  {workspace.key} — {workspace.name}
                </span>
              {/each}
            </div>
          </div>
        {/if}
      </div>

      <div class="space-y-2">
        <label for="delete-import-confirmation" class="text-sm font-medium" style="color: var(--ds-text);">
          {t('systemImport.typeJobId')}
        </label>
        <Input
          id="delete-import-confirmation"
          bind:value={deleteConfirmation}
          placeholder={deleteJob.id}
          disabled={isDeletingImportedData}
        />
      </div>
    </div>
    <DialogFooter
      onCancel={closeDeleteImportedData}
      onConfirm={deleteImportedData}
      confirmLabel={t('systemImport.deleteImportedData')}
      variant="danger"
      loading={isDeletingImportedData}
      confirmDisabled={!canConfirmDeleteImportData}
      showKeyboardHint={true}
    />
  </Modal>
{/if}

<!-- Import Wizard Modal -->
<JiraImportWizard
  bind:isOpen={showWizard}
  onClose={closeWizard}
  onComplete={closeWizard}
/>
