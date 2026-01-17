<script>
  import { ArrowLeft, Plus, Edit, Trash2, ClipboardList, X } from 'lucide-svelte';
  import { api } from '../../api.js';
  import MilkdownEditor from '../../editors/MilkdownEditor.svelte';
  import { navigate, currentRoute } from '../../router.js';
  import ConfirmDialog from '../../dialogs/ConfirmDialog.svelte';
  import Button from '../../components/Button.svelte';
  import Label from '../../components/Label.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import { createShortcutHandler } from '../../utils/keyboardShortcuts.js';
  import { t } from '../../stores/i18n.svelte.js';

  let { workspaceId = null } = $props();

  let testCase = $state(null);
  let testSteps = $state([]);
  let editingStep = $state(null);
  let showStepForm = $state(false);
  let showDeleteConfirmation = $state(false);
  let stepToDelete = $state(null);
  let loading = $state(true);
  let error = $state(null);
  let loadingTestCaseId = $state(null); // Guard to prevent duplicate loads
  let showImagePreview = $state(false);
  let previewImage = $state({ src: '', alt: '' });

  let stepFormData = $state({
    action: '',
    data: '',
    expected: ''
  });

  // DataTable columns definition
  const columns = $derived([
    { key: 'step_number', label: t('testing.step'), width: '70px', align: 'text-center', slot: 'step_number' },
    { key: 'action', label: t('testing.action'), slot: 'step_action' },
    { key: 'data', label: t('testing.data'), slot: 'step_data' },
    { key: 'expected', label: t('testing.expectedResult'), slot: 'step_expected' },
    { key: 'actions', label: '' }
  ]);

  // Get testCaseId from route params
  let testCaseId = $derived($currentRoute.params?.testId ? parseInt($currentRoute.params.testId) : null);

  // Load data when testCaseId changes
  $effect(() => {
    const currentTestCaseId = testCaseId;
    if (currentTestCaseId && currentTestCaseId !== loadingTestCaseId) {
      loadData(currentTestCaseId);
    }
  });

  // Global keyboard shortcut handler using centralized system
  // Note: handler is created as a function that returns a new handler each render
  // to ensure it captures the current showStepForm state
  function handleGlobalKeydown(event) {
    createShortcutHandler({
      addStep: showAddStepForm
    }, 'testSteps', { guard: () => !showStepForm })(event);
  }


  async function loadData(id) {
    if (!id) return;

    loadingTestCaseId = id;
    loading = true;
    error = null;

    try {
      await loadTestCase(id);
      await loadTestSteps(id);
    } catch (err) {
      console.error('Failed to load data:', err);
      error = t('testing.failedToLoadTests');
    } finally {
      loading = false;
    }
  }

  async function loadTestCase(id) {
    try {
      testCase = await api.tests.testCases.get(workspaceId, id);
    } catch (err) {
      console.error('Failed to load test case:', err);
      throw err;
    }
  }

  async function loadTestSteps(id = testCaseId) {
    try {
      testSteps = await api.tests.testCases.steps.getAll(workspaceId, id) || [];
    } catch (err) {
      console.error('Failed to load test steps:', err);
      throw err;
    }
  }

  function goBack() {
    navigate(getTestBasePath());
  }

  function getTestBasePath() {
    return workspaceId ? `/workspaces/${workspaceId}/tests` : '/workspaces';
  }

  function showAddStepForm() {
    showStepForm = true;
    editingStep = null;
    stepFormData = { action: '', data: '', expected: '' };
    // Focus the first input after DOM update (allow time for MilkdownEditor to initialize)
    setTimeout(() => {
      const firstEditor = document.querySelector('#step-action-input .ProseMirror');
      firstEditor?.focus();
    }, 100);
  }

  function showEditStepForm(step) {
    showStepForm = true;
    editingStep = step;
    stepFormData = {
      action: step.action,
      data: step.data,
      expected: step.expected
    };
    // Focus the first input after DOM update (allow time for MilkdownEditor to initialize)
    setTimeout(() => {
      const firstEditor = document.querySelector('#step-action-input .ProseMirror');
      firstEditor?.focus();
    }, 100);
  }

  function cancelStepForm() {
    showStepForm = false;
    editingStep = null;
    stepFormData = { action: '', data: '', expected: '' };
  }

  async function handleStepSubmit() {
    if (!stepFormData.action.trim()) return;
    
    try {
      if (editingStep) {
        await api.tests.testCases.steps.update(workspaceId, testCaseId, editingStep.id, stepFormData);
      } else {
        await api.tests.testCases.steps.create(workspaceId, testCaseId, stepFormData);
      }
      
      await loadTestSteps();
      cancelStepForm();
    } catch (error) {
      console.error('Failed to save test step:', error);
      alert(t('testing.failedToSaveStep') + ': ' + (error.message || error));
    }
  }

  function deleteTestStep(stepId) {
    stepToDelete = stepId;
    showDeleteConfirmation = true;
  }

  async function confirmDeleteStep() {
    try {
      await api.tests.testCases.steps.delete(workspaceId, testCaseId, stepToDelete);
      await loadTestSteps();
    } catch (error) {
      console.error('Failed to delete test step:', error);
    } finally {
      stepToDelete = null;
    }
  }

  // Build dropdown action items for each step
  function buildStepDropdownItems(step) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        onClick: () => showEditStepForm(step)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        onClick: () => deleteTestStep(step.id)
      }
    ];
  }

  function handleRenderedContentClick(event) {
    const img = event.target?.closest('img');
    if (!img) return;

    event.preventDefault();
    previewImage = {
      src: img.src,
      alt: img.alt || ''
    };
    showImagePreview = true;
  }

  function closePreview() {
    showImagePreview = false;
    previewImage = { src: '', alt: '' };
  }

  function handleFormKeydown(event) {
    if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
      event.preventDefault();
      handleStepSubmit();
    }
  }
</script>

<svelte:window onkeydown={handleGlobalKeydown} />

<!-- Header -->
<div class="p-6 pb-0">
  {#if loading}
    <div class="flex items-center justify-center py-12">
      <Spinner size="lg" />
    </div>
  {:else if error}
    <div class="text-center py-12">
      <div class="text-lg font-medium mb-2" style="color: var(--ds-text-danger);">{t('common.error')}</div>
      <div class="text-sm" style="color: var(--ds-text-subtle);">{error}</div>
    </div>
  {:else if testCase}
    <div class="flex items-start justify-between gap-4 mb-6">
      <div>
        <h2 class="text-lg font-semibold" style="color: var(--ds-text);">
          {t('testing.testStepsFor', { title: testCase.title })}
        </h2>
        {#if testCase.preconditions}
          <div class="text-sm mt-3 px-4 py-3 rounded border-l-4"
               style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle); border-left-color: var(--ds-status-info-solid);">
            <strong style="color: var(--ds-text);">{t('testing.preconditions')}:</strong> {testCase.preconditions}
          </div>
        {/if}
      </div>
      <div class="flex items-center gap-3">
        <Button
          onclick={goBack}
          icon={ArrowLeft}
        >
          {t('testing.backToTestCases')}
        </Button>
        {#if !showStepForm}
          <Button
            variant="primary"
            onclick={showAddStepForm}
            icon={Plus}
            size="medium"
            keyboardHint="A"
          >
            {t('testing.addTestStep')}
          </Button>
        {/if}
      </div>
    </div>
  {/if}
</div>

{#if !loading && !error && testCase}
  <!-- Content -->
  <div class="p-6">
    <!-- Add Step Form (if showing) -->
    {#if showStepForm}
      <div class="test-step-form mb-6 p-5 rounded-xl border shadow-sm" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
        <h4 class="text-lg font-medium mb-4" style="color: var(--ds-text);">
          {editingStep ? t('testing.editTestStep') : t('testing.addTestStep')}
        </h4>
        <form onsubmit={(e) => { e.preventDefault(); handleStepSubmit(); }} onkeydown={handleFormKeydown}>
          <div class="grid grid-cols-3 gap-4">
            <!-- Action Column -->
            <div>
              <Label color="default" class="mb-2" required>{t('testing.action')}</Label>
              <div id="step-action-input" class="border rounded overflow-hidden" style="border-color: var(--ds-border); min-height: 80px;">
                <MilkdownEditor
                  bind:content={stepFormData.action}
                  placeholder={t('testing.actionPlaceholder')}
                  showToolbar={true}
                  entityType="test_case"
                  entityId={testCaseId}
                />
              </div>
            </div>

            <!-- Data Column -->
            <div>
              <Label color="default" class="mb-2">{t('testing.data')}</Label>
              <div class="border rounded overflow-hidden" style="border-color: var(--ds-border); min-height: 80px;">
                <MilkdownEditor
                  bind:content={stepFormData.data}
                  placeholder={t('testing.dataPlaceholder')}
                  showToolbar={true}
                  entityType="test_case"
                  entityId={testCaseId}
                />
              </div>
            </div>

            <!-- Expected Result Column -->
            <div>
              <Label color="default" class="mb-2" required>{t('testing.expectedResult')}</Label>
              <div class="border rounded overflow-hidden" style="border-color: var(--ds-border); min-height: 80px;">
                <MilkdownEditor
                  bind:content={stepFormData.expected}
                  placeholder={t('testing.expectedPlaceholder')}
                  showToolbar={true}
                  entityType="test_case"
                  entityId={testCaseId}
                />
              </div>
            </div>
          </div>

          <div class="flex gap-2 justify-end mt-4">
            <Button
              type="button"
              variant="default"
              onclick={cancelStepForm}
              size="medium"
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="submit"
              variant="primary"
              disabled={!stepFormData.action.trim() || !stepFormData.expected.trim()}
              size="medium"
              keyboardHint="⌘ ↵"
            >
              {editingStep ? t('testing.updateStep') : t('testing.addStep')}
            </Button>
          </div>
        </form>
      </div>
    {/if}

    <!-- Test Steps List -->
    <DataTable
      {columns}
      data={testSteps}
      keyField="id"
      emptyMessage={t('testing.noTestStepsYet')}
      emptyDescription={t('testing.addFirstTestStep')}
      emptyIcon={ClipboardList}
      actionItems={buildStepDropdownItems}
    >
      <div slot="step_number" let:item={step}>
        <span style="color: var(--ds-text-link); font-weight: 500;">
          {step.step_number || (testSteps.findIndex(s => s.id === step.id) + 1)}
        </span>
      </div>

      <div slot="step_action" let:item={step}>
        <div class="text-sm prose-sm max-w-none test-step-rendered" onclick={handleRenderedContentClick}>
          <MilkdownEditor content={step.action || ''} readonly={true} showToolbar={false} />
        </div>
      </div>

      <div slot="step_data" let:item={step}>
        <div class="text-sm prose-sm max-w-none test-step-rendered" onclick={handleRenderedContentClick}>
          {#if step.data}
            <MilkdownEditor content={step.data} readonly={true} showToolbar={false} />
          {:else}
            <span style="color: var(--ds-text-subtle);">—</span>
          {/if}
        </div>
      </div>

      <div slot="step_expected" let:item={step}>
        <div class="text-sm prose-sm max-w-none test-step-rendered" onclick={handleRenderedContentClick}>
          <MilkdownEditor content={step.expected || ''} readonly={true} showToolbar={false} />
        </div>
      </div>
    </DataTable>

    <!-- Steps Summary -->
    {#if testSteps && testSteps.length > 0}
      <div class="mt-4 text-sm" style="color: var(--ds-text-subtle);">
        {t('testing.testStepsConfigured', { count: testSteps.length })}
      </div>
    {/if}
  </div>
{/if}

<!-- Delete Step Confirmation Dialog -->
<ConfirmDialog
  bind:show={showDeleteConfirmation}
  title={t('testing.deleteTestStep')}
  message={t('testing.deleteStepConfirm')}
  confirmText={t('testing.deleteStep')}
  cancelText={t('common.cancel')}
  variant="danger"
  onconfirm={confirmDeleteStep}
  oncancel={() => {
    showDeleteConfirmation = false;
    stepToDelete = null;
  }}
/>

{#if showImagePreview && previewImage.src}
  <div class="image-lightbox-backdrop" onclick={closePreview}>
    <div class="image-lightbox" onclick={(e) => e.stopPropagation()}>
      <Button class="lightbox-close" variant="ghost" icon={X} onclick={closePreview} title={t('testing.closeImagePreview')} />
      <img src={previewImage.src} alt={previewImage.alt} />
      {#if previewImage.alt}
        <div class="lightbox-caption">{previewImage.alt}</div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .attachment-item:hover {
    transform: translateY(-1px);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  }

  :global(.test-step-rendered img) {
    max-width: 300px;
    width: 100%;
    height: auto;
    cursor: pointer;
    border-radius: 6px;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.12);
  }

  :global(.test-step-rendered p) {
    margin: 0.25rem 0;
  }

  .image-lightbox-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.75);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 50;
    padding: 24px;
  }

  .image-lightbox {
    position: relative;
    background: var(--ds-surface-raised);
    padding: 16px;
    border-radius: 8px;
    max-width: 90vw;
    max-height: 90vh;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
  }

  .image-lightbox img {
    max-width: 85vw;
    max-height: 80vh;
    object-fit: contain;
    border-radius: 6px;
  }

  .lightbox-caption {
    font-size: 14px;
    color: var(--ds-text-subtle);
  }

  .lightbox-close {
    position: absolute;
    top: 8px;
    right: 8px;
    border: none;
    background: var(--ds-background-neutral);
    color: var(--ds-text);
    width: 28px;
    height: 28px;
    border-radius: 50%;
    font-size: 18px;
    cursor: pointer;
    line-height: 1;
  }

  .lightbox-close:hover {
    background: var(--ds-background-neutral-hovered);
  }
</style>
