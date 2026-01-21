<script>
  import { createCombobox, melt } from '@melt-ui/svelte';
  import { fly } from 'svelte/transition';
  import { Check } from 'lucide-svelte';
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

  // Project combobox setup
  const {
    elements: { menu: projectMenu, input: projectInput, option: projectOption },
    states: { open: projectOpen, inputValue: projectInputValue, touchedInput: projectTouchedInput, selected: projectSelected },
    helpers: { isSelected: isProjectSelected }
  } = createCombobox({
    forceVisible: true,
    preventScroll: false
  });

  // Work item combobox setup
  const {
    elements: { menu: workItemMenu, input: workItemInput, option: workItemOption },
    states: { open: workItemOpen, inputValue: workItemInputValue, touchedInput: workItemTouchedInput, selected: workItemSelected },
    helpers: { isSelected: isWorkItemSelected }
  } = createCombobox({
    forceVisible: true,
    preventScroll: false
  });

  // Handle project selection
  $effect(() => {
    if ($projectSelected) {
      const selectedProject = projectOptions.find(p => p.id === $projectSelected.value);
      if (selectedProject) {
        formData.project_id = selectedProject.id;
      }
    }
  });

  // Handle work item selection
  $effect(() => {
    if ($workItemSelected) {
      const selectedItem = workItemOptions.find(w => w.id === $workItemSelected.value);
      if (selectedItem) {
        formData.item_id = selectedItem.id;
        // Auto-populate description with item title if description is empty
        if (!formData.description.trim()) {
          formData.description = selectedItem.title;
        }
      }
    }
  });

  // Set display value when project is selected externally
  $effect(() => {
    if (!$projectTouchedInput && formData.project_id) {
      const selectedProject = projectOptions.find(p => p.id === formData.project_id);
      if (selectedProject) {
        $projectInputValue = selectedProject.name;
      }
    } else if (!formData.project_id) {
      $projectInputValue = '';
    }
  });

  // Set display value when work item is selected externally
  $effect(() => {
    if (!$workItemTouchedInput && formData.item_id) {
      const selectedItem = workItemOptions.find(w => w.id === formData.item_id);
      if (selectedItem) {
        $workItemInputValue = selectedItem.title;
      }
    } else if (!formData.item_id) {
      $workItemInputValue = '';
    }
  });

  // Filter projects based on search input
  const filteredProjectsForDisplay = $derived.by(() => {
    if (!$projectTouchedInput || !$projectInputValue) {
      return projectOptions;
    }
    const search = $projectInputValue.toLowerCase();
    return projectOptions.filter(project =>
      project.name.toLowerCase().includes(search) ||
      project.subtitle?.toLowerCase().includes(search)
    );
  });

  // Filter work items based on search input
  const filteredWorkItemsForDisplay = $derived.by(() => {
    if (!$workItemTouchedInput || !$workItemInputValue) {
      return workItemOptions.slice(0, 20);
    }
    const search = $workItemInputValue.toLowerCase();
    return workItemOptions.filter(item =>
      item.title.toLowerCase().includes(search) ||
      item.subtitle?.toLowerCase().includes(search)
    ).slice(0, 20);
  });

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
  let:submitHint
>
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
        <div class="relative">
          <input
            use:melt={$projectInput}
            type="text"
            placeholder={t('placeholders.searchProjects')}
            disabled={!allowProjectChange}
            class="w-full px-3 py-2.5 pr-8 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50 text-sm"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
          />

          <div class="absolute right-3 top-1/2 transform -translate-y-1/2 pointer-events-none">
            <svg class="w-4 h-4 transition-transform duration-200 {$projectOpen ? 'rotate-180' : ''}"
                 fill="none" stroke="currentColor" viewBox="0 0 24 24"
                 style="color: var(--ds-text-subtle);">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
            </svg>
          </div>

          {#if $projectOpen && filteredProjectsForDisplay.length > 0 && allowProjectChange}
            <div
              use:melt={$projectMenu}
              class="absolute z-50 w-full mt-2 rounded border shadow-lg max-h-60 overflow-y-auto"
              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
              transition:fly={{ duration: 150, y: -5 }}
            >
              {#each filteredProjectsForDisplay as project (project.id)}
                <div
                  use:melt={$projectOption({ value: project.id, label: project.name, item: project })}
                  class="px-4 py-3 cursor-pointer border-b last:border-b-0 transition-colors duration-150"
                  style="border-color: var(--ds-border);
                         {$isProjectSelected({ value: project.id, label: project.name, item: project })
                           ? 'background-color: var(--ds-background-selected); color: var(--ds-text);'
                           : 'color: var(--ds-text); hover:background-color: var(--ds-background-neutral-hovered);'}"
                >
                  <div class="flex items-center justify-between">
                    <div class="flex flex-col">
                      <span class="font-medium">{project.name}</span>
                      {#if project.subtitle}
                        <span class="text-sm mt-1" style="color: var(--ds-text-subtle);">{project.subtitle}</span>
                      {/if}
                    </div>
                    {#if $isProjectSelected({ value: project.id, label: project.name, item: project })}
                      <Check class="w-4 h-4 text-blue-600" />
                    {/if}
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </div>
    {/if}

    <!-- Work Item Selection (optional on time page) -->
    {#if showWorkItemField}
      <div>
        <Label color="default" class="mb-2">{t('time.workItemOptional')}</Label>
        <div class="relative">
          <input
            use:melt={$workItemInput}
            type="text"
            placeholder={t('placeholders.searchWorkItems')}
            class="w-full px-3 py-2.5 pr-8 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50 text-sm"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
          />

          <div class="absolute right-3 top-1/2 transform -translate-y-1/2 pointer-events-none">
            <svg class="w-4 h-4 transition-transform duration-200 {$workItemOpen ? 'rotate-180' : ''}"
                 fill="none" stroke="currentColor" viewBox="0 0 24 24"
                 style="color: var(--ds-text-subtle);">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
            </svg>
          </div>

          {#if $workItemOpen && filteredWorkItemsForDisplay.length > 0}
            <div
              use:melt={$workItemMenu}
              class="absolute z-50 w-full mt-2 rounded border shadow-lg max-h-60 overflow-y-auto"
              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
              transition:fly={{ duration: 150, y: -5 }}
            >
              {#each filteredWorkItemsForDisplay as workItem (workItem.id)}
                <div
                  use:melt={$workItemOption({ value: workItem.id, label: workItem.title, item: workItem })}
                  class="px-4 py-3 cursor-pointer border-b last:border-b-0 transition-colors duration-150"
                  style="border-color: var(--ds-border);
                         {$isWorkItemSelected({ value: workItem.id, label: workItem.title, item: workItem })
                           ? 'background-color: var(--ds-background-selected); color: var(--ds-text);'
                           : 'color: var(--ds-text); hover:background-color: var(--ds-background-neutral-hovered);'}"
                >
                  <div class="flex items-center justify-between">
                    <div class="flex flex-col flex-1 min-w-0">
                      <span class="font-medium text-sm truncate">{workItem.title}</span>
                      {#if workItem.subtitle}
                        <span class="text-xs mt-1 truncate" style="color: var(--ds-text-subtle);">{workItem.subtitle}</span>
                      {/if}
                      {#if workItem.status}
                        <span class="text-xs px-1.5 py-0.5 bg-gray-100 text-gray-700 rounded mt-1 inline-block w-fit">
                          {workItem.status}
                        </span>
                      {/if}
                    </div>
                    {#if $isWorkItemSelected({ value: workItem.id, label: workItem.title, item: workItem })}
                      <Check class="w-4 h-4 text-blue-600 flex-shrink-0 ml-2" />
                    {/if}
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
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
</Modal>
