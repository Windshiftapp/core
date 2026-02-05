<script>
  import { onMount } from 'svelte';
  import { useEventListener } from 'runed';
  import { ChevronLeft, ChevronRight, Calendar, CheckSquare, X, ChevronDown, Download, ExternalLink, MoreHorizontal } from 'lucide-svelte';
  import { api } from '../../api.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import PersonalTaskDetail from '../personal/PersonalTaskDetail.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import { exportTasksToICS } from '../../utils/icsExport.js';
  import { authStore, workspacesStore } from '../../stores';
  import { getStatusStyleFromStatuses } from '../../utils/statusColors.js';
  import { t } from '../../stores/i18n.svelte.js';
  import Checkbox from '../../components/Checkbox.svelte';

  // Get current date and week
  let currentDate = $state(new Date());
  let currentWeekStart = $state(new Date());
  let weekDays = $state([]);
  let workItems = $state({}); // Store scheduled work items for each day
  let showTasksSidebar = $state(false);
  let assignedWorkItems = $state([]); // Real work items assigned to user
  let availableStatuses = $state([]); // Status options for work items

  // Item detail modal state
  let showItemModal = $state(false);
  let selectedItemId = $state(null);
  let selectedWorkspaceId = $state(null);

  // Time grid configuration
  const DAY_START_HOUR = 6;   // 6 AM
  const DAY_END_HOUR = 22;    // 10 PM
  const PIXELS_PER_HOUR = 60; // Height per hour
  const TOTAL_HOURS = DAY_END_HOUR - DAY_START_HOUR;
  const GRID_HEIGHT = TOTAL_HOURS * PIXELS_PER_HOUR;
  const DEFAULT_DURATION = 60; // Default 1 hour for new items

  // Drag state for resizing
  let resizingItem = $state(null);
  let resizeEdge = $state(null); // 'top' or 'bottom'
  let resizeStartY = $state(0);
  let resizeStartTime = $state('');
  let resizeStartDuration = $state(0);

  // Inline task creation state
  let creatingTaskAt = $state(null); // { dateKey, time, top }
  let newTaskTitle = $state('');

  // Drag vs click distinction
  let isDragging = $state(false);
  let lastDragOrResizeEnd = $state(0);

  // Status configuration for ItemPicker
  const statusConfig = {
    icon: {
      type: 'color-dot',
      source: (item) => item.categoryColor || '#9CA3AF',
      size: 'w-2 h-2'
    },
    primary: {
      text: (item) => item.label
    },
    searchFields: ['label', 'value'],
    getValue: (item) => item.id,
    getLabel: (item) => item.label
  };

  // Transform availableStatuses for ItemPicker
  let statusOptions = $derived(availableStatuses.map(status => ({
    id: status.id,
    label: status.name,
    value: status.name,
    categoryColor: status.category_color
  })));

  // Generate time slots for the grid
  const timeSlots = Array.from({ length: TOTAL_HOURS }, (_, i) => {
    const hour = DAY_START_HOUR + i;
    return {
      hour,
      label: formatTimeLabel(hour),
      top: i * PIXELS_PER_HOUR
    };
  });

  function formatTimeLabel(hour) {
    const h = hour % 12 || 12;
    const ampm = hour < 12 ? 'AM' : 'PM';
    return `${h} ${ampm}`;
  }

  function timeToMinutes(timeStr) {
    if (!timeStr) return null;
    const [hours, minutes] = timeStr.split(':').map(Number);
    return hours * 60 + minutes;
  }

  function minutesToTime(minutes) {
    const h = Math.floor(minutes / 60);
    const m = minutes % 60;
    return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}`;
  }

  function getItemTop(scheduledTime) {
    if (!scheduledTime) return 0;
    const minutes = timeToMinutes(scheduledTime);
    const startMinutes = DAY_START_HOUR * 60;
    return ((minutes - startMinutes) / 60) * PIXELS_PER_HOUR;
  }

  function getItemHeight(durationMinutes) {
    return (durationMinutes || DEFAULT_DURATION) / 60 * PIXELS_PER_HOUR;
  }

  function snapToGrid(minutes, gridMinutes = 15) {
    return Math.round(minutes / gridMinutes) * gridMinutes;
  }

  // Load assigned work items
  async function loadAssignedWorkItems() {
    try {
      const user = authStore.currentUser;
      const workspace = $workspacesStore.personalWorkspace;

      if (!user || !user.id) {
        console.warn('No authenticated user found for loading assigned work');
        assignedWorkItems = [];
        return;
      }

      // Load items from TWO sources:
      // 1. Items assigned to user (from any workspace)
      const assignedResponse = await api.items.getAll({
        assignee_id: user.id,
        limit: 50
      });
      const assignedItems = assignedResponse.items || [];

      // 2. Items from personal workspace (regardless of assignee)
      let personalItems = [];
      if (workspace && workspace.id) {
        const personalResponse = await api.items.getAll({
          workspace_id: workspace.id,
          limit: 50
        });
        personalItems = personalResponse.items || [];
      }

      // Merge and deduplicate by item ID
      const itemMap = new Map();
      [...assignedItems, ...personalItems].forEach(item => {
        itemMap.set(item.id, item);
      });
      assignedWorkItems = Array.from(itemMap.values());

      // Load available statuses
      const statusResponse = await api.statuses.getAll();
      availableStatuses = statusResponse || [];
    } catch (error) {
      console.error('Failed to load work items:', error);
      assignedWorkItems = [];
      availableStatuses = [];
    }
  }

  // Check if item is from personal workspace (should be checkbox)
  function isPersonalWorkspaceItem(item) {
    return item.workspace_name === "User's Todo List" ||
           item.workspace_name?.toLowerCase().includes('personal') ||
           item.workspace_name?.toLowerCase().includes('todo');
  }

  // Personal task helpers (same as TodoList)
  function isPersonalTaskCompleted(item) {
    const status = availableStatuses.find(s => s.id === item.status_id);
    return status?.category_name === 'Done' || status?.name.toLowerCase().includes('complete') || status?.name.toLowerCase().includes('done');
  }

  async function togglePersonalTask(item) {
    try {
      let targetStatusId;

      if (isPersonalTaskCompleted(item)) {
        // If completed, move to "Open" or first non-done status
        const openStatus = availableStatuses.find(s => s.name.toLowerCase() === 'open') ||
                          availableStatuses.find(s => s.category_name !== 'Done') ||
                          availableStatuses[0];
        targetStatusId = openStatus.id;
      } else {
        // If not completed, move to "Done" or first done status
        const doneStatus = availableStatuses.find(s => s.category_name === 'Done') ||
                          availableStatuses.find(s => s.name.toLowerCase().includes('done')) ||
                          availableStatuses.find(s => s.name.toLowerCase().includes('complete'));
        targetStatusId = doneStatus?.id;
      }

      if (targetStatusId) {
        await updateWorkItemStatus(item, targetStatusId);
        // Update the work item in calendar data and save
        Object.keys(workItems).forEach(dateKey => {
          const itemIndex = workItems[dateKey].findIndex(w => w.id === item.id);
          if (itemIndex !== -1) {
            workItems[dateKey][itemIndex].status_id = targetStatusId;
          }
        });
        workItems = { ...workItems };
      }
    } catch (error) {
      console.error('Failed to toggle personal task:', error);
    }
  }

  // Update work item status
  async function updateWorkItemStatus(item, newStatusId) {
    try {
      await api.items.update(item.id, {
        ...item,
        status_id: newStatusId
      });

      // Update local state
      const index = assignedWorkItems.findIndex(i => i.id === item.id);
      if (index !== -1) {
        assignedWorkItems[index] = { ...assignedWorkItems[index], status_id: newStatusId };
        assignedWorkItems = [...assignedWorkItems];
      }
    } catch (error) {
      console.error('Failed to update work item status:', error);
    }
  }

  function getStatusName(statusId) {
    const statusObj = availableStatuses.find(s => s.id === statusId);
    return statusObj?.name || 'Open';
  }

  // Open work item in modal
  function openWorkItem(workItem) {
    selectedItemId = workItem.id;
    selectedWorkspaceId = workItem.workspace_id;
    showItemModal = true;
  }

  // Close item modal
  async function closeItemModal() {
    showItemModal = false;
    selectedItemId = null;
    selectedWorkspaceId = null;
    await loadScheduledItems();
    await loadAssignedWorkItems();
  }

  // Get all tasks for the current week (work items only)
  let allWeekTasks = $derived((() => {
    const tasks = [];
    weekDays.forEach(day => {
      if (workItems[day.dateKey]?.length) {
        workItems[day.dateKey].forEach(item => {
          tasks.push({
            ...item,
            date: day.date,
            dateKey: day.dateKey,
            dayName: day.dayName,
            dayNumber: day.dayNumber
          });
        });
      }
    });
    return tasks.sort((a, b) => a.date - b.date);
  })());

  // Calculate week start (Monday)
  function getWeekStart(date) {
    const d = new Date(date);
    const day = d.getDay();
    const diff = d.getDate() - day + (day === 0 ? -6 : 1); // Adjust for Monday start
    return new Date(d.setDate(diff));
  }

  // Generate week days array
  function generateWeekDays(weekStart) {
    const days = [];
    for (let i = 0; i < 7; i++) {
      const date = new Date(weekStart);
      date.setDate(weekStart.getDate() + i);
      days.push({
        date: new Date(date),
        dayName: date.toLocaleDateString('en-US', { weekday: 'short' }),
        dayNumber: date.getDate(),
        isToday: isToday(date),
        isCurrentMonth: date.getMonth() === currentDate.getMonth(),
        dateKey: formatDateKey(date)
      });
    }
    return days;
  }

  function isToday(date) {
    const today = new Date();
    return date.toDateString() === today.toDateString();
  }

  function formatDateKey(date) {
    return date.toISOString().split('T')[0]; // YYYY-MM-DD format
  }

  function formatWeekRange(weekStart) {
    const weekEnd = new Date(weekStart);
    weekEnd.setDate(weekStart.getDate() + 6);

    const startStr = weekStart.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric'
    });
    const endStr = weekEnd.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric'
    });

    return `${startStr} - ${endStr}`;
  }

  function navigateWeek(direction) {
    const newWeekStart = new Date(currentWeekStart);
    newWeekStart.setDate(currentWeekStart.getDate() + (direction * 7));
    currentWeekStart = newWeekStart;
    weekDays = generateWeekDays(currentWeekStart);
    loadScheduledItems();
  }

  function goToToday() {
    currentDate = new Date();
    currentWeekStart = getWeekStart(currentDate);
    weekDays = generateWeekDays(currentWeekStart);
    loadScheduledItems();
  }

  // Create a new task in the calendar
  async function createCalendarTask(dateKey, title, scheduledTime = null) {
    try {
      const user = authStore.currentUser;
      const workspace = $workspacesStore.personalWorkspace;

      if (!user || !user.id) {
        throw new Error('No authenticated user found');
      }

      if (!workspace || !workspace.id) {
        throw new Error('No personal workspace found');
      }

      // Find default status
      const defaultStatus = availableStatuses.find(s => s.is_default) ||
                           availableStatuses.find(s => s.name.toLowerCase() === 'open') ||
                           availableStatuses[0];

      // Create the item in the database
      const itemData = {
        title: title.trim(),
        description: '',
        workspace_id: workspace.id,
        assignee_id: user.id,
        status_id: defaultStatus?.id || 1
      };

      const newItem = await api.items.create(itemData);

      // Schedule it for the selected date with time
      await scheduleItemToCalendar(newItem, dateKey, scheduledTime, DEFAULT_DURATION);

      // Add to calendar display
      if (!workItems[dateKey]) {
        workItems[dateKey] = [];
      }

      workItems[dateKey].push({
        id: newItem.id,
        title: newItem.title,
        description: newItem.description,
        status_id: newItem.status_id,
        priority: newItem.priority,
        workspace_name: workspace.name,
        workspace_key: workspace.key,
        workspace_id: workspace.id,
        is_personal: true,
        type: 'work-item',
        scheduledDate: dateKey,
        scheduledTime: scheduledTime,
        durationMinutes: DEFAULT_DURATION
      });

      workItems = { ...workItems };

      // Reload assigned work items to stay in sync
      await loadAssignedWorkItems();
    } catch (error) {
      console.error('Failed to create calendar task:', error);
      alert(t('time.calendar.failedToCreateTask') + ': ' + error.message);
    }
  }

  async function removeWorkItem(dateKey, itemId) {
    if (workItems[dateKey]) {
      // Find the work item before removing it
      const workItem = workItems[dateKey].find(w => w.id === itemId);

      if (workItem && workItem.type === 'work-item') {
        try {
          // Unschedule item via API
          await unscheduleItemFromCalendar(itemId);

          // Remove from calendar
          workItems[dateKey] = workItems[dateKey].filter(w => w.id !== itemId);
          workItems = { ...workItems };

          // Add back to sidebar
          const { scheduledDate, scheduledTime, durationMinutes, ...originalItem } = workItem;
          assignedWorkItems = [...assignedWorkItems, originalItem];
        } catch (error) {
          console.error('Failed to unschedule item:', error);
        }
      }
    }
  }

  // Calendar data persistence functions
  async function scheduleItemToCalendar(item, dateKey, scheduledTime = null, durationMinutes = DEFAULT_DURATION) {
    try {
      const user = authStore.currentUser;
      const workspace = $workspacesStore.personalWorkspace;

      if (!user || !user.id) {
        throw new Error('No authenticated user found');
      }

      const scheduleRequest = {
        user_id: user.id,
        workspace_id: workspace?.id || item.workspace_id,
        scheduled_date: dateKey,
        scheduled_time: scheduledTime || '',
        duration_minutes: durationMinutes
      };

      const response = await fetch(`/api/items/${item.id}/schedule`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(scheduleRequest)
      });

      if (!response.ok) {
        throw new Error(`Failed to schedule item: ${response.status} ${response.statusText}`);
      }
    } catch (error) {
      console.error('Failed to schedule item:', error);
      throw error;
    }
  }

  async function updateItemSchedule(itemId, dateKey, scheduledTime, durationMinutes) {
    try {
      const user = authStore.currentUser;
      const workspace = $workspacesStore.personalWorkspace;

      if (!user || !user.id) {
        throw new Error('No authenticated user found');
      }

      const scheduleRequest = {
        user_id: user.id,
        workspace_id: workspace?.id,
        scheduled_date: dateKey,
        scheduled_time: scheduledTime || '',
        duration_minutes: durationMinutes
      };

      const response = await fetch(`/api/items/${itemId}/schedule`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(scheduleRequest)
      });

      if (!response.ok) {
        throw new Error(`Failed to update schedule: ${response.status} ${response.statusText}`);
      }
    } catch (error) {
      console.error('Failed to update schedule:', error);
      throw error;
    }
  }

  async function unscheduleItemFromCalendar(itemId) {
    try {
      const user = authStore.currentUser;
      if (!user || !user.id) {
        throw new Error('No authenticated user found');
      }

      const response = await fetch(`/api/items/${itemId}/unschedule?user_id=${user.id}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json',
        }
      });

      if (!response.ok) {
        throw new Error(`Failed to unschedule item: ${response.status} ${response.statusText}`);
      }
    } catch (error) {
      console.error('Failed to unschedule item:', error);
      throw error;
    }
  }

  async function loadScheduledItems() {
    try {
      const user = authStore.currentUser;
      if (!user || !user.id) {
        console.warn('No authenticated user found for loading scheduled items');
        return;
      }

      const startDate = formatDateKey(new Date(currentWeekStart));
      const endDate = formatDateKey(new Date(currentWeekStart.getTime() + 6 * 24 * 60 * 60 * 1000));

      const response = await fetch(`/api/calendar/scheduled-items?user_id=${user.id}&start_date=${startDate}&end_date=${endDate}`, {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
        }
      });

      if (!response.ok) {
        throw new Error(`Failed to load scheduled items: ${response.status} ${response.statusText}`);
      }

      const scheduledData = await response.json();

      // Transform scheduled data back into workItems format
      workItems = {};
      const scheduledItemIds = new Set();

      Object.entries(scheduledData).forEach(([dateKey, items]) => {
        workItems[dateKey] = items.map(item => ({
          id: item.id,
          title: item.title,
          description: item.description,
          status_id: item.status_id,
          priority: item.priority,
          workspace_name: item.workspace_name,
          workspace_key: item.workspace_key,
          workspace_id: item.workspace_id,
          is_personal: item.is_personal === true || item.is_personal === 1,
          type: 'work-item',
          scheduledDate: dateKey,
          scheduledTime: item.scheduled_time || null,
          durationMinutes: item.duration_minutes != null ? item.duration_minutes : DEFAULT_DURATION,
          dueDate: item.due_date || null
        }));

        items.forEach(item => scheduledItemIds.add(item.id));
      });

      // Filter out already scheduled items from sidebar
      assignedWorkItems = assignedWorkItems.filter(item => !scheduledItemIds.has(item.id));

    } catch (error) {
      console.error('Failed to load scheduled items:', error);
    }
  }

  // Resize handlers - useEventListener auto-attaches/detaches based on resizingItem
  useEventListener(() => resizingItem ? document : undefined, 'mousemove', handleResize);
  useEventListener(() => resizingItem ? document : undefined, 'mouseup', stopResize);

  function startResize(e, item, dateKey, edge) {
    e.preventDefault();
    e.stopPropagation();

    resizingItem = { ...item, dateKey };
    resizeEdge = edge;
    resizeStartY = e.clientY;
    resizeStartTime = item.scheduledTime || minutesToTime(DAY_START_HOUR * 60);
    resizeStartDuration = item.durationMinutes || DEFAULT_DURATION;
  }

  function handleResize(e) {
    if (!resizingItem) return;

    const deltaY = e.clientY - resizeStartY;
    const deltaMinutes = Math.round((deltaY / PIXELS_PER_HOUR) * 60);
    const snappedDelta = snapToGrid(deltaMinutes, 15);

    const dateKey = resizingItem.dateKey;
    const itemIndex = workItems[dateKey]?.findIndex(w => w.id === resizingItem.id);

    if (itemIndex === -1 || itemIndex === undefined) return;

    if (resizeEdge === 'top') {
      // Adjust start time
      const startMinutes = timeToMinutes(resizeStartTime);
      let newStartMinutes = snapToGrid(startMinutes + snappedDelta, 15);

      // Clamp to valid range
      newStartMinutes = Math.max(DAY_START_HOUR * 60, Math.min(newStartMinutes, (DAY_END_HOUR - 1) * 60));

      // Adjust duration to keep end time the same
      const endMinutes = startMinutes + resizeStartDuration;
      const newDuration = endMinutes - newStartMinutes;

      if (newDuration >= 15) { // Minimum 15 minutes
        workItems[dateKey][itemIndex].scheduledTime = minutesToTime(newStartMinutes);
        workItems[dateKey][itemIndex].durationMinutes = newDuration;
        workItems = { ...workItems };
      }
    } else if (resizeEdge === 'bottom') {
      // Adjust duration
      let newDuration = snapToGrid(resizeStartDuration + snappedDelta, 15);

      // Clamp to valid range
      const startMinutes = timeToMinutes(resizeStartTime);
      const maxDuration = (DAY_END_HOUR * 60) - startMinutes;
      newDuration = Math.max(15, Math.min(newDuration, maxDuration));

      workItems[dateKey][itemIndex].durationMinutes = newDuration;
      workItems = { ...workItems };
    }
  }

  async function stopResize() {
    // Set timestamp IMMEDIATELY to block click events - must be first!
    lastDragOrResizeEnd = Date.now();

    if (resizingItem) {
      const dateKey = resizingItem.dateKey;
      const item = workItems[dateKey]?.find(w => w.id === resizingItem.id);

      if (item) {
        try {
          await updateItemSchedule(item.id, dateKey, item.scheduledTime, item.durationMinutes);
        } catch (error) {
          console.error('Failed to save resize:', error);
        }
      }
    }

    resizingItem = null;
    resizeEdge = null;
  }

  // Handle click on time grid to create task at that time
  function handleGridClick(e, dateKey) {
    // Block clicks for 200ms after drag/resize ends
    if (Date.now() - lastDragOrResizeEnd < 200) return;

    // Only create if clicking empty space (not an item or other element)
    if (e.target !== e.currentTarget) return;

    const rect = e.currentTarget.getBoundingClientRect();
    const y = e.clientY - rect.top;
    const minutes = snapToGrid(DAY_START_HOUR * 60 + (y / PIXELS_PER_HOUR) * 60, 15);
    const time = minutesToTime(minutes);
    const top = getItemTop(time);

    creatingTaskAt = { dateKey, time, top };
    newTaskTitle = '';
  }

  function handleInlineTaskKeydown(e) {
    if (e.key === 'Enter' && newTaskTitle.trim()) {
      createCalendarTask(creatingTaskAt.dateKey, newTaskTitle.trim(), creatingTaskAt.time);
      creatingTaskAt = null;
      newTaskTitle = '';
    } else if (e.key === 'Escape') {
      creatingTaskAt = null;
      newTaskTitle = '';
    }
  }

  function handleInlineTaskBlur() {
    // Small delay to allow Enter key to process first
    setTimeout(() => {
      creatingTaskAt = null;
      newTaskTitle = '';
    }, 100);
  }

  // PDF Export functionality
  async function exportToPDF() {
    // ... (keeping existing PDF export logic)
    alert('PDF export coming soon for time-block view');
  }

  // Menu items for calendar actions
  let calendarMenuItems = $derived([
    {
      id: 'export-ics',
      type: 'regular',
      icon: Download,
      title: t('time.calendar.exportWeekToICS'),
      onClick: handleExportICS
    }
  ]);

  function handleExportICS() {
    // Get only items scheduled for the current week
    const currentWeekTasks = [];
    for (const dateKey of Object.keys(workItems)) {
      const dayItems = workItems[dateKey] || [];
      currentWeekTasks.push(...dayItems);
    }

    const filename = `windshift-calendar-${currentWeekStart.toISOString().split('T')[0]}.ics`;
    exportTasksToICS(currentWeekTasks, window.location.origin, filename);
  }

  onMount(async () => {
    currentWeekStart = getWeekStart(currentDate);
    weekDays = generateWeekDays(currentWeekStart);

    // Load personal workspace for task creation
    await workspacesStore.loadPersonalWorkspace();

    // Load real work items first
    await loadAssignedWorkItems();

    // Load scheduled items from database
    await loadScheduledItems();
  });
</script>

<div class="min-h-screen flex">
  <!-- Tasks Sidebar -->
  {#if showTasksSidebar}
    <div class="w-80 min-w-80 border-r flex flex-col" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <!-- Sidebar Header -->
      <div class="p-6 border-b" style="border-color: var(--ds-border);">
        <div class="flex items-center justify-between">
          <div>
            <h3 class="text-lg font-semibold flex items-center gap-2" style="color: var(--ds-text);">
              <CheckSquare class="w-5 h-5" style="color: var(--ds-accent-orange);" />
              {t('time.calendar.myWorkItems')}
            </h3>
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('time.calendar.dragToSchedule')}</p>
          </div>
          <button
            onclick={() => showTasksSidebar = false}
            class="p-1 rounded transition-colors sidebar-close-btn"
          >
            <X class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          </button>
        </div>
      </div>

      <!-- Work Items List -->
      <div class="flex-1 overflow-y-auto p-4">
        {#if assignedWorkItems.length === 0}
          <div class="text-center py-8">
            <CheckSquare class="w-8 h-8 mx-auto mb-3" style="color: var(--ds-text-subtlest);" />
            <p class="text-sm" style="color: var(--ds-text-subtle);">{t('time.calendar.noWorkItems')}</p>
            <p class="text-xs mt-1" style="color: var(--ds-text-subtlest);">{t('time.calendar.workItemsWillAppear')}</p>
          </div>
        {:else}
          <div class="space-y-3">
            {#each assignedWorkItems as item}
              <div
                class="rounded p-3 border transition-colors cursor-move shadow-sm sidebar-item"
                style="background-color: var(--ds-surface); border-color: var(--ds-border);"
                role="button"
                tabindex="0"
                draggable="true"
                ondragstart={(e) => {
                  e.dataTransfer.setData('text/plain', JSON.stringify({
                    id: item.id,
                    title: item.title,
                    description: item.description,
                    status_id: item.status_id,
                    priority: item.priority,
                    workspace_name: item.workspace_name,
                    workspace_key: item.workspace_key,
                    workspace_id: item.workspace_id,
                    is_personal: isPersonalWorkspaceItem(item),
                    type: 'work-item'
                  }));
                }}
              >
                {#if isPersonalWorkspaceItem(item)}
                  <!-- Personal Task with Checkbox -->
                  <div class="flex items-start gap-3">
                    <Checkbox
                      checked={isPersonalTaskCompleted(item)}
                      onchange={() => togglePersonalTask(item)}
                      size="small"
                      class="flex-shrink-0 mt-0.5"
                    />
                    <div class="flex-1 min-w-0">
                      <button
                        onclick={(e) => { e.stopPropagation(); openWorkItem(item); }}
                        class="text-xs font-semibold truncate block w-full text-left cursor-pointer hover:underline"
                        style="color: {isPersonalTaskCompleted(item) ? 'var(--ds-text-subtle)' : 'var(--ds-text)'}; {isPersonalTaskCompleted(item) ? 'text-decoration: line-through;' : ''}"
                        title={item.title}
                      >
                        {item.title}
                      </button>
                      <button
                        onclick={(e) => { e.stopPropagation(); openWorkItem(item); }}
                        class="text-[10px] font-mono cursor-pointer hover:underline text-left"
                        style="color: var(--ds-accent-orange);"
                      >
                        {item.workspace_key || item.workspace_name}-{item.workspace_item_number || item.id}
                      </button>
                    </div>
                  </div>
                {:else}
                  <!-- Work Item with Status -->
                  <div class="flex flex-col gap-1">
                    <button
                      onclick={(e) => { e.stopPropagation(); openWorkItem(item); }}
                      class="text-xs font-semibold truncate block w-full text-left cursor-pointer hover:underline"
                      style="color: var(--ds-text);"
                      title={item.title}
                    >
                      {item.title}
                    </button>
                    <button
                      onclick={(e) => { e.stopPropagation(); openWorkItem(item); }}
                      class="text-[10px] font-mono cursor-pointer hover:underline text-left"
                      style="color: var(--ds-accent-blue);"
                    >
                      {item.workspace_key}-{item.workspace_item_number || item.id}
                    </button>
                    <div class="flex items-center justify-between gap-2 mt-1">
                      <span class="text-xs px-1.5 py-0.5 rounded text-[10px] font-medium" style="background-color: var(--ds-accent-blue-subtler); color: var(--ds-accent-blue);">
                        {item.workspace_name}
                      </span>
                      <span class="text-xs px-1.5 py-0.5 rounded font-medium" style={getStatusStyleFromStatuses(getStatusName(item.status_id), availableStatuses)}>
                        {getStatusName(item.status_id)}
                      </span>
                    </div>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Progress Footer -->
      <div class="p-4 border-t" style="border-color: var(--ds-border);">
        <div class="text-xs" style="color: var(--ds-text-subtle);">
          {t('time.calendar.itemsCompleted', {
            completed: assignedWorkItems.filter(item => {
              if (isPersonalWorkspaceItem(item)) {
                return isPersonalTaskCompleted(item);
              } else {
                const status = availableStatuses.find(s => s.id === item.status_id);
                return status?.category_name === 'Done';
              }
            }).length,
            total: assignedWorkItems.length
          })}
        </div>
      </div>
    </div>
  {/if}

  <!-- Main Calendar Area -->
  <div class="calendar-content flex-1 flex flex-col" style="background-color: var(--ds-surface);">
    <!-- Header -->
    <div class="px-6 pt-6">
      <PageHeader
        icon={Calendar}
        title={t('time.calendar.title')}
        subtitle="{formatWeekRange(currentWeekStart)}"
        count={t('time.calendar.itemCount', { count: allWeekTasks.length })}
      >
        {#snippet actions()}
          <div class="flex items-center space-x-4">
            <!-- Week Navigation -->
            <div class="flex items-center space-x-2">
              <button
                class="p-2 rounded transition-colors text-[var(--ds-text-subtle)] hover:bg-[var(--ds-background-neutral-hovered)]"
                onclick={() => navigateWeek(-1)}
                title={t('time.calendar.previousWeek')}
              >
                <ChevronLeft class="w-4 h-4" />
              </button>
              <button
                class="text-xs px-2 py-1 rounded transition-colors text-[var(--ds-text-subtle)] hover:bg-[var(--ds-background-neutral-hovered)] hover:text-[var(--ds-text)]"
                onclick={goToToday}
              >
                {t('time.calendar.thisWeek')}
              </button>
              <button
                class="p-2 rounded transition-colors text-[var(--ds-text-subtle)] hover:bg-[var(--ds-background-neutral-hovered)]"
                onclick={() => navigateWeek(1)}
                title={t('time.calendar.nextWeek')}
              >
                <ChevronRight class="w-4 h-4" />
              </button>
            </div>

            <!-- Action Buttons -->
            <div class="flex gap-3 items-center">
              <button
                onclick={() => showTasksSidebar = true}
                class="px-4 py-2 border rounded transition-colors text-sm font-medium flex items-center gap-2"
                style="background-color: var(--ds-surface-raised); color: var(--ds-text); border-color: var(--ds-border);"
              >
                <CheckSquare class="w-4 h-4" />
                {t('time.calendar.myWorkItems')} ({assignedWorkItems.length})
              </button>
              <DropdownMenu
                items={calendarMenuItems}
                triggerIcon={MoreHorizontal}
                placement="bottom"
              />
            </div>
          </div>
        {/snippet}
      </PageHeader>
    </div>

    <!-- Calendar Grid with Time Ruler -->
    <div class="flex-1 overflow-auto p-4">
      <div class="flex" style="min-width: 1200px;">
        <!-- Time Ruler -->
        <div class="w-16 flex-shrink-0">
          <!-- Spacer to match day headers -->
          <div class="p-2 text-center border-b mb-2 invisible" style="border-color: var(--ds-border);">
            <span class="text-xs font-medium">X</span>
            <span class="block text-lg font-bold">0</span>
          </div>
          <!-- Time labels -->
          <div class="relative" style="height: {GRID_HEIGHT}px;">
            {#each timeSlots as slot}
              <div
                class="absolute w-full text-right pr-2 text-xs"
                style="top: {slot.top}px; color: var(--ds-text-subtle);"
              >
                {slot.label}
              </div>
            {/each}
          </div>
        </div>

        <!-- Day Columns -->
        <div class="flex-1 grid grid-cols-7 gap-2">
          {#each weekDays as day}
            <div class="flex flex-col">
              <!-- Day Header -->
              <div class="p-2 text-center border-b mb-2" style="border-color: var(--ds-border);">
                <span class="text-xs font-medium" style="color: var(--ds-text-subtle);">{day.dayName}</span>
                <span class="block text-lg font-bold" style="color: {day.isToday ? 'var(--ds-accent-orange)' : 'var(--ds-text)'};">
                  {day.dayNumber}
                </span>
              </div>

              <!-- Time Grid -->
              <div
                class="relative rounded border"
                style="height: {GRID_HEIGHT}px; background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
                onclick={(e) => handleGridClick(e, day.dateKey)}
                ondragover={(e) => {
                  e.preventDefault();
                  e.currentTarget.classList.add('drag-over-valid');
                }}
                ondragleave={(e) => {
                  if (!e.currentTarget.contains(e.relatedTarget)) {
                    e.currentTarget.classList.remove('drag-over-valid');
                  }
                }}
                ondrop={async (e) => {
                  e.preventDefault();
                  e.currentTarget.classList.remove('drag-over-valid');

                  // Calculate drop time based on Y position
                  const rect = e.currentTarget.getBoundingClientRect();
                  const y = e.clientY - rect.top;
                  const minutes = snapToGrid(DAY_START_HOUR * 60 + (y / PIXELS_PER_HOUR) * 60, 15);
                  const dropTime = minutesToTime(minutes);

                  try {
                    const itemData = JSON.parse(e.dataTransfer.getData('text/plain'));

                    if (itemData.type === 'work-item') {
                      const isSameDay = itemData.sourceDateKey === day.dateKey;

                      if (isSameDay) {
                        // Same-day drop: just update the time
                        const itemIndex = workItems[day.dateKey].findIndex(w => w.id === itemData.id);
                        if (itemIndex !== -1) {
                          workItems[day.dateKey][itemIndex].scheduledTime = dropTime;
                          workItems = { ...workItems };
                          await updateItemSchedule(itemData.id, day.dateKey, dropTime, itemData.durationMinutes || DEFAULT_DURATION);
                        }
                      } else {
                        // Different day drop: move item between days
                        if (!workItems[day.dateKey]) {
                          workItems[day.dateKey] = [];
                        }

                        const exists = workItems[day.dateKey].some(w => w.id === itemData.id);
                        if (!exists) {
                          // Remove from sidebar or source day
                          if (itemData.sourceDateKey) {
                            if (workItems[itemData.sourceDateKey]) {
                              workItems[itemData.sourceDateKey] = workItems[itemData.sourceDateKey].filter(w => w.id !== itemData.id);
                            }
                          } else {
                            assignedWorkItems = assignedWorkItems.filter(item => item.id !== itemData.id);
                          }

                          // Schedule with time
                          await scheduleItemToCalendar(itemData, day.dateKey, dropTime, itemData.durationMinutes || DEFAULT_DURATION);

                          workItems[day.dateKey].push({
                            ...itemData,
                            scheduledDate: day.dateKey,
                            scheduledTime: dropTime,
                            durationMinutes: itemData.durationMinutes || DEFAULT_DURATION
                          });

                          workItems = { ...workItems };
                        }
                      }
                    }
                  } catch (error) {
                    console.error('Failed to drop item:', error);
                  }
                }}
              >
                <!-- Hour lines -->
                {#each timeSlots as slot}
                  <div
                    class="absolute w-full border-t"
                    style="top: {slot.top}px; border-color: var(--ds-border); opacity: 0.5;"
                  ></div>
                {/each}

                <!-- Scheduled Items -->
                {#if workItems[day.dateKey]?.length}
                  {#each workItems[day.dateKey] as workItem}
                    {@const hasTime = workItem.scheduledTime}
                    {@const top = hasTime ? getItemTop(workItem.scheduledTime) : 0}
                    {@const height = getItemHeight(workItem.durationMinutes)}

                    {#if hasTime}
                      <!-- Time-positioned item -->
                      <div
                        class="absolute left-1 right-1 rounded group/item cursor-grab calendar-time-item"
                        style="top: {top}px; height: {height}px;"
                        class:calendar-item-personal={workItem.is_personal}
                        class:calendar-item-work={!workItem.is_personal}
                        draggable="true"
                        ondragstart={(e) => {
                          isDragging = true;
                          e.dataTransfer.setData('text/plain', JSON.stringify({
                            ...workItem,
                            sourceDateKey: day.dateKey
                          }));
                        }}
                        ondragend={() => { isDragging = false; lastDragOrResizeEnd = Date.now(); }}
                      >
                        <!-- Top resize handle -->
                        <div
                          class="resize-handle resize-handle-top"
                          onmousedown={(e) => { e.stopPropagation(); e.preventDefault(); startResize(e, workItem, day.dateKey, 'top'); }}
                        >
                          <div class="resize-handle-cap resize-handle-cap-left"></div>
                          <div class="resize-handle-cap resize-handle-cap-right"></div>
                        </div>

                        <!-- Content -->
                        <div class="p-2 h-full overflow-hidden">
                          <div class="flex items-start justify-between gap-1">
                            <div class="flex-1 min-w-0">
                              <button
                                onclick={(e) => { e.stopPropagation(); openWorkItem(workItem); }}
                                class="text-xs font-medium truncate block w-full text-left cursor-pointer hover:underline"
                                style="color: var(--ds-text);{isPersonalTaskCompleted(workItem) ? ' text-decoration: line-through;' : ''}"
                              >
                                {workItem.title}
                              </button>
                              <p class="text-[10px]" style="color: var(--ds-text-subtle);">
                                {workItem.scheduledTime} - {minutesToTime(timeToMinutes(workItem.scheduledTime) + workItem.durationMinutes)}
                              </p>
                              {#if workItem.dueDate}
                                {@const dueDate = new Date(workItem.dueDate)}
                                {@const isOverdue = dueDate < new Date() && dueDate.toDateString() !== new Date().toDateString()}
                                <p class="text-[10px]" style="color: {isOverdue ? 'var(--ds-text-danger)' : 'var(--ds-text-subtle)'};">
                                  Due: {dueDate.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
                                </p>
                              {/if}
                              <button
                                onclick={(e) => { e.stopPropagation(); openWorkItem(workItem); }}
                                class="text-[10px] font-mono cursor-pointer hover:underline"
                                style="color: {workItem.is_personal ? 'var(--ds-accent-orange)' : 'var(--ds-accent-blue)'};"
                              >
                                {workItem.workspace_key || 'WORK'}-{workItem.id}
                              </button>
                            </div>
                            <button
                              onclick={(e) => { e.stopPropagation(); removeWorkItem(day.dateKey, workItem.id); }}
                              class="opacity-0 group-hover/item:opacity-100 text-xs"
                              style="color: var(--ds-text-subtlest);"
                            >×</button>
                          </div>
                        </div>

                        <!-- Bottom resize handle -->
                        <div
                          class="resize-handle resize-handle-bottom"
                          onmousedown={(e) => { e.stopPropagation(); e.preventDefault(); startResize(e, workItem, day.dateKey, 'bottom'); }}
                        >
                          <div class="resize-handle-cap resize-handle-cap-left"></div>
                          <div class="resize-handle-cap resize-handle-cap-right"></div>
                        </div>
                      </div>
                    {:else}
                      <!-- Unscheduled item (no time) - show at top -->
                      <div
                        class="mx-1 mt-1 rounded p-2 group/item cursor-grab"
                        class:calendar-item-personal={workItem.is_personal}
                        class:calendar-item-work={!workItem.is_personal}
                        draggable="true"
                        ondragstart={(e) => {
                          isDragging = true;
                          e.dataTransfer.setData('text/plain', JSON.stringify({
                            ...workItem,
                            sourceDateKey: day.dateKey
                          }));
                        }}
                        ondragend={() => { isDragging = false; lastDragOrResizeEnd = Date.now(); }}
                      >
                        <div class="flex flex-col gap-0.5">
                          <div class="flex items-center justify-between gap-1">
                            <button
                              onclick={(e) => { e.stopPropagation(); openWorkItem(workItem); }}
                              class="text-xs font-medium truncate flex-1 text-left cursor-pointer hover:underline"
                              style="color: var(--ds-text);{isPersonalTaskCompleted(workItem) ? ' text-decoration: line-through;' : ''}"
                            >
                              {workItem.title}
                            </button>
                            <button
                              onclick={(e) => { e.stopPropagation(); removeWorkItem(day.dateKey, workItem.id); }}
                              class="opacity-0 group-hover/item:opacity-100 text-xs"
                              style="color: var(--ds-text-subtlest);"
                            >×</button>
                          </div>
                          {#if workItem.dueDate}
                            {@const dueDate = new Date(workItem.dueDate)}
                            {@const isOverdue = dueDate < new Date() && dueDate.toDateString() !== new Date().toDateString()}
                            <p class="text-[10px]" style="color: {isOverdue ? 'var(--ds-text-danger)' : 'var(--ds-text-subtle)'};">
                              Due: {dueDate.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
                            </p>
                          {/if}
                          <button
                            onclick={(e) => { e.stopPropagation(); openWorkItem(workItem); }}
                            class="text-[10px] font-mono cursor-pointer hover:underline text-left"
                            style="color: {workItem.is_personal ? 'var(--ds-accent-orange)' : 'var(--ds-accent-blue)'};"
                          >
                            {workItem.workspace_key || 'WORK'}-{workItem.id}
                          </button>
                        </div>
                      </div>
                    {/if}
                  {/each}
                {/if}

                <!-- Inline task creation input -->
                {#if creatingTaskAt?.dateKey === day.dateKey}
                  <div
                    class="absolute left-1 right-1 rounded z-20 calendar-item-personal"
                    style="top: {creatingTaskAt.top}px; height: 60px;"
                  >
                    <div class="p-2 h-full">
                      <p class="text-[10px] mb-1" style="color: var(--ds-text-subtle);">
                        {creatingTaskAt.time} - {minutesToTime(timeToMinutes(creatingTaskAt.time) + DEFAULT_DURATION)}
                      </p>
                      <!-- svelte-ignore a11y_autofocus -->
                      <input
                        type="text"
                        bind:value={newTaskTitle}
                        placeholder={t('time.calendar.newTaskPlaceholder')}
                        class="w-full text-xs font-medium border-0 outline-none p-0"
                        style="background: transparent; color: var(--ds-text);"
                        autofocus
                        onkeydown={handleInlineTaskKeydown}
                        onblur={handleInlineTaskBlur}
                        onclick={(e) => e.stopPropagation()}
                      />
                    </div>
                  </div>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      </div>
    </div>
  </div>
</div>

<!-- Personal Task Detail Modal -->
{#if showItemModal && selectedItemId}
  <PersonalTaskDetail
    itemId={selectedItemId}
    workspaceId={selectedWorkspaceId}
    onclose={closeItemModal}
    onupdate={closeItemModal}
  />
{/if}

<style>
  /* Custom scrollbar for todo lists */
  .overflow-y-auto::-webkit-scrollbar {
    width: 4px;
  }

  .overflow-y-auto::-webkit-scrollbar-track {
    background: transparent;
  }

  .overflow-y-auto::-webkit-scrollbar-thumb {
    background: var(--ds-border);
    border-radius: 2px;
  }

  .overflow-y-auto::-webkit-scrollbar-thumb:hover {
    background: var(--ds-text-subtlest);
  }

  /* Sidebar hover states */
  .sidebar-close-btn:hover {
    background-color: var(--ds-surface);
  }

  .sidebar-item:hover {
    border-color: var(--ds-interactive) !important;
  }

  /* Drag over styling */
  :global(.drag-over-valid) {
    background-color: var(--ds-accent-blue-subtler) !important;
    border-color: var(--ds-interactive) !important;
  }

  /* Calendar item styles - solid backgrounds with dark mode support */
  .calendar-item-personal {
    background-color: var(--ds-accent-orange-subtler) !important;
    border: 1px solid var(--ds-border) !important;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
  }

  .calendar-item-work {
    background-color: var(--ds-accent-blue-subtler) !important;
    border: 1px solid var(--ds-border) !important;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
  }

  .calendar-item-personal:hover,
  .calendar-item-work:hover {
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.15);
  }

  /* Resize handles - blue gradient like DropIndicator */
  .resize-handle {
    position: absolute;
    left: 4px;
    right: 4px;
    height: 4px;
    background: linear-gradient(90deg, var(--ds-interactive-subtle, #60a5fa), var(--ds-interactive, #2874bb));
    border-radius: 9999px;
    box-shadow: 0 0 0 1px var(--ds-surface-raised, #ffffff), 0 2px 6px rgba(59, 130, 246, 0.25);
    cursor: ns-resize;
    opacity: 0;
    transition: opacity 0.15s ease;
    z-index: 10;
  }

  .resize-handle-top {
    top: -2px;
  }

  .resize-handle-bottom {
    bottom: -2px;
  }

  .calendar-time-item:hover .resize-handle {
    opacity: 1;
  }

  .resize-handle-cap {
    position: absolute;
    top: -2px;
    width: 8px;
    height: 8px;
    background: var(--ds-interactive, #2874bb);
    border-radius: 9999px;
    box-shadow: 0 0 0 1px var(--ds-surface-raised, #ffffff);
  }

  .resize-handle-cap-left {
    left: -4px;
  }

  .resize-handle-cap-right {
    right: -4px;
  }
</style>
