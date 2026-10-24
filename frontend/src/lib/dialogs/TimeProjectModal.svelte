<script>
  import FormModal from './FormModal.svelte';
  import Label from '../components/Label.svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { isNoCustomer } from '../utils/customerLabel.js';
  import DescriptionText from '../components/DescriptionText.svelte';
	import TextField from '../components/TextField.svelte';
	import TextareaField from '../components/TextareaField.svelte';
	import SelectField from '../components/SelectField.svelte';

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
      <TextField
        label={t('timeProject.projectName')}
        id="time-project-name"
        required
        bind:value={formData.name}
      />
    </div>

    <div>
      <SelectField
        label={t('timeProject.status')}
        options={statusOptions.map(status => ({ value: status, label: status }))}
        bind:value={formData.status}
      />
    </div>

    <div>
      <SelectField
        label={t('timeProject.customerOptional')}
        options={[{ value: '', label: t('time.noCustomer') }, ...customers.filter(c => c.active && !isNoCustomer(c)).map(customer => ({ value: customer.id, label: customer.name }))]}
        bind:value={formData.customer_id}
      />
    </div>

    <div>
      <SelectField
        label={t('timeProject.categoryOptional')}
        options={[{ value: '', label: t('timeProject.none') }, ...categories.map(category => ({ value: category.id, label: category.name }))]}
        bind:value={formData.category_id}
      />
    </div>
  </div>

  <div class="mt-6">
    <TextField
      label={t('timeProject.hourlyRate')}
      type="number"
      min="0"
      step="0.01"
      bind:value={formData.hourly_rate}
    />
  </div>

  <div class="mt-6">
    <TextField
      label={t('timeProject.maxHours')}
      type="number"
      min="0"
      step="0.5"
      placeholder={t('timeProject.maxHoursPlaceholder')}
      bind:value={formData.settings.max_hours}
    />
    <DescriptionText as="div">
      {t('timeProject.maxHoursHint')}
    </DescriptionText>
  </div>

  <!-- Color Picker -->
  <div class="mt-6">
    <div class="flex items-center gap-3 mb-2">
      <Label>{t('timeProject.projectColor')}</Label>
      <IconSelector bind:selectedColor={formData.color} colorOnly compact />
      {#if formData.color}
        <button
          onclick={() => formData.color = ''}
          class="text-xs px-2 py-0.5 rounded hover-bg transition-colors"
          style="color: var(--ds-text-subtle);"
          type="button"
        >
          {t('common.clear')}
        </button>
      {/if}
    </div>
  </div>

  <div class="mt-6">
    <TextareaField
      label={t('common.description')}
      id="time-project-description"
      rows={3}
      bind:value={formData.description}
    />
  </div>
</FormModal>
