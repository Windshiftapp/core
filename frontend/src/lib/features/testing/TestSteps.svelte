<script>
  import { IconArrowLeft, IconPlus, IconEdit, IconTrash, IconClipboardList, IconX } from '@tabler/icons-svelte-runes';
  import { api } from '../../api.js';
  import MilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
  import { navigate, currentRoute } from '../../router.js';
  import { confirm } from '../../composables/useConfirm.js';
  import Button from '../../components/Button.svelte';
  import Label from '../../components/Label.svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import AlertBox from '../../components/AlertBox.svelte';
  import Card from '../../components/Card.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import { toHotkeyString } from '../../utils/keyboardShortcuts.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';

  let { workspaceId = null } = $props();

  let testCase = $state(null);
  let testSteps = $state([]);
  let editingStep = $state(null);
  let showStepForm = $state(false);
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
      const firstEditor = /** @type {HTMLElement | null} */ (document.querySelector('#step-action-input .ProseMirror'));
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
      const firstEditor = /** @type {HTMLElement | null} */ (document.querySelector('#step-action-input .ProseMirror'));
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
      errorToast(t('testing.failedToSaveStep') + ': ' + (error.message || error));
    }
  }

  async function deleteTestStep(stepId) {
    const ok = await confirm({
      title: t('testing.deleteTestStep'),
      message: t('testing.deleteStepConfirm'),
      confirmText: t('testing.deleteStep'),
      variant: 'danger',
    });
    if (!ok) return;
    try {
      await api.tests.testCases.steps.delete(workspaceId, testCaseId, stepId);
      await loadTestSteps();
    } catch (error) {
      console.error('Failed to delete test step:', error);
    }
  }

  // Build dropdown action items for each step
  function buildStepDropdownItems(step) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: IconEdit,
        title: t('common.edit'),
        onClick: () => showEditStepForm(step)
      },
      {
        id: 'delete',
        type: 'regular',
        icon: IconTrash,
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

<!-- Header -->
<div class="p-6 pb-0">
  {#if loading}
    <StateDisplay type="loading" message={t('common.loading')} size="lg" />
  {:else if error}
    <StateDisplay type="error" title={t('common.error')} message={error} size="lg" />
  {:else if testCase}
    <PageHeader title={t('testing.testStepsFor', { title: testCase.title })} marginClass="mb-4">
      {#snippet actions()}
        <div class="flex items-center gap-3">
          <Button onclick={goBack} variant="ghost" icon={IconArrowLeft} dataTestid="test-steps-back">
            {t('testing.backToTestCases')}
          </Button>
          {#if !showStepForm}
            <Button
              variant="primary"
              onclick={showAddStepForm}
              icon={IconPlus}
              size="medium"
              keyboardHint="A"
              hotkeyConfig={{ key: toHotkeyString('testSteps', 'addStep'), guard: () => !showStepForm }}
              dataTestid="test-step-create-button"
            >
              {t('testing.addTestStep')}
            </Button>
          {/if}
        </div>
      {/snippet}
    </PageHeader>
    {#if testCase.preconditions}
      <AlertBox variant="info" class="mb-6">
        <strong>{t('testing.preconditions')}:</strong> {testCase.preconditions}
      </AlertBox>
    {/if}
  {/if}
</div>

{#if !loading && !error && testCase}
  <!-- Content -->
  <div class="p-6">
    <!-- Add Step Form (if showing) -->
    {#if showStepForm}
      <Card variant="raised" padding="spacious" shadow class="test-step-form mb-6">
        <h4 class="text-lg font-medium mb-4" style="color: var(--ds-text);">
          {editingStep ? t('testing.editTestStep') : t('testing.addTestStep')}
        </h4>
        <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <form onsubmit={(e) => { e.preventDefault(); handleStepSubmit(); }} onkeydown={handleFormKeydown}>
          <div class="grid grid-cols-3 gap-4">
            <!-- Action Column -->
            <div>
              <Label color="default" class="mb-2" required>{t('testing.action')}</Label>
              <div id="step-action-input" data-testid="test-step-action" class="border rounded overflow-hidden" style="border-color: var(--ds-border); min-height: 80px;">
                <MilkdownEditor
                  bind:content={stepFormData.action}
                  testId="test-step-action-editor"
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
              <div data-testid="test-step-data" class="border rounded overflow-hidden" style="border-color: var(--ds-border); min-height: 80px;">
                <MilkdownEditor
                  bind:content={stepFormData.data}
                  testId="test-step-data-editor"
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
              <div data-testid="test-step-expected" class="border rounded overflow-hidden" style="border-color: var(--ds-border); min-height: 80px;">
                <MilkdownEditor
                  bind:content={stepFormData.expected}
                  testId="test-step-expected-editor"
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
              dataTestid="test-step-submit"
            >
              {editingStep ? t('testing.updateStep') : t('testing.addStep')}
            </Button>
          </div>
        </form>
      </Card>
    {/if}

    <!-- Test Steps List -->
    <DataTable
      {columns}
      data={testSteps}
      keyField="id"
      emptyMessage={t('testing.noTestStepsYet')}
      emptyDescription={t('testing.addFirstTestStep')}
      emptyIcon={IconClipboardList}
      actionItems={buildStepDropdownItems}
      rowAttrs={(step) => ({ 'data-testid': `test-step-row-${step.id}` })}
    >
      {#snippet step_number(step)}
        <span style="color: var(--ds-text-link); font-weight: 500;">
          {step.step_number || (testSteps.findIndex(s => s.id === step.id) + 1)}
        </span>
      {/snippet}

      {#snippet step_action(step)}
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="text-sm prose-sm max-w-none test-step-rendered" onclick={handleRenderedContentClick}>
          <MilkdownEditor content={step.action || ''} readonly={true} showToolbar={false} />
        </div>
      {/snippet}

      {#snippet step_data(step)}
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="text-sm prose-sm max-w-none test-step-rendered" onclick={handleRenderedContentClick}>
          {#if step.data}
            <MilkdownEditor content={step.data} readonly={true} showToolbar={false} />
          {:else}
            <span style="color: var(--ds-text-subtle);">—</span>
          {/if}
        </div>
      {/snippet}

      {#snippet step_expected(step)}
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="text-sm prose-sm max-w-none test-step-rendered" onclick={handleRenderedContentClick}>
          <MilkdownEditor content={step.expected || ''} readonly={true} showToolbar={false} />
        </div>
      {/snippet}
    </DataTable>

    <!-- Steps Summary -->
    {#if testSteps && testSteps.length > 0}
      <div class="mt-4 text-sm" style="color: var(--ds-text-subtle);">
        {t('testing.testStepsConfigured', { count: testSteps.length })}
      </div>
    {/if}
  </div>
{/if}


{#if showImagePreview && previewImage.src}
  <div class="image-lightbox-backdrop" role="button" tabindex="0" onclick={closePreview} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') closePreview(); }}>
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="image-lightbox" onclick={(e) => e.stopPropagation()}>
      <Button class="lightbox-close" variant="ghost" icon={IconX} onclick={closePreview} title={t('testing.closeImagePreview')} />
      <img src={previewImage.src} alt={previewImage.alt} />
      {#if previewImage.alt}
        <div class="lightbox-caption">{previewImage.alt}</div>
      {/if}
    </div>
  </div>
{/if}

<style>
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

</style>
