<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../../router.js';
  import { api } from '../../api.js';
  import { ArrowLeft, Play, Edit2, Trash2 } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import ConfirmDialog from '../../dialogs/ConfirmDialog.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import SectionHeader from '../../layout/SectionHeader.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let template = null;
  let executions = [];
  let testSet = null;
  let loading = true;
  let editMode = false;
  let editName = '';
  let editDescription = '';

  // Dialog state
  let showConfirmDialog = false;
  let confirmMessage = '';
  let confirmTitle = '';
  let confirmAction = null;

  $: workspaceId = $currentRoute.params.id;
  $: templateId = $currentRoute.params.templateId;

  function testPath(suffix = '') {
    const base = workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces';
    return `${base}${suffix}`;
  }

  onMount(async () => {
    if (templateId) {
      await loadTemplate(templateId);
    }
  });

  async function loadTemplate(templateId) {
    try {
      loading = true;
      template = await api.tests.testRunTemplates.get(workspaceId, templateId);
      executions = await api.tests.testRunTemplates.getExecutions(workspaceId, templateId);

      if (template.set_id) {
        testSet = await api.tests.testSets.get(workspaceId, template.set_id);
      }
    } catch (error) {
      console.error('Failed to load template:', error);
    } finally {
      loading = false;
    }
  }

  function goBack() {
    navigate(testPath('/templates'));
  }

  function toggleEditMode() {
    if (!editMode) {
      editName = template.name;
      editDescription = template.description || '';
      editMode = true;
    } else {
      editMode = false;
    }
  }

  async function saveEdit() {
    if (!editName.trim()) {
      confirmTitle = t('validation.required');
      confirmMessage = t('testing.templateNameRequired');
      confirmAction = null;
      showConfirmDialog = true;
      return;
    }

    try {
      await api.tests.testRunTemplates.update(workspaceId, templateId, {
        set_id: template.set_id,
        name: editName,
        description: editDescription
      });

      template.name = editName;
      template.description = editDescription;
      editMode = false;
    } catch (error) {
      console.error('Failed to update template:', error);
      confirmTitle = t('common.error');
      confirmMessage = t('testing.failedToUpdateTemplate');
      confirmAction = null;
      showConfirmDialog = true;
    }
  }

  async function deleteTemplate() {
    confirmTitle = t('testing.deleteTemplate');
    confirmMessage = t('testing.deleteTemplateConfirm', { name: template.name });
    confirmAction = async () => {
      try {
        await api.tests.testRunTemplates.delete(workspaceId, templateId);
        navigate(testPath('/templates'));
      } catch (error) {
        console.error('Failed to delete template:', error);
        confirmTitle = t('common.error');
        confirmMessage = t('testing.failedToDeleteTemplate');
        confirmAction = null;
        showConfirmDialog = true;
      }
    };
    showConfirmDialog = true;
  }

  async function executeTemplate() {
    try {
      const newRun = await api.tests.testRunTemplates.execute(workspaceId, templateId);
      // Navigate to the execution page
      navigate(testPath(`/runs/${newRun.id}/execute`));
    } catch (error) {
      console.error('Failed to execute template:', error);
      confirmTitle = t('common.error');
      confirmMessage = t('testing.failedToStartExecution');
      confirmAction = null;
      showConfirmDialog = true;
    }
  }

  function viewRunDetails(run) {
    navigate(testPath(`/runs/${run.id}`));
  }

  function continueExecution(execution) {
    navigate(testPath(`/runs/${execution.id}/execute`));
  }

  function getRunStatus(run) {
    if (run.ended_at) {
      return { text: t('testing.completed'), style: 'background: var(--ds-status-success-bg); color: var(--ds-status-success-text);' };
    }
    return { text: t('testing.inProgress'), style: 'background: var(--ds-status-info-bg); color: var(--ds-status-info-text);' };
  }

  // Keyboard shortcuts
  function handleEditKeydown(event) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      saveEdit();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      toggleEditMode();
    }
  }
</script>

<div class="min-h-screen flex flex-col p-6" style="background-color: var(--ds-surface-raised);">
  <div class="flex-1 -mx-6 -mb-6 px-10 py-6">
    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Spinner />
      </div>
    {:else if template}
      <!-- Header -->
      <div class="flex items-center justify-between mb-6">
        <div class="flex items-center gap-3">
          <button
            onclick={goBack}
            class="p-2 rounded cursor-pointer"
            onmouseenter={(e) => e.target.style.background = 'var(--ds-surface-hovered)'}
            onmouseleave={(e) => e.target.style.background = ''}
          >
            <ArrowLeft class="w-5 h-5" />
          </button>
          <div class="flex-1">
            {#if editMode}
              <input
                type="text"
                bind:value={editName}
                onkeydown={handleEditKeydown}
                class="text-2xl font-semibold px-2 py-1 border rounded w-full"
                style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
                autofocus
              />
            {:else}
              <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">
                {template.name}
              </h1>
            {/if}
            <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">
              Created: {new Date(template.created_at).toLocaleString()}
              {#if template.updated_at && template.updated_at !== template.created_at}
                • Updated: {new Date(template.updated_at).toLocaleString()}
              {/if}
            </div>
          </div>
        </div>

        <div class="flex items-center gap-3">
          {#if editMode}
            <Button
              variant="primary"
              onclick={saveEdit}
            >
              {t('common.save')}
            </Button>
            <Button
              variant="default"
              onclick={toggleEditMode}
            >
              {t('common.cancel')}
            </Button>
          {:else}
            <Button
              variant="default"
              onclick={toggleEditMode}
              icon={Edit2}
            >
              {t('common.edit')}
            </Button>
            <Button
              variant="danger"
              onclick={deleteTemplate}
              icon={Trash2}
            >
              {t('common.delete')}
            </Button>
            <Button
              variant="primary"
              onclick={executeTemplate}
              icon={Play}
              size="medium"
            >
              {t('testing.executeTemplate')}
            </Button>
          {/if}
        </div>
      </div>

      <!-- Template Details -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <!-- Main Content -->
        <div class="lg:col-span-2 space-y-6">
          <!-- Template Information -->
          <div class="p-6" style="background-color: var(--ds-surface-raised);">
            <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">{t('testing.templateInformation')}</h2>

            <div class="space-y-4">
              <div>
                <div class="text-sm font-medium mb-1" style="color: var(--ds-text-subtle);">{t('testing.testPlan')}</div>
                {#if testSet}
                  <a href={`/workspaces/${workspaceId}/tests/sets/${testSet.id}`} class="hover:underline" style="color: var(--ds-text-link);">
                    {testSet.name}
                  </a>
                {:else}
                  <div style="color: var(--ds-text);">{t('common.loading')}</div>
                {/if}
              </div>

              <div>
                <div class="text-sm font-medium mb-1" style="color: var(--ds-text-subtle);">{t('common.description')}</div>
                {#if editMode}
                  <Textarea
                    bind:value={editDescription}
                    onkeydown={handleEditKeydown}
                    rows={4}
                    placeholder={t('testing.templateDescriptionPlaceholder')}
                  />
                {:else}
                  <div class="text-sm" style="color: var(--ds-text);">
                    {template.description || t('testing.noDescription')}
                  </div>
                {/if}
              </div>
            </div>
          </div>

          <!-- Executions List -->
          <div class="p-6" style="background-color: var(--ds-surface-raised);">
            <SectionHeader title={t('testing.executionsCount', { count: executions.length })}>
              {#snippet actions()}
                <Button
                  variant="primary"
                  onclick={executeTemplate}
                  icon={Play}
                  size="small"
                >
                  {t('testing.newExecution')}
                </Button>
              {/snippet}
            </SectionHeader>

            {#if executions.length > 0}
              <div class="space-y-3">
                {#each executions as execution}
                  {@const status = getRunStatus(execution)}
                  <div class="border rounded p-4 transition" style="border-color: var(--ds-border);" onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-surface-hovered)'} onmouseleave={(e) => e.currentTarget.style.background = ''}>
                    <div class="flex items-center justify-between">
                      <div class="flex-1">
                        <div class="font-medium mb-1" style="color: var(--ds-text);">
                          {execution.name}
                        </div>
                        <div class="text-sm" style="color: var(--ds-text-subtle);">
                          {t('testing.started')}: {new Date(execution.started_at).toLocaleString()}
                          {#if execution.ended_at}
                            • {t('testing.ended')}: {new Date(execution.ended_at).toLocaleString()}
                          {/if}
                        </div>
                      </div>
                      <div class="flex items-center gap-3">
                        <span class="px-2 py-1 text-xs font-semibold rounded-full" style={status.style}>
                          {status.text}
                        </span>
                        <div class="flex gap-2">
                          {#if !execution.ended_at}
                            <button
                              onclick={() => continueExecution(execution)}
                              class="cursor-pointer text-sm font-medium"
                              style="color: var(--ds-text-success);"
                            >
                              {t('common.continue')}
                            </button>
                          {/if}
                          <button
                            onclick={() => viewRunDetails(execution)}
                            class="cursor-pointer text-sm"
                            style="color: var(--ds-text-link);"
                          >
                            {execution.ended_at ? t('testing.results') : t('testing.progress')}
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                {/each}
              </div>
            {:else}
              <div class="text-center py-8">
                <div class="text-6xl mb-4">🚀</div>
                <div class="text-lg font-medium mb-2" style="color: var(--ds-text);">{t('testing.noExecutionsYet')}</div>
                <div class="text-sm mb-4" style="color: var(--ds-text-subtle);">
                  {t('testing.clickExecuteTemplate')}
                </div>
                <Button
                  variant="primary"
                  onclick={executeTemplate}
                  icon={Play}
                  size="medium"
                >
                  {t('testing.executeTemplate')}
                </Button>
              </div>
            {/if}
          </div>
        </div>

        <!-- Sidebar -->
        <div class="space-y-6">
          <!-- Quick Stats -->
          <div class="p-6" style="background-color: var(--ds-surface-raised);">
            <h3 class="font-semibold mb-4" style="color: var(--ds-text);">{t('testing.quickStats')}</h3>

            <div class="space-y-3">
              <div class="flex justify-between">
                <span class="text-sm" style="color: var(--ds-text-subtle);">{t('testing.totalExecutions')}</span>
                <span class="text-sm font-medium" style="color: var(--ds-text);">{executions.length}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-sm" style="color: var(--ds-text-subtle);">{t('testing.completed')}</span>
                <span class="text-sm font-medium" style="color: var(--ds-text-success);">
                  {executions.filter(e => e.ended_at).length}
                </span>
              </div>
              <div class="flex justify-between">
                <span class="text-sm" style="color: var(--ds-text-subtle);">{t('testing.inProgress')}</span>
                <span class="text-sm font-medium" style="color: var(--ds-text-info);">
                  {executions.filter(e => !e.ended_at).length}
                </span>
              </div>
            </div>
          </div>

          <!-- Test Set Info -->
          {#if testSet}
            <div class="p-6" style="background-color: var(--ds-surface-raised);">
              <h3 class="font-semibold mb-4" style="color: var(--ds-text);">{t('testing.testPlanDetails')}</h3>

              <div class="space-y-3">
                <div>
                  <div class="text-sm font-medium" style="color: var(--ds-text-subtle);">{t('common.name')}</div>
                  <a href={`/workspaces/${workspaceId}/tests/sets/${testSet.id}`} class="text-sm hover:underline" style="color: var(--ds-text-link);">
                    {testSet.name}
                  </a>
                </div>
                {#if testSet.description}
                  <div>
                    <div class="text-sm font-medium" style="color: var(--ds-text-subtle);">{t('common.description')}</div>
                    <div class="text-sm" style="color: var(--ds-text);">
                      {testSet.description}
                    </div>
                  </div>
                {/if}
              </div>
            </div>
          {/if}
        </div>
      </div>
    {:else}
      <div class="text-center py-12">
        <div style="color: var(--ds-text-subtle);">{t('testing.templateNotFound')}</div>
        <div class="mt-4">
          <Button
            variant="primary"
            onclick={goBack}
          >
            {t('testing.backToTemplates')}
          </Button>
        </div>
      </div>
    {/if}
  </div>
</div>

<!-- Confirm Dialog -->
<ConfirmDialog
  bind:show={showConfirmDialog}
  title={confirmTitle}
  message={confirmMessage}
  confirmText={confirmAction ? "Confirm" : "OK"}
  cancelText={confirmAction ? "Cancel" : ""}
  variant={confirmAction ? "danger" : "info"}
  onconfirm={() => {
    if (confirmAction) {
      confirmAction();
    }
    showConfirmDialog = false;
  }}
  oncancel={() => showConfirmDialog = false}
/>
