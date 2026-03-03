<script>
  import { Calendar, Target } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { formatDateWithOptions } from '../utils/dateFormatter.js';
  import MilkdownEditor from '../editors/LazyMilkdownEditor.svelte';
  import FieldChip from '../components/FieldChip.svelte';
  import { useForm, validators } from '../composables/useForm.svelte.js';

  let {
    formData = $bindable({
      name: '',
      description: '',
      target_date: '',
      status: 'planning'
    }),
    nameInputRef = $bindable(null)
  } = $props();

  // Initialize form with useForm composable
  const form = useForm({
    initialValues: {
      name: '',
      description: '',
      target_date: '',
      status: 'planning'
    },
    schema: {
      name: validators.required('Name is required'),
      target_date: validators.required('Target date is required')
    }
  });

  // Sync form values to bindable formData for parent component compatibility
  $effect(() => {
    formData.name = form.values.name;
    formData.description = form.values.description;
    formData.target_date = form.values.target_date;
    formData.status = form.values.status;
  });

  // Status options for milestones - reactive for i18n
  const milestoneStatusOptions = $derived([
    { value: 'planning', label: t('createModal.planning') },
    { value: 'in-progress', label: t('createModal.inProgress') },
    { value: 'completed', label: t('createModal.completed') },
    { value: 'cancelled', label: t('createModal.cancelled') }
  ]);

  export function validate() {
    return form.isValid;
  }

  export function getFormData() {
    return {
      name: form.values.name,
      description: form.values.description,
      target_date: form.values.target_date,
      status: form.values.status,
      category_id: null
    };
  }

  export function reset() {
    form.reset();
  }

  export function isValid() {
    return form.isValid;
  }

  export function isDirty() {
    return form.isDirty;
  }
</script>

<div class="space-y-3">
  <!-- Title Input -->
  <input
    bind:this={nameInputRef}
    bind:value={form.values.name}
    type="text"
    class="w-full text-lg font-medium border-0 outline-none bg-transparent"
    style="color: var(--ds-text);"
    placeholder={t('createModal.milestoneName')}
    onblur={() => form.touchField('name')}
  />
  {#if form.hasError('name')}
    <span class="text-xs" style="color: var(--ds-text-danger);">{form.errors.name}</span>
  {/if}

  <!-- Description -->
  <div class="min-h-[60px]">
    <MilkdownEditor
      bind:content={form.values.description}
      placeholder={t('createModal.addDescription')}
      compact={true}
      showToolbar={false}
      readonly={false}
      itemId={null}
    />
  </div>

  <!-- Field Chips Row -->
  <div class="flex flex-wrap items-center gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
    <!-- Target Date Chip -->
    <FieldChip
      label={t('createModal.targetDate')}
      value={form.values.target_date}
      displayValue={form.values.target_date ? formatDateWithOptions(form.values.target_date, { month: 'short', day: 'numeric' }) : ''}
      icon={Calendar}
      placeholder={t('createModal.targetDate')}
      required={true}
    >
      {#snippet children({ close: closePopover })}
        <div class="p-3">
          <input
            type="date"
            bind:value={form.values.target_date}
            class="w-full px-3 py-2 rounded border text-sm"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            onchange={() => {
              form.touchField('target_date');
              closePopover();
            }}
          />
        </div>
      {/snippet}
    </FieldChip>

    <!-- Status Chip -->
    <FieldChip
      label={t('createModal.status')}
      value={form.values.status}
      displayValue={milestoneStatusOptions.find(s => s.value === form.values.status)?.label || t('createModal.planning')}
      icon={Target}
      placeholder={t('createModal.status')}
    >
      {#snippet children({ close: closePopover })}
        <div class="p-2 max-h-48 overflow-y-auto">
          {#each milestoneStatusOptions as status}
            <button
              type="button"
              class="w-full px-3 py-2 text-left text-sm rounded transition-colors"
              style="color: var(--ds-text);"
              onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
              onmouseout={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
              onfocus={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
              onblur={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
              onclick={() => {
                form.setValue('status', status.value);
                closePopover();
              }}
            >
              {status.label}
            </button>
          {/each}
        </div>
      {/snippet}
    </FieldChip>
  </div>
</div>
