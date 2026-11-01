<script>
  import { t } from '../../stores/i18n.svelte.js';
  import Checkbox from '../../components/Checkbox.svelte';
  import Input from '../../components/Input.svelte';
  import Label from '../../components/Label.svelte';
  import FormField from '../../components/FormField.svelte';
  import NativeSelect from '../../components/NativeSelect.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import CustomFieldRenderer from '../items/CustomFieldRenderer.svelte';
  import { getFieldLabel, parseFormOptions } from './formModel.js';

  let {
    fields = [],
    customFieldDefinitions = [],
    currentStep = 1,
    formData = $bindable({ title: '', description: '' }),
    customFieldValues = $bindable({}),
    isDarkMode = false,
    idPrefix = 'form',
  } = $props();

  let currentStepFields = $derived(
    fields.filter((field) => (field.step_number || 1) === currentStep)
  );

  function getCustomFieldDefinition(fieldId) {
    return customFieldDefinitions.find((field) => field.id.toString() === fieldId);
  }
</script>

<div class="space-y-4" data-testid={`${idPrefix}-fields`}>
  {#if currentStepFields.some((field) => field.field_identifier === 'title')}
    {@const titleField = currentStepFields.find((field) => field.field_identifier === 'title')}
    <FormField
      label={titleField.display_name || t('requestForm.title')}
      id={`${idPrefix}-title`}
      required
      helper={titleField.description}
    >
      <Input
        id={`${idPrefix}-title`}
        bind:value={formData.title}
        placeholder={t('requestForm.enterTitle')}
        required={true}
      />
    </FormField>
  {/if}

  {#if currentStepFields.some((field) => field.field_identifier === 'description')}
    {@const descriptionField = currentStepFields.find((field) => field.field_identifier === 'description')}
    <div>
      <Label for={`${idPrefix}-description`} required={descriptionField.is_required} class="mb-1.5" color="default">
        {descriptionField.display_name || t('requestForm.description')}
      </Label>
      <Textarea
        id={`${idPrefix}-description`}
        bind:value={formData.description}
        rows={4}
        placeholder={t('requestForm.describeRequest')}
        required={descriptionField.is_required}
      />
      {#if descriptionField.description}
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">{descriptionField.description}</p>
      {/if}
    </div>
  {/if}

  {#each currentStepFields.filter((field) => field.field_type === 'custom') as field}
    {@const fieldDefinition = getCustomFieldDefinition(field.field_identifier)}
    {#if fieldDefinition}
      <div>
        <Label required={field.is_required} class="mb-1.5" color="default">
          {field.display_name || fieldDefinition.name}
        </Label>
        <CustomFieldRenderer
          field={{ ...fieldDefinition, is_required: field.is_required }}
          value={customFieldValues[field.field_identifier]}
          onChange={(value) => {
            customFieldValues[field.field_identifier] = value;
          }}
          readonly={false}
          required={field.is_required}
          milestones={[]}
          {isDarkMode}
        />
        {#if field.description}
          <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">{field.description}</p>
        {/if}
      </div>
    {/if}
  {/each}

  {#each currentStepFields.filter((field) => field.field_type === 'virtual') as field}
    {@const fieldId = `${idPrefix}-${field.field_identifier}`}
    <div>
      {#if field.virtual_field_type === 'checkbox'}
        <Checkbox
          id={fieldId}
          bind:checked={customFieldValues[field.field_identifier]}
          label={getFieldLabel(field)}
        />
      {:else}
        <Label for={fieldId} required={field.is_required} class="mb-1.5" color="default">
          {getFieldLabel(field)}
        </Label>
        {#if field.virtual_field_type === 'textarea'}
          <Textarea
            id={fieldId}
            bind:value={customFieldValues[field.field_identifier]}
            rows={3}
            required={field.is_required}
          />
        {:else if field.virtual_field_type === 'select'}
          <NativeSelect
            id={fieldId}
            bind:value={customFieldValues[field.field_identifier]}
            options={parseFormOptions(field.virtual_field_options)}
            placeholder={t('requestForm.selectOption')}
            required={field.is_required}
          />
        {:else}
          <Input
            id={fieldId}
            bind:value={customFieldValues[field.field_identifier]}
            required={field.is_required}
          />
        {/if}
      {/if}
      {#if field.description}
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">{field.description}</p>
      {/if}
    </div>
  {/each}
</div>
