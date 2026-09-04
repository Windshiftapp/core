<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../../router.js';
  import { api } from '../../api.js';
  import { IconArrowLeft, IconPlayerPlay, IconEdit, IconTrash } from '@tabler/icons-svelte-runes';
  import Button from '../../components/Button.svelte';
  import { confirm } from '../../composables/useConfirm.js';
  import EmptyState from '../../components/EmptyState.svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Card from '../../components/Card.svelte';
  import Panel from '../../components/Panel.svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import SectionHeader from '../../layout/SectionHeader.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatAuthenticatedDateTime } from '../../utils/authenticatedDateFormatter.js';

  let template = $state(null);
  let executions = $state([]);
  let testSet = $state(null);
  let loading = $state(true);
  let editMode = $state(false);
  let editName = $state('');
  let editDescription = $state('');

  let workspaceId = $derived($currentRoute.params.id);
  let templateId = $derived($currentRoute.params.templateId);

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

      if (template.plan_id) {
        testSet = await api.tests.testPlans.get(workspaceId, template.plan_id);
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
      await confirm({
        title: t('validation.required'),
        message: t('testing.templateNameRequired'),
        confirmText: t('common.ok'),
        cancelText: '',
        variant: 'info',
      });
      return;
    }

    try {
      await api.tests.testRunTemplates.update(workspaceId, templateId, {
        plan_id: template.plan_id,
        name: editName,
        description: editDescription
      });

      template.name = editName;
      template.description = editDescription;
      editMode = false;
    } catch (error) {
      console.error('Failed to update template:', error);
      await confirm({
        title: t('common.error'),
        message: t('testing.failedToUpdateTemplate'),
        confirmText: t('common.ok'),
        cancelText: '',
        variant: 'danger',
      });
    }
  }

  async function deleteTemplate() {
    const ok = await confirm({
      title: t('testing.deleteTemplate'),
      message: t('testing.deleteTemplateConfirm', { name: template.name }),
      confirmText: 'Confirm',
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await api.tests.testRunTemplates.delete(workspaceId, templateId);
      navigate(testPath('/templates'));
    } catch (error) {
      console.error('Failed to delete template:', error);
      await confirm({
        title: t('common.error'),
        message: t('testing.failedToDeleteTemplate'),
        confirmText: t('common.ok'),
        cancelText: '',
        variant: 'danger',
      });
    }
  }

  async function executeTemplate() {
    try {
      const newRun = await api.tests.testRunTemplates.execute(workspaceId, templateId);
      navigate(testPath(`/runs/${newRun.id}/execute`));
    } catch (error) {
      console.error('Failed to execute template:', error);
      await confirm({
        title: t('common.error'),
        message: t('testing.failedToStartExecution'),
        confirmText: t('common.ok'),
        cancelText: '',
        variant: 'danger',
      });
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
      return { text: t('testing.completed'), color: 'green' };
    }
    return { text: t('testing.inProgress'), color: 'blue' };
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

<div class="min-h-screen flex flex-col p-6" style="background-color: var(--ds-surface);">
  <div class="flex-1">
    {#if loading}
      <StateDisplay type="loading" message={t('common.loading')} size="lg" />
    {:else if template}
      <!-- Header -->
      <Button onclick={goBack} variant="ghost" size="small" icon={IconArrowLeft} class="mb-4">
        {t('testing.testRunTemplates')}
      </Button>
      {#if editMode}
        <div class="flex items-center justify-between gap-4 mb-8">
          <div class="flex-1 max-w-2xl">
            <!-- svelte-ignore a11y_autofocus -->
            <Input
              type="text"
              bind:value={editName}
              onkeydown={handleEditKeydown}
              class="text-xl font-medium"
              autofocus
            />
          </div>
          <div class="flex items-center gap-3">
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
          </div>
        </div>
      {:else}
        <PageHeader
          title={template.name}
          subtitle={`Created: ${formatAuthenticatedDateTime(template.created_at)}${template.updated_at && template.updated_at !== template.created_at ? ` • Updated: ${formatAuthenticatedDateTime(template.updated_at)}` : ''}`}
        >
          {#snippet actions()}
          <div class="flex flex-wrap items-center gap-3">
            <Button
              variant="default"
              onclick={toggleEditMode}
              icon={IconEdit}
            >
              {t('common.edit')}
            </Button>
            <Button
              variant="danger"
              onclick={deleteTemplate}
              icon={IconTrash}
            >
              {t('common.delete')}
            </Button>
            <Button
              variant="primary"
              onclick={executeTemplate}
              icon={IconPlayerPlay}
              size="medium"
            >
              {t('testing.executeTemplate')}
            </Button>
          </div>
          {/snippet}
        </PageHeader>
      {/if}

      <!-- Template Details -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <!-- Main Content -->
        <div class="lg:col-span-2 space-y-6">
          <!-- Template Information -->
          <Card variant="raised" padding="spacious" shadow>
            <SectionHeader title={t('testing.templateInformation')} />

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
          </Card>

          <!-- Executions List -->
          <Card variant="raised" padding="spacious" shadow>
            <SectionHeader title={t('testing.executionsCount', { count: executions.length })}>
              {#snippet actions()}
                <Button
                  variant="primary"
                  onclick={executeTemplate}
                  icon={IconPlayerPlay}
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
                  <Panel padding="default" hoverable>
                    <div class="flex items-center justify-between">
                      <div class="flex-1">
                        <div class="font-medium mb-1" style="color: var(--ds-text);">
                          {execution.name}
                        </div>
                        <div class="text-sm" style="color: var(--ds-text-subtle);">
                          {t('testing.started')}: {formatAuthenticatedDateTime(execution.started_at)}
                          {#if execution.ended_at}
                            • {t('testing.ended')}: {formatAuthenticatedDateTime(execution.ended_at)}
                          {/if}
                        </div>
                      </div>
                      <div class="flex items-center gap-3">
                        <Lozenge color={status.color} text={status.text} />
                        <div class="flex gap-2">
                          {#if !execution.ended_at}
                            <Button
                              onclick={() => continueExecution(execution)}
                              variant="link"
                              size="small"
                            >
                              {t('common.continue')}
                            </Button>
                          {/if}
                          <Button
                            onclick={() => viewRunDetails(execution)}
                            variant="link"
                            size="small"
                          >
                            {execution.ended_at ? t('testing.results') : t('testing.progress')}
                          </Button>
                        </div>
                      </div>
                    </div>
                  </Panel>
                {/each}
              </div>
            {:else}
              <EmptyState icon={IconPlayerPlay} title={t('testing.noExecutionsYet')} description={t('testing.clickExecuteTemplate')}>
                {#snippet action()}
                  <Button variant="primary" onclick={executeTemplate} icon={IconPlayerPlay} size="medium">
                    {t('testing.executeTemplate')}
                  </Button>
                {/snippet}
              </EmptyState>
            {/if}
          </Card>
        </div>

        <!-- Sidebar -->
        <div class="space-y-6">
          <!-- Quick Stats -->
          <Card variant="raised" padding="spacious" shadow>
            <SectionHeader title={t('testing.quickStats')} />

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
          </Card>

          <!-- Test Set Info -->
          {#if testSet}
            <Card variant="raised" padding="spacious" shadow>
              <SectionHeader title={t('testing.testPlanDetails')} />

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
            </Card>
          {/if}
        </div>
      </div>
    {:else}
      <EmptyState title={t('testing.templateNotFound')}>
        {#snippet action()}
          <Button variant="primary" onclick={goBack}>
            {t('testing.backToTemplates')}
          </Button>
        {/snippet}
      </EmptyState>
    {/if}
  </div>
</div>
