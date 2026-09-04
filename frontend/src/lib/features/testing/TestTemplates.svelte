<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { writable } from 'svelte/store';
  import { navigate } from '../../router.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { IconFiles } from '@tabler/icons-svelte-runes';
  import { escapeHtml } from '../../utils/sanitize.ts';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import FormField from '../../components/FormField.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import TestManagementHeader from './TestManagementHeader.svelte';

  let { workspaceId = null } = $props();

  const testSets = writable([]);
  const testTemplates = writable([]);
  const milestones = writable([]);

  let showForm = $state(false);
  let selectedSetId = $state('');
  let templateName = $state('');
  let templateDescription = $state('');

  function testPath(suffix = '') {
    const base = workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces';
    return `${base}${suffix}`;
  }

  // Filtering
  let selectedMilestoneFilter = $state(null);

  const workspaceTestBase = $derived.by(() => workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces');
  const templateColumns = $derived.by(() => [
    {
      key: 'name',
      label: t('testing.templateName'),
      html: true,
      render: (template) => `<a href="${workspaceTestBase}/templates/${template.id}" style="color: var(--ds-text-link);" class="hover:underline">${escapeHtml(template.name)}</a>`
    },
    {
      key: 'testSetName',
      label: t('testing.testPlan'),
      html: true,
      render: (template) => `<a href="${workspaceTestBase}/sets?milestone=${template.milestoneId || ''}" style="color: var(--ds-text-link);" class="hover:underline">${escapeHtml(template.testSetName)}</a>`
    },
    {
      key: 'milestoneName',
      label: t('milestones.milestone'),
      html: true,
      render: (template) => template.milestoneId
        ? `<a href="/milestones" style="color: var(--ds-text-link);" class="hover:underline">${escapeHtml(template.milestoneName)}</a>`
        : `<span style="color: var(--ds-text-subtle);">${t('testing.noMilestone')}</span>`
    },
    { key: 'description', label: t('common.description'), render: (template) => template.description || '-' },
    {
      key: 'created_at',
      label: t('common.created'),
      render: (template) => template.created_at ? new Date(template.created_at).toLocaleString() : '-'
    },
    { key: 'actions', label: t('common.actions'), width: 'w-20', align: 'text-right' }
  ]);

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    try {
      const [sets, templates, milestonesData] = await Promise.all([
        api.tests.testPlans.getAll(workspaceId),
        api.tests.testRunTemplates.getAll(workspaceId),
        api.milestones.getAll()
      ]);

      testSets.set(sets || []);
      testTemplates.set(templates || []);
      milestones.set(milestonesData || []);
    } catch (error) {
      console.error('Failed to load data:', error);
    }
  }

  function showAddForm() {
    showForm = true;
    selectedSetId = '';
    templateName = '';
    templateDescription = '';
    // Focus the first input after the form is rendered
    setTimeout(() => {
      const firstInput = document.getElementById('set-select');
      if (firstInput) firstInput.focus();
    }, 100);
  }

  async function createTemplate() {
    if (!selectedSetId || !templateName) {
      errorToast(t('testing.selectPlanAndName'));
      return;
    }

    try {
      await api.tests.testRunTemplates.create(workspaceId, {
        plan_id: parseInt(selectedSetId),
        name: templateName,
        description: templateDescription || ''
      });
      await loadData();
      showForm = false;
    } catch (error) {
      console.error('Failed to create test template:', error);
      errorToast(t('testing.failedToCreateTemplate'));
    }
  }

  async function deleteTemplate(template) {
    const ok = await confirm({
      title: t('testing.deleteTemplate'),
      message: t('testing.deleteTemplateConfirm', { name: template.name }),
      confirmText: t('common.confirm'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await api.tests.testRunTemplates.delete(workspaceId, template.id);
      await loadData();
    } catch (error) {
      console.error('Failed to delete template:', error);
      errorToast(t('testing.failedToDeleteTemplate'));
    }
  }

  async function executeTemplate(template) {
    try {
      const newRun = await api.tests.testRunTemplates.execute(workspaceId, template.id);
      // Navigate to the execution page
      navigate(testPath(`/runs/${newRun.id}/execute`));
    } catch (error) {
      console.error('Failed to execute template:', error);
      errorToast(t('testing.failedToStartExecution'));
    }
  }

  // Keyboard shortcuts for forms
  function handleFormKeydown(event) {
    if (event.key === 'Enter') {
      event.preventDefault();
      createTemplate();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      showForm = false;
    }
  }

  function viewTemplateDetails(template) {
    navigate(testPath(`/templates/${template.id}`));
  }

  function templateActions(template) {
    return [
      {
        id: 'execute',
        title: t('testing.execute'),
        color: 'var(--ds-status-success-text)',
        onClick: () => executeTemplate(template)
      },
      {
        id: 'view',
        title: t('testing.viewDetails'),
        onClick: () => viewTemplateDetails(template)
      },
      {
        id: 'delete',
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        onClick: () => deleteTemplate(template)
      }
    ];
  }

  // Computed property for filtered test sets
  const filteredTestSets = $derived.by(() => selectedMilestoneFilter
    ? $testSets.filter(set => set.milestone_id === selectedMilestoneFilter)
    : $testSets);

  // Enrich templates with test set and milestone info
  const enrichedTemplates = $derived.by(() => $testTemplates.map(template => {
    const set = $testSets.find(s => s.id === template.plan_id);
    const milestone = set ? $milestones.find(m => m.id === set.milestone_id) : null;
    return {
      ...template,
      testSetName: set?.name || 'Unknown',
      testSetId: template.plan_id,
      milestoneName: milestone?.name || 'No milestone',
      milestoneId: set?.milestone_id
    };
  }));

  // Filter templates by milestone
  const filteredTemplates = $derived.by(() => selectedMilestoneFilter
    ? enrichedTemplates.filter(t => t.milestoneId === selectedMilestoneFilter)
    : enrichedTemplates);

</script>

<div class="min-h-screen flex flex-col p-6" style="background-color: var(--ds-surface);">
  <TestManagementHeader
    title={t('testing.testRunTemplates')}
    subtitle={t('testing.testRunTemplatesSubtitle')}
    bind:milestoneFilter={selectedMilestoneFilter}
    oncreate={showAddForm}
  >
    {#snippet primaryAction()}
      <Button
        onclick={showAddForm}
        variant="primary"
        size="medium"
        keyboardHint="A"
      >
        {t('testing.createTemplate')}
      </Button>
    {/snippet}
  </TestManagementHeader>

  <Modal
    bind:isOpen={showForm}
    maxWidth="max-w-2xl"
    onclose={() => showForm = false}
  >
    <form onsubmit={(e) => { e.preventDefault(); createTemplate(); }}>
      <ModalHeader title={t('testing.createTestRunTemplate')} showCloseButton={false} />
      <div class="p-6 pb-2">
        <FormField id="set-select" label={t('testing.selectTestPlan')}>
          <Select id="set-select" bind:value={selectedSetId} options={[{ value: '', label: t('testing.selectTestPlanPlaceholder') }, ...filteredTestSets.map(set => ({ value: set.id, label: set.name }))]} />
        </FormField>
        <FormField id="template-name" label={t('testing.templateName')}>
          <Input
            id="template-name"
            bind:value={templateName}
            placeholder={t('testing.templateNamePlaceholder')}
          />
        </FormField>
        <FormField id="template-description" label={t('testing.descriptionOptional')}>
          <Textarea
            id="template-description"
            bind:value={templateDescription}
            placeholder={t('testing.templateDescriptionPlaceholder')}
            rows={3}
          />
        </FormField>
      </div>
      <DialogFooter
        cancelLabel={t('common.cancel')}
        confirmLabel={t('testing.createTemplate')}
        onCancel={() => showForm = false}
        onConfirm={createTemplate}
        disabled={!selectedSetId || !templateName.trim()}
        showKeyboardHint={true}
      />
    </form>
  </Modal>

  <!-- Content wrapper -->
  <div class="flex-1">
    <DataTable
      columns={templateColumns}
      data={filteredTemplates}
      keyField="id"
      actionItems={templateActions}
      emptyMessage={t('testing.noTemplatesYet')}
      emptyDescription={t('testing.createTemplatesHint')}
      emptyIcon={IconFiles}
    />
  </div>
</div>
