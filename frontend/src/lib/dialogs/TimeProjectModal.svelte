<script>
  import FormModal from './FormModal.svelte';
  import Input from '../components/Input.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';
  import ColorPicker from '../editors/ColorPicker.svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Props
  let {
    isOpen = false,
    formData = $bindable({
      customer_id: '',
      category_id: '',
      name: '',
      description: '',
      status: 'Active',
      color: '',
      hourly_rate: 0,
      settings: { max_hours: '' }
    }),
    customers = [],
    categories = [],
    statusOptions = ['Active', 'On Hold', 'Completed', 'Archived'],
    isEditing = false,
    onsave = () => {},
    oncancel = () => {}
  } = $props();

  function handleSave() {
    if (formData.name.trim()) {
      onsave();
    }
  }
</script>

<FormModal
  {isOpen}
  title={t('timeProject.newProject')}
  editTitle={t('timeProject.editProject')}
  {isEditing}
  onSave={handleSave}
  onCancel={oncancel}
  saveLabel={isEditing ? t('timeProject.updateProject') : t('timeProject.createProject')}
  saveDisabled={!formData.name.trim()}
  maxWidth="max-w-2xl"
>
  <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
    <div>
      <Label required class="mb-2">{t('timeProject.projectName')}</Label>
      <Input bind:value={formData.name} required />
    </div>

    <div>
      <Label class="mb-2">{t('timeProject.status')}</Label>
      <Select bind:value={formData.status}>
        {#each statusOptions as status}
          <option value={status}>{status}</option>
        {/each}
      </Select>
    </div>

    <div>
      <Label class="mb-2">{t('timeProject.customerOptional')}</Label>
      <Select bind:value={formData.customer_id}>
        <option value="">{t('timeProject.none')}</option>
        {#each customers.filter(c => c.active) as customer}
          <option value={customer.id}>{customer.name}</option>
        {/each}
      </Select>
    </div>

    <div>
      <Label class="mb-2">{t('timeProject.categoryOptional')}</Label>
      <Select bind:value={formData.category_id}>
        <option value="">{t('timeProject.none')}</option>
        {#each categories as category}
          <option value={category.id}>{category.name}</option>
        {/each}
      </Select>
    </div>
  </div>

  <div class="mt-6">
    <Label class="mb-2">{t('timeProject.hourlyRate')}</Label>
    <Input type="number" bind:value={formData.hourly_rate} min="0" step="0.01" />
  </div>

  <div class="mt-6">
    <Label class="mb-2">{t('timeProject.maxHours')}</Label>
    <Input type="number" bind:value={formData.settings.max_hours} min="0" step="0.5" placeholder={t('timeProject.maxHoursPlaceholder')} />
    <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
      {t('timeProject.maxHoursHint')}
    </div>
  </div>

  <!-- Color Picker -->
  <div class="mt-6">
    <div class="flex items-center gap-3 mb-2">
      <Label>{t('timeProject.projectColor')}</Label>
      {#if formData.color}
        <div class="flex items-center gap-2">
          <div class="w-6 h-6 rounded flex-shrink-0" style="background-color: {formData.color}; border: 1px solid var(--ds-border);"></div>
          <span class="text-xs" style="color: var(--ds-text-subtle);">{formData.color}</span>
          <button
            onclick={() => formData.color = ''}
            class="text-xs px-2 py-0.5 rounded hover-bg transition-colors"
            style="color: var(--ds-text-subtle);"
            type="button"
          >
            {t('common.clear')}
          </button>
        </div>
      {/if}
    </div>
    <ColorPicker bind:value={formData.color} />
  </div>

  <div class="mt-6">
    <Label class="mb-2">{t('common.description')}</Label>
    <Textarea bind:value={formData.description} rows={3} />
  </div>
</FormModal>
