<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { writable } from 'svelte/store';
  import { navigate } from '../../router.js';
  import ConfirmDialog from '../../dialogs/ConfirmDialog.svelte';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { FileStack } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import MilestoneCombobox from '../../pickers/MilestoneCombobox.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import Label from '../../components/Label.svelte';
  import DataTable from '../../components/DataTable.svelte';

  let { workspaceId = null } = $props();

  const testSets = writable([]);
  const testTemplates = writable([]);
  const milestones = writable([]);

  let showForm = $state(false);
  let selectedSetId = $state('');
  let templateName = $state('');
  let templateDescription = $state('');

  // Error/Confirm dialog state
  let showConfirmDialog = $state(false);
  let confirmMessage = $state('');
  let confirmTitle = $state('');
  let confirmAction = $state(null);

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
      label: 'Template Name',
      html: true,
      render: (template) => `<a href="${workspaceTestBase}/templates/${template.id}" style="color: var(--ds-text-link);" class="hover:underline">${template.name}</a>`
    },
    {
      key: 'testSetName',
      label: 'Test Plan',
      html: true,
      render: (template) => `<a href="${workspaceTestBase}/sets?milestone=${template.milestoneId || ''}" style="color: var(--ds-text-link);" class="hover:underline">${template.testSetName}</a>`
    },
    {
      key: 'milestoneName',
      label: 'Milestone',
      html: true,
      render: (template) => template.milestoneId
        ? `<a href="/milestones" style="color: var(--ds-text-link);" class="hover:underline">${template.milestoneName}</a>`
        : `<span style="color: var(--ds-text-subtle);">No milestone</span>`
    },
    { key: 'description', label: 'Description', render: (template) => template.description || '-' },
    {
      key: 'created_at',
      label: 'Created',
      render: (template) => template.created_at ? new Date(template.created_at).toLocaleString() : '-'
    },
    { key: 'actions', label: 'Actions', width: 'w-20', align: 'text-right' }
  ]);

  onMount(async () => {
    await loadData();

    // Check for URL parameters
    const urlParams = new URLSearchParams(window.location.search);
    const milestoneParam = urlParams.get('milestone');
    if (milestoneParam) {
      selectedMilestoneFilter = parseInt(milestoneParam);
    }

    // Add keyboard shortcuts
    const handleKeyDown = (e) => {
      // Only handle shortcuts when not typing in inputs or textareas
      if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') {
        return;
      }

      // 'a' key to add test template
      if (e.key === 'a' || e.key === 'A') {
        e.preventDefault();
        showAddForm();
      }
    };

    document.addEventListener('keydown', handleKeyDown);

    // Cleanup
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  });

  async function loadData() {
    try {
      const [sets, templates, milestonesData] = await Promise.all([
        api.tests.testSets.getAll(workspaceId),
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
      errorToast('Please select a test plan and enter a template name');
      return;
    }

    try {
      await api.tests.testRunTemplates.create(workspaceId, {
        set_id: parseInt(selectedSetId),
        name: templateName,
        description: templateDescription || ''
      });
      await loadData();
      showForm = false;
    } catch (error) {
      console.error('Failed to create test template:', error);
      errorToast('Failed to create template. Please try again.');
    }
  }

  async function deleteTemplate(template) {
    confirmTitle = 'Delete Template';
    confirmMessage = `Are you sure you want to delete the template "${template.name}"? This will not delete any existing test runs created from this template.`;
    confirmAction = async () => {
      try {
        await api.tests.testRunTemplates.delete(workspaceId, template.id);
        await loadData();
      } catch (error) {
        console.error('Failed to delete template:', error);
        errorToast('Failed to delete template. Please try again.');
      }
    };
    showConfirmDialog = true;
  }

  async function executeTemplate(template) {
    try {
      const newRun = await api.tests.testRunTemplates.execute(workspaceId, template.id);
      // Navigate to the execution page
      navigate(testPath(`/runs/${newRun.id}/execute`));
    } catch (error) {
      console.error('Failed to execute template:', error);
      errorToast('Failed to start execution. Please try again.');
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
        title: 'Execute',
        color: 'var(--ds-status-success-text)',
        onClick: () => executeTemplate(template)
      },
      {
        id: 'view',
        title: 'View Details',
        onClick: () => viewTemplateDetails(template)
      },
      {
        id: 'delete',
        title: 'Delete',
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
    const set = $testSets.find(s => s.id === template.set_id);
    const milestone = set ? $milestones.find(m => m.id === set.milestone_id) : null;
    return {
      ...template,
      testSetName: set?.name || 'Unknown',
      testSetId: template.set_id,
      milestoneName: milestone?.name || 'No milestone',
      milestoneId: set?.milestone_id
    };
  }));

  // Filter templates by milestone
  const filteredTemplates = $derived.by(() => selectedMilestoneFilter
    ? enrichedTemplates.filter(t => t.milestoneId === selectedMilestoneFilter)
    : enrichedTemplates);

  // Handle milestone selection and update URL
  function handleMilestoneSelect(event) {
    selectedMilestoneFilter = event.detail.value;
    updateURL();
  }

  function updateURL() {
    const url = new URL(window.location);
    if (selectedMilestoneFilter) {
      url.searchParams.set('milestone', selectedMilestoneFilter.toString());
    } else {
      url.searchParams.delete('milestone');
    }
    window.history.replaceState({}, '', url);
  }
</script>

<div class="min-h-screen flex flex-col p-6" style="background-color: var(--ds-surface-raised);">
  <PageHeader
    title="Test Run Templates"
    subtitle="Create reusable test run configurations"
  >
    {#snippet actions()}
      <div class="flex items-center gap-3">
        <div class="w-48">
          <MilestoneCombobox
            bind:value={selectedMilestoneFilter}
            placeholder="All milestones"
            onselect={handleMilestoneSelect}
          />
        </div>
        <Button
          onclick={showAddForm}
          variant="primary"
          size="medium"
          keyboardHint="A"
        >
          Create Template
        </Button>
      </div>
    {/snippet}
  </PageHeader>

  <Modal
    bind:isOpen={showForm}
    maxWidth="max-w-2xl"
    onclose={() => showForm = false}
  >
    <div class="p-6" style="background-color: var(--ds-surface-raised);">
      <h3 class="text-xl font-semibold mb-4" style="color: var(--ds-text);">Create Test Run Template</h3>
      <form class="space-y-4" onsubmit={(e) => { e.preventDefault(); createTemplate(); }}>
        <div>
          <Label for="set-select" color="default" class="mb-2">Select Test Plan</Label>
          <Select id="set-select" bind:value={selectedSetId}>
            <option value="">-- Select a test plan --</option>
            {#each filteredTestSets as set}
              <option value={set.id}>{set.name}</option>
            {/each}
          </Select>
        </div>
        <div>
          <Label for="template-name" color="default" class="mb-2">Template Name</Label>
          <Input
            id="template-name"
            bind:value={templateName}
            placeholder="e.g., Regression Tests, Smoke Tests"
          />
        </div>
        <div>
          <Label for="template-description" color="default" class="mb-2">Description (Optional)</Label>
          <Textarea
            id="template-description"
            bind:value={templateDescription}
            placeholder="Add notes about this template..."
            rows={3}
          />
        </div>
        <div class="flex justify-end gap-3 pt-2">
          <Button
            variant="outline"
            type="button"
            onclick={() => showForm = false}
            keyboardHint="Esc"
          >
            Cancel
          </Button>
          <Button
            type="submit"
            variant="primary"
            keyboardHint="↵"
          >
            Create Template
          </Button>
        </div>
      </form>
    </div>
  </Modal>

  <!-- Content wrapper -->
  <div class="flex-1 -mx-6 -mb-6 px-10 py-6">
    <DataTable
      columns={templateColumns}
      data={filteredTemplates}
      keyField="id"
      actionItems={templateActions}
      emptyMessage="No test templates yet"
      emptyDescription="Create reusable templates to speed up test run creation."
      emptyIcon={FileStack}
    />
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
