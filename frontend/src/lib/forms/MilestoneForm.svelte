<script>
  import { Calendar, Target } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';
  import MilkdownEditor from '../editors/LazyMilkdownEditor.svelte';
  import FieldChip from '../components/FieldChip.svelte';

  let {
    formData = $bindable({
      name: '',
      description: '',
      target_date: '',
      status: 'planning'
    }),
    nameInputRef = $bindable(null)
  } = $props();

  // Status options for milestones - reactive for i18n
  const milestoneStatusOptions = $derived([
    { value: 'planning', label: t('createModal.planning') },
    { value: 'in-progress', label: t('createModal.inProgress') },
    { value: 'completed', label: t('createModal.completed') },
    { value: 'cancelled', label: t('createModal.cancelled') }
  ]);

  export function validate() {
    return formData.name.trim() !== '' && formData.target_date !== '';
  }

  export function getFormData() {
    return {
      name: formData.name,
      description: formData.description,
      target_date: formData.target_date,
      status: formData.status,
      category_id: null
    };
  }

  export function reset() {
    formData = {
      name: '',
      description: '',
      target_date: '',
      status: 'planning'
    };
  }

  export function isValid() {
    return formData.name.trim() !== '' && formData.target_date !== '';
  }
</script>

<div class="space-y-3">
  <!-- Title Input -->
  <input
    bind:this={nameInputRef}
    bind:value={formData.name}
    type="text"
    class="w-full text-lg font-medium border-0 outline-none bg-transparent"
    style="color: var(--ds-text);"
    placeholder={t('createModal.milestoneName')}
  />

  <!-- Description -->
  <div class="min-h-[60px]">
    <MilkdownEditor
      bind:content={formData.description}
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
      value={formData.target_date}
      displayValue={formData.target_date ? new Date(formData.target_date).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }) : ''}
      icon={Calendar}
      placeholder={t('createModal.targetDate')}
      required={true}
    >
      {#snippet children({ close: closePopover })}
        <div class="p-3">
          <input
            type="date"
            bind:value={formData.target_date}
            class="w-full px-3 py-2 rounded border text-sm"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            onchange={() => closePopover()}
          />
        </div>
      {/snippet}
    </FieldChip>

    <!-- Status Chip -->
    <FieldChip
      label={t('createModal.status')}
      value={formData.status}
      displayValue={milestoneStatusOptions.find(s => s.value === formData.status)?.label || t('createModal.planning')}
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
              onclick={() => {
                formData.status = status.value;
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
