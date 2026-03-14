<script>
  import { BasePicker } from '../pickers';
  import Input from '../components/Input.svelte';
  import Label from '../components/Label.svelte';
  import {
    addMinutesToTime,
    createDurationSync,
    durationToString,
    minutesBetweenTimes,
    parseDuration
  } from '../utils/timeUtils.js';
  import Modal from './Modal.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Configuration props
  let {
    defaultProjectId = null,
    defaultItemId = null,
    showProjectField = true,
    showWorkItemField = true,
    allowProjectChange = true,
    projects = [],
    customers = [],
    workItems = [],
    workspaces = [],
    editingWorklog = null,
    onsave = () => {},
    oncancel = () => {}
  } = $props();

  // Helper to format unix timestamp to HH:MM
  function formatTimeFromUnix(unixTimestamp) {
    if (!unixTimestamp) return '';
    const date = new Date(unixTimestamp * 1000);
    return date.toTimeString().substring(0, 5);
  }

  // Helper to format minutes to duration string
  function formatDurationFromMinutes(minutes) {
    if (!minutes) return '';
    const hours = Math.floor(minutes / 60);
    const mins = minutes % 60;
    if (hours === 0) return `${mins}m`;
    if (mins === 0) return `${hours}h`;
    return `${hours}h ${mins}m`;
  }

  const durationSync = createDurationSync(); // Guarded updates to avoid recursive time sync
  let formData = $state({
    project_id: editingWorklog?.project_id ?? defaultProjectId,
    item_id: editingWorklog?.item_id ?? defaultItemId,
    description: editingWorklog?.description ?? '',
    date: editingWorklog ? new Date(editingWorklog.date * 1000).toISOString().split('T')[0] : new Date().toISOString().split('T')[0],
    start_time: editingWorklog ? formatTimeFromUnix(editingWorklog.start_time) : '',
    end_time: editingWorklog ? formatTimeFromUnix(editingWorklog.end_time) : '',
    duration: editingWorklog ? formatDurationFromMinutes(editingWorklog.duration_minutes) : ''
  });

  // Initialize form data when defaults change
  $effect(() => {
    if (defaultProjectId && !formData.project_id) {
      formData.project_id = defaultProjectId;
    }
    if (defaultItemId && !formData.item_id) {
      formData.item_id = defaultItemId;
    }
  });

  // Prepare projects for combobox with customer info (only Active projects can have time logged)
  const projectOptions = $derived(projects
    .filter(project => project.status === 'Active')
    .map(project => {
      const customer = customers.find(c => c.id === project.customer_id);
      return {
        id: project.id,
        name: project.name,
        subtitle: customer?.name || 'Unknown Customer',
        project
      };
    }));

  // Prepare work items for combobox with workspace info
  const workItemOptions = $derived(workItems.map(item => ({
    id: item.id,
    title: item.title,
    subtitle: item.workspace_name || 'Unknown Workspace',
    status: item.status,
    item
  })));

  // Handle start time changes
  function onStartTimeChange() {
    durationSync.guard(() => {
      if (formData.start_time && formData.duration) {
        const durationMinutes = parseDuration(formData.duration);
        if (durationMinutes > 0) {
          formData.end_time = addMinutesToTime(formData.start_time, durationMinutes);
        }
      }
    });
  }

  // Handle duration changes
  function onDurationChange() {
    durationSync.guard(() => {
      if (formData.start_time && formData.duration) {
        const durationMinutes = parseDuration(formData.duration);
        if (durationMinutes > 0) {
          formData.end_time = addMinutesToTime(formData.start_time, durationMinutes);
        }
      }
    });
  }

  // Handle end time changes
  function onEndTimeChange() {
    durationSync.guard(() => {
      if (formData.start_time && formData.end_time) {
        const durationMinutes = minutesBetweenTimes(formData.start_time, formData.end_time);
        if (durationMinutes > 0) {
          formData.duration = durationToString(durationMinutes);
        }
      }
    });
  }

  function getProjectName(projectId) {
    const project = projects.find(p => p.id === projectId);
    return project ? project.name : 'Unknown Project';
  }

  function getWorkItemTitle(itemId) {
    const workItem = workItems.find(w => w.id === itemId);
    return workItem ? workItem.title : 'Unknown Work Item';
  }

  function handleSave() {
    const data = {
      project_id: parseInt(formData.project_id),
      item_id: formData.item_id ? parseInt(formData.item_id) : undefined,
      description: formData.description,
      date: formData.date,
      start_time: formData.start_time || undefined,
      end_time: formData.end_time || undefined,
      duration: formData.duration || undefined
    };
    onsave({ detail: data });
  }

  function handleCancel() {
    oncancel();
  }

  const isFormValid = $derived(formData.project_id && formData.description.trim() && formData.duration);
</script>

<Modal
  isOpen={true}
  onclose={handleCancel}
  maxWidth="max-w-2xl"
  onSubmit={handleSave}
  submitDisabled={!isFormValid}
>
  {#snippet children(submitHint)}
  <!-- Header -->
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">{editingWorklog ? t('time.editTimeEntry') : t('time.logTime')}</h3>
  </div>

  <!-- Form -->
  <div class="p-6 space-y-4">
    <!-- Project Selection (shown at top when from work item) -->
    {#if showProjectField}
      <div>
        <Label color="default" required={!defaultItemId} class="mb-2">{t('time.timeTrackingProject')}</Label>
        <BasePicker
          bind:value={formData.project_id}
          items={projectOptions}
          placeholder={t('placeholders.searchProjects')}
          disabled={!allowProjectChange}
          searchFields={['name', 'subtitle']}
          getValue={(project) => project?.id}
          getLabel={(project) => project?.name ?? ''}
        >
          {#snippet itemSnippet({ item: project })}
            <div class="flex flex-col min-w-0">
              <span class="font-medium">{project.name}</span>
              {#if project.subtitle}
                <span class="text-xs" style="color: var(--ds-text-subtle);">{project.subtitle}</span>
              {/if}
            </div>
          {/snippet}
        </BasePicker>
      </div>
    {/if}

    <!-- Work Item Selection (optional on time page) -->
    {#if showWorkItemField}
      <div>
        <Label color="default" class="mb-2">{t('time.workItemOptional')}</Label>
        <BasePicker
          bind:value={formData.item_id}
          items={workItemOptions}
          placeholder={t('placeholders.searchWorkItems')}
          allowClear={true}
          searchFields={['title', 'subtitle']}
          getValue={(item) => item?.id}
          getLabel={(item) => item?.title ?? ''}
          onSelect={(item) => {
            if (item && !formData.description.trim()) {
              formData.description = item.title;
            }
          }}
        >
          {#snippet itemSnippet({ item: workItem })}
            <div class="flex flex-col min-w-0">
              <span class="font-medium text-sm truncate">{workItem.title}</span>
              {#if workItem.subtitle}
                <span class="text-xs truncate" style="color: var(--ds-text-subtle);">{workItem.subtitle}</span>
              {/if}
              {#if workItem.status}
                <span class="text-xs px-1.5 py-0.5 bg-gray-100 text-gray-700 rounded mt-1 inline-block w-fit">
                  {workItem.status}
                </span>
              {/if}
            </div>
          {/snippet}
        </BasePicker>
      </div>
    {/if}

    <!-- Description -->
    <div>
      <Label color="default" required class="mb-2">{t('common.description')}</Label>
      <Input
        id="time-log-description"
        bind:value={formData.description}
        placeholder={t('time.whatDidYouWorkOn')}
        size="small"
      />
    </div>

    <!-- Time fields -->
    <div class="grid grid-cols-4 gap-3">
      <!-- Start Time -->
      <div>
        <Label color="default" class="mb-2">{t('time.start')}</Label>
        <Input
          type="time"
          bind:value={formData.start_time}
          oninput={onStartTimeChange}
          size="small"
        />
      </div>

      <!-- Duration -->
      <div>
        <Label color="default" required class="mb-2">{t('time.duration')}</Label>
        <Input
          bind:value={formData.duration}
          oninput={onDurationChange}
          placeholder="2h"
          size="small"
        />
      </div>

      <!-- End Time -->
      <div>
        <Label color="default" class="mb-2">{t('time.end')}</Label>
        <Input
          type="time"
          bind:value={formData.end_time}
          oninput={onEndTimeChange}
          size="small"
        />
      </div>

      <!-- Date -->
      <div>
        <Label color="default" class="mb-2">{t('common.date')}</Label>
        <Input
          type="date"
          bind:value={formData.date}
          size="small"
        />
      </div>
    </div>

    <!-- Helper text -->
    <div class="text-xs" style="color: var(--ds-text-subtle);">
      {t('time.durationHelperText')}
    </div>

  </div>

  <!-- Footer -->
  <DialogFooter
    onCancel={handleCancel}
    onConfirm={handleSave}
    confirmLabel={editingWorklog ? t('time.updateEntry') : t('time.logTime')}
    disabled={!isFormValid}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
  {/snippet}
</Modal>
