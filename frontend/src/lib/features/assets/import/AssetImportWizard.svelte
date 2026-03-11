<script>
  import { assetImportStore } from './AssetImportStore.svelte.js';
  import Modal from '../../../dialogs/Modal.svelte';
  import ModalHeader from '../../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../../dialogs/DialogFooter.svelte';
  import Button from '../../../components/Button.svelte';
  import Stepper from '../../../components/Stepper.svelte';
  import { addToast } from '../../../stores/toasts.svelte.js';
  import { FileSpreadsheet, ChevronLeft } from 'lucide-svelte';

  import UploadStep from './UploadStep.svelte';
  import MappingStep from './MappingStep.svelte';
  import PreviewStep from './PreviewStep.svelte';
  import ImportStep from './ImportStep.svelte';

  let {
    isOpen = $bindable(false),
    setId,
    onComplete = () => {},
    onClose = () => {},
  } = $props();

  let wizard = $derived(assetImportStore.wizard);
  let upload = $derived(assetImportStore.upload);
  let importData = $derived(assetImportStore.import);
  let currentStep = $derived(wizard.currentStep);
  let steps = $derived(wizard.steps);
  let currentStepId = $derived(steps[currentStep]?.id || 'upload');

  // Load metadata when modal opens
  $effect(() => {
    if (isOpen && setId) {
      assetImportStore.setTarget(setId);
    }
  });

  function handleClose() {
    assetImportStore.reset();
    isOpen = false;
    onClose();
  }

  function handleComplete() {
    onComplete();
    handleClose();
  }

  async function handleNext() {
    if (currentStepId === 'upload') {
      if (!upload.uploadId) {
        addToast({ message: 'Please upload a CSV file first', variant: 'warning' });
        return;
      }
      if (!assetImportStore.target.assetTypeId) {
        addToast({ message: 'Please select an asset type', variant: 'warning' });
        return;
      }
      assetImportStore.nextStep();
    } else if (currentStepId === 'mapping') {
      if (!assetImportStore.isMappingValid()) {
        addToast({ message: 'Please map at least the Title field', variant: 'warning' });
        return;
      }
      assetImportStore.nextStep();
    } else if (currentStepId === 'preview') {
      const result = await assetImportStore.startImport();
      if (!result?.success) {
        addToast({ message: result?.error || 'Failed to start import', variant: 'error' });
      }
    } else if (currentStepId === 'import') {
      handleComplete();
    }
  }

  function getConfirmLabel() {
    if (currentStepId === 'upload') return 'Continue';
    if (currentStepId === 'mapping') return 'Continue';
    if (currentStepId === 'preview') return 'Start Import';
    if (currentStepId === 'import') {
      if (importData.result) return 'Done';
      return null;
    }
    return 'Continue';
  }

  let confirmLabel = $derived(getConfirmLabel());

  let isLoading = $derived(
    upload.isUploading || importData.isImporting
  );

  let isDisabled = $derived(
    isLoading ||
    (currentStepId === 'upload' && (!upload.uploadId || !assetImportStore.target.assetTypeId)) ||
    (currentStepId === 'mapping' && !assetImportStore.isMappingValid()) ||
    (currentStepId === 'import' && !importData.result && !importData.error)
  );
</script>

<Modal bind:isOpen maxWidth="max-w-4xl" onclose={handleClose}>
  <div class="flex flex-col max-h-[90vh]">
    <ModalHeader
      title="Import Assets from CSV"
      subtitle={upload.fileName || 'Upload a CSV file to bulk import assets'}
      icon={FileSpreadsheet}
      onClose={handleClose}
    />

    <!-- Step indicator -->
    <div class="px-6 w-full py-3 border-b overflow-x-auto" style="border-color: var(--ds-border);">
      <Stepper
        {steps}
        currentStep={currentStep + 1}
        showLabels={true}
        size="small"
        getLabel={(step) => step.label}
      />
    </div>

    <!-- Content area -->
    <div class="p-6 overflow-y-auto flex-1 min-h-0">
      {#if currentStepId === 'upload'}
        <UploadStep />
      {:else if currentStepId === 'mapping'}
        <MappingStep />
      {:else if currentStepId === 'preview'}
        <PreviewStep />
      {:else if currentStepId === 'import'}
        <ImportStep />
      {/if}
    </div>

    <!-- Footer -->
    <DialogFooter
      showCancel={false}
      confirmLabel={confirmLabel}
      loading={isLoading}
      disabled={isDisabled}
      onConfirm={confirmLabel ? handleNext : null}
    >
      {#snippet extra()}
        <Button
          variant="ghost"
          onclick={() => currentStep > 0 ? assetImportStore.prevStep() : handleClose()}
          disabled={isLoading || (currentStepId === 'import' && !importData.result && !importData.error)}
        >
          {#if currentStep === 0}
            Cancel
          {:else}
            <ChevronLeft size={16} class="mr-1" />
            Back
          {/if}
        </Button>
      {/snippet}
    </DialogFooter>
  </div>
</Modal>
