<script>
  import { Tags, X, Plus } from 'lucide-svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import Modal from '../../dialogs/Modal.svelte';
  import Input from '../../components/Input.svelte';
  import Button from '../../components/Button.svelte';
  import Label from '../../components/Label.svelte';
  import EmptyState from '../../components/EmptyState.svelte';

  let {
    isOpen = false,
    workspaceId,
    testCase = null,
    labels = [],
    assignedLabels = $bindable([]),
    onClose = () => {},
    onLabelsChanged = () => {}
  } = $props();

  let labelSearchQuery = $state('');
  let showCreateLabelForm = $state(false);
  let newLabelData = $state({
    name: '',
    color: '#3B82F6',
    description: ''
  });

  // Preset color palette
  const colorPalette = [
    '#EF4444', '#F59E0B', '#10B981', '#3B82F6', '#8B5CF6',
    '#EC4899', '#6B7280', '#DC2626', '#F97316', '#059669',
    '#0EA5E9', '#7C3AED', '#DB2777', '#4B5563'
  ];

  // Filter labels based on search query
  let filteredLabels = $derived.by(() => {
    if (!labelSearchQuery) return labels;
    return labels.filter(label =>
      label.name.toLowerCase().includes(labelSearchQuery.toLowerCase()) ||
      (label.description && label.description.toLowerCase().includes(labelSearchQuery.toLowerCase()))
    );
  });

  function isLabelAssigned(labelId) {
    return assignedLabels.some(label => label.id === labelId);
  }

  async function addLabelToTestCase(labelId) {
    try {
      await api.tests.testCases.labels.add(workspaceId, testCase.id, labelId);
      const updatedLabels = await api.tests.testCases.labels.getAll(workspaceId, testCase.id);
      assignedLabels = updatedLabels || [];
      onLabelsChanged();
    } catch (error) {
      console.error('Failed to add label to test case:', error);
    }
  }

  async function removeLabelFromTestCase(labelId) {
    try {
      await api.tests.testCases.labels.remove(workspaceId, testCase.id, labelId);
      const updatedLabels = await api.tests.testCases.labels.getAll(workspaceId, testCase.id);
      assignedLabels = updatedLabels || [];
      onLabelsChanged();
    } catch (error) {
      console.error('Failed to remove label from test case:', error);
    }
  }

  function showCreateLabelFormModal() {
    showCreateLabelForm = true;
    newLabelData = {
      name: '',
      color: '#3B82F6',
      description: ''
    };
  }

  async function handleCreateLabel() {
    try {
      await api.tests.testLabels.create(workspaceId, newLabelData);
      showCreateLabelForm = false;
      newLabelData = { name: '', color: '#3B82F6', description: '' };
      onLabelsChanged();
    } catch (error) {
      console.error('Failed to create label:', error);
    }
  }

  function handleClose() {
    labelSearchQuery = '';
    showCreateLabelForm = false;
    onClose();
  }
</script>

<Modal
  {isOpen}
  maxWidth="max-w-2xl"
  onclose={handleClose}
>
  <div class="max-h-[80vh] flex flex-col">
    <!-- Header -->
    <div class="flex items-center justify-between p-6 border-b shrink-0" style="border-color: var(--ds-border);">
      <div>
        <h3 class="text-xl font-semibold" style="color: var(--ds-text);">
          {t('testing.manageLabels')}: {testCase?.title}
        </h3>
        <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">
          {t('testing.clickLabelsToAssign')}
        </div>
      </div>
      <button
        onclick={handleClose}
        class="p-2 hover:bg-[var(--ds-background-neutral-hovered)] rounded-full transition-colors"
        aria-label={t('testing.closeLabelsModal')}
      >
        <X class="w-6 h-6" style="color: var(--ds-text-subtle);" />
      </button>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto p-6">
      <div class="space-y-4">
        <!-- Search and create new label -->
        <div class="mb-6 space-y-2">
          <Label class="block text-xs font-medium" color="subtle">
            {t('testing.searchExistingLabels')}
          </Label>
          <Input
            placeholder={t('testing.searchLabelsPlaceholder')}
            bind:value={labelSearchQuery}
            size="small"
          />
          <div class="flex items-center justify-between pt-2 text-sm" style="color: var(--ds-text-subtle);">
            <span>{t('testing.cantFindLabel')}</span>
            <Button
              variant="ghost"
              onclick={showCreateLabelFormModal}
              icon={Plus}
              size="small"
              style="color: var(--ds-interactive);"
            >
              {t('testing.newLabel')}
            </Button>
          </div>
        </div>

        <!-- Create New Label Form -->
        {#if showCreateLabelForm}
          <div class="bg-gray-50 rounded p-4 border" style="background-color: var(--ds-background-neutral); border-color: var(--ds-border);">
            <h4 class="font-medium mb-3" style="color: var(--ds-text);">{t('testing.createNewLabel')}</h4>
            <form onsubmit={(e) => { e.preventDefault(); handleCreateLabel(); }} class="space-y-3">
              <div>
                <Label class="block text-xs font-medium mb-1">{t('common.name')}</Label>
                <Input
                  bind:value={newLabelData.name}
                  required
                  placeholder={t('testing.enterLabelName')}
                  size="small"
                />
              </div>
              <div class="flex gap-3">
                <div class="flex-1">
                  <Label class="block text-xs font-medium mb-1">{t('common.color')}</Label>
                  <div class="flex items-center gap-3">
                    <!-- Color Preview Circle -->
                    <div
                      class="w-8 h-8 rounded-full border-2 flex-shrink-0"
                      style="background-color: {newLabelData.color}; border-color: var(--ds-border-bold);"
                    ></div>

                    <!-- Color Palette -->
                    <div class="flex flex-wrap gap-1.5">
                      {#each colorPalette as color}
                        <button
                          type="button"
                          onclick={() => newLabelData.color = color}
                          class="w-6 h-6 rounded-full border-2 transition-all hover:scale-110 {newLabelData.color === color ? 'ring-2' : ''}"
                          style="background-color: {color}; border-color: {newLabelData.color === color ? 'var(--ds-border-bold)' : 'var(--ds-border)'}; {newLabelData.color === color ? '--tw-ring-color: var(--ds-border);' : ''}"
                          aria-label={t('testing.selectColor', { color })}
                        ></button>
                      {/each}

                      <!-- Custom Color Input -->
                      <div class="relative">
                        <input
                          type="color"
                          bind:value={newLabelData.color}
                          class="w-6 h-6 rounded-full border-2 cursor-pointer opacity-0 absolute inset-0"
                          style="border-color: var(--ds-border);"
                          aria-label={t('testing.customColorPicker')}
                        />
                        <div class="w-6 h-6 rounded-full border-2 cursor-pointer flex items-center justify-center text-xs font-bold" style="border-color: var(--ds-border); color: var(--ds-text-subtle); background: linear-gradient(45deg, #ff0000 25%, #ffff00 25%, #ffff00 50%, #00ff00 50%, #00ff00 75%, #0000ff 75%);">
                          +
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
                <div class="flex-2">
                  <Label class="block text-xs font-medium mb-1">{t('common.description')}</Label>
                  <Input
                    bind:value={newLabelData.description}
                    placeholder={t('testing.optionalDescription')}
                    size="small"
                  />
                </div>
              </div>
              <div class="flex gap-2 pt-2">
                <Button
                  type="submit"
                  variant="primary"
                  size="small"
                >
                  {t('common.create')}
                </Button>
                <Button
                  type="button"
                  variant="default"
                  onclick={() => showCreateLabelForm = false}
                  size="small"
                >
                  {t('common.cancel')}
                </Button>
              </div>
            </form>
          </div>
        {/if}

        <!-- Labels List -->
        {#if labels && labels.length > 0}
          {#if filteredLabels.length > 0}
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {#each filteredLabels as label}
                {@const isAssigned = isLabelAssigned(label.id)}
                <button
                  onclick={() => isAssigned ? removeLabelFromTestCase(label.id) : addLabelToTestCase(label.id)}
                  class="flex items-center gap-3 p-3 border rounded transition-all hover:shadow-sm {isAssigned ? 'ring-2 ring-opacity-50' : 'hover:border-gray-300'}"
                  style="
                    border-color: {isAssigned ? label.color : 'var(--ds-border)'};
                    ring-color: {isAssigned ? label.color : 'transparent'};
                    background-color: {isAssigned ? label.color + '10' : 'var(--ds-surface)'};
                  "
                >
                  <div
                    class="w-4 h-4 rounded-full flex-shrink-0"
                    style="background-color: {label.color};"
                  ></div>
                  <div class="flex-1 text-left">
                    <div class="font-medium" style="color: var(--ds-text);">{label.name}</div>
                    {#if label.description}
                      <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">{label.description}</div>
                    {/if}
                  </div>
                  {#if isAssigned}
                    <div class="text-xs px-2 py-1 rounded" style="background: var(--ds-status-success-bg); color: var(--ds-status-success-text);">
                      {t('testing.assigned')}
                    </div>
                  {:else}
                    <div class="text-xs px-2 py-1 rounded" style="color: var(--ds-text-subtle); background-color: var(--ds-background-neutral);">
                      {t('testing.clickToAssign')}
                    </div>
                  {/if}
                </button>
              {/each}
            </div>
          {:else}
            <EmptyState
              icon={Tags}
              title={t('testing.noLabelsMatchSearch')}
              description={t('testing.adjustSearchOrCreate')}
            />
          {/if}
        {:else}
          <EmptyState
            icon={Tags}
            title={t('testing.noLabelsAvailable')}
            description={t('testing.createFirstLabel')}
          />
        {/if}
      </div>
    </div>

    <!-- Footer -->
    <div class="border-t p-4 shrink-0" style="border-color: var(--ds-border); background-color: var(--ds-background-neutral);">
      <div class="flex justify-between items-center">
        <div class="text-sm" style="color: var(--ds-text-subtle);">
          {t('testing.labelsAssigned', { count: assignedLabels.length })}
        </div>
        <Button
          onclick={handleClose}
          variant="primary"
          size="medium"
        >
          {t('common.done')}
        </Button>
      </div>
    </div>
  </div>
</Modal>
