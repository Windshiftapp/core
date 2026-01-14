<script>
  import { onMount } from 'svelte';
  import { Folder, Plus, Edit, Trash2, Tags, X, GripVertical, FileCheck, ChevronDown, ChevronRight, MoreHorizontal } from 'lucide-svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import { api } from '../../api.js';
  import EmptyState from '../../components/EmptyState.svelte';
  import Input from '../../components/Input.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Select from '../../components/Select.svelte';
  import LabelCombobox from '../../pickers/LabelCombobox.svelte';
  import { writable, get } from 'svelte/store';
  import ConfirmDialog from '../../dialogs/ConfirmDialog.svelte';
  import Button from '../../components/Button.svelte';
  import Label from '../../components/Label.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Tooltip from '../../components/Tooltip.svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { createShortcutHandler, getShortcutDisplay, matchesShortcut } from '../../utils/keyboardShortcuts.js';
  import { currentRoute, navigate } from '../../router.js';

  let { workspaceId = null } = $props();

  const testFolders = writable([]);
  const testCases = writable([]);
  const testLabels = writable([]);
  let selectedFolder = $state(null);
  let noFolderCount = $state(0);

  let showFolderForm = $state(false);
  let showCaseForm = $state(false);
  let showLabelsModal = $state(false);
  let showCreateLabelForm = $state(false);
  let editingFolder = $state(null);
  let editingCase = $state(null);
  let selectedTestCase = $state(null);
  let selectedTestCaseLabels = $state([]);
  let labelSearchQuery = $state('');
  let selectedLabelFilterId = $state(null);
  const derivedFolderTree = $derived.by(() => buildFolderTree($testFolders));
  let collapsedFolders = new Set();

  // Two-key shortcut mode for steps navigation (S + 1-9)
  let stepsShortcutMode = $state(false);
  let stepsShortcutTimeout = null;

  // Focus management
  let titleInputRef = null;
  let folderNameInputRef = null;
  
  // Confirmation dialogs
  let showDeleteTestCaseConfirmation = $state(false);
  let showDeleteFolderConfirmation = $state(false);
  let testCaseToDelete = null;
  let folderToDelete = null;
  
  let folderFormData = $state({
    name: '',
    description: '',
    parent_id: '',
    sort_order: 0
  });

  let caseFormData = $state({
    title: '',
    preconditions: '',
    priority: 'medium',
    status: 'active',
    estimated_hours: 0,
    estimated_minutes: 0
  });

  // Priority options for test cases
  const priorityOptions = [
    { value: 'low', label: 'Low', color: '#6B7280' },
    { value: 'medium', label: 'Medium', color: '#3B82F6' },
    { value: 'high', label: 'High', color: '#F59E0B' },
    { value: 'critical', label: 'Critical', color: '#EF4444' }
  ];

  // Status options for test cases
  const statusOptions = [
    { value: 'active', label: 'Active' },
    { value: 'inactive', label: 'Inactive' },
    { value: 'draft', label: 'Draft' }
  ];

  // Helper to get priority color
  function getPriorityColor(priority) {
    const option = priorityOptions.find(p => p.value === priority);
    return option ? option.color : '#6B7280';
  }

  // Helper to convert seconds to hours and minutes
  function secondsToHoursMinutes(seconds) {
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    return { hours, minutes };
  }

  // Helper to convert hours and minutes to seconds
  function hoursMinutesToSeconds(hours, minutes) {
    return (hours * 3600) + (minutes * 60);
  }

  // Format duration for display
  function formatDuration(seconds) {
    if (!seconds || seconds === 0) return null;
    const { hours, minutes } = secondsToHoursMinutes(seconds);
    if (hours > 0 && minutes > 0) return `${hours}h ${minutes}m`;
    if (hours > 0) return `${hours}h`;
    return `${minutes}m`;
  }

  let newLabelData = $state({
    name: '',
    color: '#3B82F6',
    description: ''
  });

  // React to route changes for folder selection
  $effect(() => {
    const route = $currentRoute;
    const folderId = getFolderIdFromRoute(route);
    if (folderId !== selectedFolder) {
      selectedFolder = folderId;
      loadTestCases(folderId);
    }
  });

  onMount(async () => {
    await loadFolders();
    await loadTestCases(selectedFolder);
    await loadLabels();

    // Add keyboard shortcuts using centralized system
    const handleKeyDown = createShortcutHandler({
      addTestCase: showAddCaseForm,
      addFolder: showAddFolderForm
    }, 'testCases');

    document.addEventListener('keydown', handleKeyDown);
    document.addEventListener('keydown', handleStepsKeyboard);

    // Add command palette trigger listener
    const handleCommandPaletteTrigger = (e) => {
      if (e.type === 'trigger-test-case-form') {
        showAddCaseForm();
      }
    };

    window.addEventListener('trigger-test-case-form', handleCommandPaletteTrigger);

    // Cleanup
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.removeEventListener('keydown', handleStepsKeyboard);
      window.removeEventListener('trigger-test-case-form', handleCommandPaletteTrigger);
      clearTimeout(stepsShortcutTimeout);
    };
  });

  async function loadFolders() {
    try {
      const folders = await api.tests.testFolders.getAll(workspaceId);
      testFolders.set(folders || []);
      
      // Also get count of test cases with no folder
      const noFolderCases = await api.tests.testCases.getAll(workspaceId, { folder_id: null });
      noFolderCount = (noFolderCases || []).length;
    } catch (error) {
      console.error('Failed to load test folders:', error);
    }
  }

  async function loadTestCases(folderId = null) {
    try {
      const params = { folder_id: folderId };
      const cases = await api.tests.testCases.getAll(workspaceId, params);
      testCases.set(cases || []);
    } catch (error) {
      console.error('Failed to load test cases:', error);
    }
  }

  async function loadLabels() {
    try {
      const labels = await api.tests.testLabels.getAll(workspaceId);
      testLabels.set(labels || []);
    } catch (error) {
      console.error('Failed to load test labels:', error);
    }
  }

  function showAddFolderForm() {
    showFolderForm = true;
    editingFolder = null;
    folderFormData = {
      name: '',
      description: '',
      parent_id: getDefaultParentSelection(),
      sort_order: 0
    };
    // Focus the name input after modal opens
    setTimeout(() => {
      if (folderNameInputRef) {
        folderNameInputRef.focus();
      }
    }, 100);
  }

  function showEditFolderForm(folder) {
    showFolderForm = true;
    editingFolder = folder;
    folderFormData = {
      name: folder.name,
      description: folder.description,
      parent_id: folder.parent_id != null ? String(folder.parent_id) : '',
      sort_order: folder.sort_order || 0
    };
    // Focus the name input after modal opens
    setTimeout(() => {
      if (folderNameInputRef) {
        folderNameInputRef.focus();
      }
    }, 100);
  }

  function getDefaultParentSelection() {
    if (selectedFolder === null || selectedFolder === undefined) {
      return '';
    }
    const currentFolder = $testFolders.find(folder => folder.id === selectedFolder);
    if (currentFolder && (currentFolder.parent_id === null || currentFolder.parent_id === undefined)) {
      return String(currentFolder.id);
    }
    return '';
  }

  function showAddCaseForm() {
    showCaseForm = true;
    editingCase = null;
    caseFormData = {
      title: '',
      preconditions: '',
      priority: 'medium',
      status: 'active',
      estimated_hours: 0,
      estimated_minutes: 0
    };

    // Auto-focus the title field after the modal renders
    setTimeout(() => {
      if (titleInputRef) {
        titleInputRef.focus();
      }
    }, 100);
  }

  function showEditCaseForm(testCase) {
    showCaseForm = true;
    editingCase = testCase;
    const { hours, minutes } = secondsToHoursMinutes(testCase.estimated_duration || 0);
    caseFormData = {
      title: testCase.title,
      preconditions: testCase.preconditions || '',
      priority: testCase.priority || 'medium',
      status: testCase.status || 'active',
      estimated_hours: hours,
      estimated_minutes: minutes
    };

    // Auto-focus the title field after the modal renders
    setTimeout(() => {
      if (titleInputRef) {
        titleInputRef.focus();
      }
    }, 100);
  }

  async function handleFolderSubmit() {
    try {
      const parsedParentId = folderFormData.parent_id === '' ? null : Number(folderFormData.parent_id);
      const payload = {
        name: folderFormData.name,
        description: folderFormData.description,
        parent_id: Number.isNaN(parsedParentId) ? null : parsedParentId,
        sort_order: folderFormData.sort_order
      };

      if (editingFolder) {
        await api.tests.testFolders.update(workspaceId, editingFolder.id, payload);
      } else {
        await api.tests.testFolders.create(workspaceId, payload);
      }
      await loadFolders();
      showFolderForm = false;
    } catch (error) {
      console.error('Failed to save folder:', error);
    }
  }

  async function handleCaseSubmit() {
    try {
      const payload = {
        title: caseFormData.title,
        preconditions: caseFormData.preconditions,
        priority: caseFormData.priority,
        status: caseFormData.status,
        estimated_duration: hoursMinutesToSeconds(
          parseInt(caseFormData.estimated_hours) || 0,
          parseInt(caseFormData.estimated_minutes) || 0
        ),
        folder_id: selectedFolder
      };

      if (editingCase) {
        await api.tests.testCases.update(workspaceId, editingCase.id, payload);
      } else {
        await api.tests.testCases.create(workspaceId, payload);
      }

      // Reload both test cases and folders to update counts
      await loadTestCases(selectedFolder);
      await loadFolders();
      showCaseForm = false;
    } catch (error) {
      console.error('Failed to save test case:', error);
    }
  }

  function deleteFolder(id) {
    folderToDelete = id;
    showDeleteFolderConfirmation = true;
  }

  async function confirmDeleteFolder() {
    try {
      await api.tests.testFolders.delete(workspaceId, folderToDelete);
      await loadFolders();
      if (selectedFolder === folderToDelete) {
        selectedFolder = null;
        await loadTestCases();
      }
    } catch (error) {
      console.error('Failed to delete folder:', error);
    } finally {
      folderToDelete = null;
    }
  }

  function deleteTestCase(id) {
    testCaseToDelete = id;
    showDeleteTestCaseConfirmation = true;
  }

  async function confirmDeleteTestCase() {
    try {
      await api.tests.testCases.delete(workspaceId, testCaseToDelete);
      // Reload both test cases and folders to update counts
      await loadTestCases(selectedFolder);
      await loadFolders();
    } catch (error) {
      console.error('Failed to delete test case:', error);
    } finally {
      testCaseToDelete = null;
    }
  }

  async function selectFolder(folderId) {
    if (selectedFolder === folderId) {
      updateFolderQueryParam(folderId);
      return;
    }
    selectedFolder = folderId;
    updateFolderQueryParam(folderId);
    await loadTestCases(folderId);
  }


  // Label Management
  async function openLabelsModal(testCase) {
    selectedTestCase = testCase;
    try {
      const labels = await api.tests.testCases.labels.getAll(workspaceId, testCase.id);
      selectedTestCaseLabels = labels || [];
      showLabelsModal = true;
    } catch (error) {
      console.error('Failed to load test case labels:', error);
      selectedTestCaseLabels = [];
      showLabelsModal = true;
    }
  }

  function closeLabelsModal() {
    selectedTestCase = null;
    selectedTestCaseLabels = [];
    showLabelsModal = false;
  }

  async function addLabelToTestCase(labelId) {
    try {
      await api.tests.testCases.labels.add(workspaceId, selectedTestCase.id, labelId);
      // Reload labels for this test case
      const labels = await api.tests.testCases.labels.getAll(workspaceId, selectedTestCase.id);
      selectedTestCaseLabels = labels || [];
      // Reload test cases to update display
      await loadTestCases(selectedFolder);
    } catch (error) {
      console.error('Failed to add label to test case:', error);
    }
  }

  async function removeLabelFromTestCase(labelId) {
    try {
      await api.tests.testCases.labels.remove(workspaceId, selectedTestCase.id, labelId);
      // Reload labels for this test case
      const labels = await api.tests.testCases.labels.getAll(workspaceId, selectedTestCase.id);
      selectedTestCaseLabels = labels || [];
      // Reload test cases to update display
      await loadTestCases(selectedFolder);
    } catch (error) {
      console.error('Failed to remove label from test case:', error);
    }
  }

  function isLabelAssigned(labelId) {
    return selectedTestCaseLabels.some(label => label.id === labelId);
  }

  // Label creation
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
      await loadLabels(); // Refresh the labels store
      showCreateLabelForm = false;
      // Reset form data
      newLabelData = {
        name: '',
        color: '#3B82F6',
        description: ''
      };
    } catch (error) {
      console.error('Failed to create label:', error);
    }
  }

  // Filter labels based on search query
  function filteredLabels(labels, searchQuery) {
    if (!searchQuery) return labels;
    return labels.filter(label => 
      label.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      (label.description && label.description.toLowerCase().includes(searchQuery.toLowerCase()))
    );
  }

  // Filter test cases by selected label ID
  function filteredTestCases(testCases, selectedLabelId) {
    if (!selectedLabelId) return testCases;
    return testCases.filter(testCase =>
      testCase.labels && testCase.labels.some(label => label.id === selectedLabelId)
    );
  }

  // Build dropdown menu items for test case actions
  function buildTestCaseActions(testCase) {
    return [
      {
        id: 'labels',
        icon: Tags,
        title: 'Labels',
        onClick: () => openLabelsModal(testCase)
      },
      {
        id: 'edit',
        icon: Edit,
        title: 'Edit',
        onClick: () => showEditCaseForm(testCase)
      },
      { type: 'divider' },
      {
        id: 'delete',
        icon: Trash2,
        title: 'Delete',
        color: '#dc2626',
        onClick: () => deleteTestCase(testCase.id)
      }
    ];
  }

  // Custom keyboard handler for two-key steps navigation (S + 1-9)
  function handleStepsKeyboard(event) {
    // Ignore if typing in input
    if (event.target.tagName === 'INPUT' || event.target.tagName === 'TEXTAREA') {
      return;
    }

    const filteredCases = filteredTestCases($testCases, selectedLabelFilterId);

    if (stepsShortcutMode) {
      // In steps mode - waiting for number key
      const num = parseInt(event.key);
      if (num >= 1 && num <= 9 && num <= filteredCases.length) {
        event.preventDefault();
        const testCase = filteredCases[num - 1];
        navigate(`/workspaces/${workspaceId}/tests/cases/${testCase.id}/steps`);
      }
      // Exit mode on any key
      stepsShortcutMode = false;
      clearTimeout(stepsShortcutTimeout);
      return;
    }

    if (matchesShortcut(event, { key: 's' })) {
      event.preventDefault();
      stepsShortcutMode = true;
      // Auto-exit after 2 seconds
      stepsShortcutTimeout = setTimeout(() => {
        stepsShortcutMode = false;
      }, 2000);
    }
  }

  function buildFolderTree(folders = []) {
    const folderMap = new Map();
    (folders || []).forEach(folder => {
      folderMap.set(folder.id, {
        ...folder,
        children: [],
        total_case_count: folder.test_case_count || 0
      });
    });

    folderMap.forEach(folder => {
      if (folder.parent_id && folderMap.has(folder.parent_id)) {
        folderMap.get(folder.parent_id).children.push(folder);
      }
    });

    const roots = [];
    folderMap.forEach(folder => {
      if (!folder.parent_id || !folderMap.has(folder.parent_id)) {
        roots.push(folder);
      }
    });

    const sortNodes = (nodes) => {
      nodes.sort((a, b) => {
        const orderDiff = (a.sort_order || 0) - (b.sort_order || 0);
        if (orderDiff !== 0) return orderDiff;
        return a.name.localeCompare(b.name);
      });
      nodes.forEach(child => sortNodes(child.children));
    };

    const computeTotals = (node) => {
      const childTotal = node.children.reduce((sum, child) => sum + computeTotals(child), 0);
      node.total_case_count = (node.test_case_count || 0) + childTotal;
      return node.total_case_count;
    };

    sortNodes(roots);
    roots.forEach(node => computeTotals(node));
    return roots;
  }

  function flattenFolderTree(tree = [], collapsed = new Set()) {
    const result = [];
    const traverse = (nodes, depth = 0) => {
      nodes.forEach(node => {
        result.push({ node, depth });
        if (node.children && node.children.length > 0 && !collapsed.has(node.id)) {
          traverse(node.children, depth + 1);
        }
      });
    };
    traverse(tree, 0);
    return result;
  }

  function toggleFolderCollapse(folderId) {
    if (collapsedFolders.has(folderId)) {
      collapsedFolders.delete(folderId);
    } else {
      collapsedFolders.add(folderId);
    }
    // Reassign to trigger reactivity
    collapsedFolders = new Set(collapsedFolders);
  }

  function isFolderCollapsed(folderId) {
    return collapsedFolders.has(folderId);
  }

  function getFolderPath(folderId, folders = []) {
    if (folderId === null || folderId === undefined) {
      return null;
    }
    const folder = folders.find(f => f.id === folderId);
    if (!folder) return null;
    if (folder.parent_id) {
      const parent = folders.find(f => f.id === folder.parent_id);
      return parent ? `${parent.name} / ${folder.name}` : folder.name;
    }
    return folder.name;
  }

  function getFolderDisplayCount(folder, depth) {
    if (depth === 0) {
      return folder.total_case_count ?? folder.test_case_count ?? 0;
    }
    return folder.test_case_count ?? 0;
  }

  function getFolderIndent(depth = 0) {
    const base = 12;
    const step = 16;
    return `${base + depth * step}px`;
  }

  function getFolderIdFromRoute(route) {
    if (!route || !route.path || !route.path.includes('/tests')) {
      return null;
    }
    const rawValue = route.query?.folder;
    if (rawValue === undefined || rawValue === '' || rawValue === 'unassigned') {
      return null;
    }
    const parsed = Number(rawValue);
    return Number.isNaN(parsed) ? null : parsed;
  }

  async function applyFolderSelectionFromRoute(folderId) {
    selectedFolder = folderId;
    await loadTestCases(folderId);
  }

  function updateFolderQueryParam(folderId) {
    if (typeof window === 'undefined') {
      return;
    }
    const url = new URL(window.location.href);
    const currentFolderParam = url.searchParams.get('folder');
    if (folderId === null || folderId === undefined) {
      if (!url.searchParams.has('folder')) {
        return;
      }
      url.searchParams.delete('folder');
    } else {
      const nextValue = String(folderId);
      if (currentFolderParam === nextValue) {
        return;
      }
      url.searchParams.set('folder', nextValue);
    }
    const nextSearch = url.searchParams.toString();
    const nextPath = `${url.pathname}${nextSearch ? `?${nextSearch}` : ''}`;
    const currentPathWithSearch = `${window.location.pathname}${window.location.search}`;
    if (nextPath !== currentPathWithSearch) {
      navigate(nextPath);
    }
  }

  const flattenedFolders = $derived.by(() => flattenFolderTree(derivedFolderTree, collapsedFolders));
  const rootFolderOptions = $derived.by(() => ($testFolders || [])
    .filter(folder => folder.parent_id === null || folder.parent_id === undefined)
    .sort((a, b) => {
      const orderDiff = (a.sort_order || 0) - (b.sort_order || 0);
      if (orderDiff !== 0) return orderDiff;
      return a.name.localeCompare(b.name);
    }));
  const folderSubtitle = $derived.by(() => selectedFolder === null
    ? 'Showing test cases with no folder assignment'
    : selectedFolder
      ? `Showing test cases in folder "${getFolderPath(selectedFolder, $testFolders) || 'Selected folder'}"`
      : 'Showing all test cases');
  $effect(() => {
    if ($currentRoute.path && $currentRoute.path.includes('/tests')) {
      const folderFromRoute = getFolderIdFromRoute($currentRoute);
      if (folderFromRoute !== selectedFolder) {
        applyFolderSelectionFromRoute(folderFromRoute);
      }
    }
  });

  // Drag and drop functions
  async function handleTestCaseMove(testCaseId, targetFolderId) {
    console.log('handleTestCaseMove called:', { testCaseId, targetFolderId });
    try {
      // Use the dedicated move endpoint that only requires folder_id
      const result = await api.tests.testCases.move(workspaceId, testCaseId, {
        folder_id: targetFolderId,
        sort_order: 1000  // Default sort order for moved items
      });
      console.log('Move API response:', result);

      // Reload data to reflect changes
      await loadTestCases(selectedFolder);
      await loadFolders();
      console.log('Reload complete');
    } catch (error) {
      console.error('Failed to move test case:', error);
    }
  }

  // Svelte action to make test case rows draggable
  function makeDraggable(element, { testCase }) {
    // Find the drag handle within the element
    const dragHandle = element.querySelector('.drag-handle');
    
    if (!dragHandle) {
      console.warn('Drag handle not found');
      return { destroy: () => {} };
    }

    const cleanup = draggable({
      element: dragHandle,
      getInitialData: () => ({
        type: 'test-case',
        testCaseId: testCase.id,
        testCaseTitle: testCase.title,
        currentFolderId: testCase.folder_id ?? null
      }),
      onDragStart: () => {
        element.style.opacity = '0.5';
      },
      onDrop: () => {
        element.style.opacity = '1';
      }
    });

    return {
      destroy: cleanup
    };
  }

  // Svelte action to make folder buttons drop targets
  function makeDropTarget(element, { folderId }) {
    let isDropTarget = false;
    
    const cleanup = dropTargetForElements({
      element,
      canDrop: ({ source }) => source.data.type === 'test-case',
      onDragEnter: () => {
        isDropTarget = true;
        element.style.backgroundColor = 'var(--ds-interactive-subtle)';
        element.style.borderColor = 'var(--ds-interactive)';
      },
      onDragLeave: () => {
        isDropTarget = false;
        element.style.backgroundColor = '';
        element.style.borderColor = '';
      },
      onDrop: ({ source }) => {
        isDropTarget = false;
        element.style.backgroundColor = '';
        element.style.borderColor = '';

        const testCaseId = source.data.testCaseId;
        const currentFolderId = source.data.currentFolderId;

        console.log('Drop detected:', { testCaseId, currentFolderId, targetFolderId: folderId, willMove: currentFolderId !== folderId });

        // Only move if dropping on a different folder
        if (currentFolderId !== folderId) {
          handleTestCaseMove(testCaseId, folderId);
        }
      }
    });

    return {
      destroy: cleanup
    };
  }
</script>

<div class="min-h-screen flex flex-col p-6" style="background-color: var(--ds-surface-raised);">
  <PageHeader
    title="Test Cases"
    subtitle={folderSubtitle}
  >
    {#snippet actions()}
      <div class="flex items-center gap-3">
        <div class="w-48">
          <LabelCombobox
            bind:value={selectedLabelFilterId}
            placeholder="All labels"
            {workspaceId}
          />
        </div>
        <Button
          onclick={showAddCaseForm}
          variant="primary"
          icon={Plus}
          size="medium"
          keyboardHint={getShortcutDisplay('testCases', 'addTestCase')}
        >
          Add Test Case
        </Button>
      </div>
    {/snippet}
  </PageHeader>

  <div class="flex flex-1 -mx-6 -mb-6">
  <!-- Left Sidebar - Folders -->
  <div class="w-72 flex-shrink-0 border-r px-4 py-6" style="border-color: var(--ds-border);">
    <div class="space-y-1">
        <!-- No Folder -->
        <div class="group relative">
          <button
            onclick={() => selectFolder(null)}
            class="w-full flex items-center px-3 py-2 text-sm font-medium transition-all cursor-pointer rounded-lg"
            style={selectedFolder === null ? 'background: var(--ds-background-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
            onmouseenter={(e) => { if (selectedFolder !== null) e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'; }}
            onmouseleave={(e) => { if (selectedFolder !== null) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
            use:makeDropTarget={{ folderId: null }}
          >
            <Folder size="16" class="mr-2 flex-shrink-0" />
            <span class="flex-1 text-left">No Folder</span>
            <span class="text-xs min-w-[20px] text-right" style="color: var(--ds-text-subtle);">
              {noFolderCount}
            </span>
          </button>
        </div>

        <!-- Regular Folders -->
        {#each flattenedFolders as { node: folder, depth } (folder.id)}
          {@const isFolderActive = selectedFolder === folder.id}
          <div class="group relative">
            <button
              onclick={() => selectFolder(folder.id)}
              class="w-full flex items-center py-2 pr-3 text-sm font-medium transition-all cursor-pointer rounded-lg"
              style={isFolderActive ? `background: var(--ds-background-selected); color: var(--ds-text); padding-left: ${getFolderIndent(depth)};` : `color: var(--ds-text-subtle); padding-left: ${getFolderIndent(depth)};`}
              onmouseenter={(e) => { if (!isFolderActive) e.currentTarget.style.cssText = `background: var(--ds-background-neutral-hovered); color: var(--ds-text); padding-left: ${getFolderIndent(depth)};`; }}
              onmouseleave={(e) => { if (!isFolderActive) e.currentTarget.style.cssText = `color: var(--ds-text-subtle); padding-left: ${getFolderIndent(depth)};`; }}
              use:makeDropTarget={{ folderId: folder.id }}
            >
              {#if folder.children && folder.children.length > 0}
                <button
                  type="button"
                  class="mr-1 inline-flex h-5 w-5 items-center justify-center cursor-pointer bg-transparent border-0 p-0"
                  style="color: var(--ds-icon-subtle);"
                  aria-label={isFolderCollapsed(folder.id) ? 'Expand folder' : 'Collapse folder'}
                  onclick={(e) => { e.stopPropagation(); toggleFolderCollapse(folder.id); }}
                >
                  {#if isFolderCollapsed(folder.id)}
                    <ChevronRight size="16" />
                  {:else}
                    <ChevronDown size="16" />
                  {/if}
                </button>
              {:else}
                <span class="inline-block w-5 mr-1"></span>
              {/if}
              <Folder size="16" class="mr-2 flex-shrink-0" />
              <Tooltip content={folder.name} class="flex-1 min-w-0 text-left">
                {#snippet children()}
                  <span class="block truncate">
                    {#if depth > 0}
                      <span class="mr-1" style="color: var(--ds-text-subtlest);">↳</span>
                    {/if}
                    {folder.name}
                  </span>
                {/snippet}
              </Tooltip>
              <div class="flex items-center gap-1">
                {#if selectedFolder === folder.id}
                  <div
                    onclick={(e) => { e.stopPropagation(); showEditFolderForm(folder); }}
                    class="p-1 cursor-pointer rounded"
                    style="color: var(--ds-icon-subtle);"
                    role="button"
                    tabindex="0"
                    onkeydown={(e) => e.key === 'Enter' && showEditFolderForm(folder)}
                    onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-interactive)'}
                    onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-icon-subtle)'}
                  >
                    <Edit size="12" />
                  </div>
                  <div
                    onclick={(e) => { e.stopPropagation(); deleteFolder(folder.id); }}
                    class="p-1 cursor-pointer rounded"
                    style="color: var(--ds-icon-subtle);"
                    role="button"
                    tabindex="0"
                    onkeydown={(e) => e.key === 'Enter' && deleteFolder(folder.id)}
                    onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-danger)'}
                    onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-icon-subtle)'}
                  >
                    <Trash2 size="12" />
                  </div>
                {/if}
                <span class="text-xs min-w-[20px] text-right" style="color: var(--ds-text-subtle);">
                  {getFolderDisplayCount(folder, depth)}
                </span>
              </div>
            </button>
          </div>
        {/each}
        
        <!-- Add Folder Button -->
        <div class="pt-2">
          <button
            onclick={showAddFolderForm}
            class="w-full flex items-center px-3 py-2 text-sm font-medium transition cursor-pointer rounded hover:bg-[var(--ds-surface-hovered)]"
            style="color: var(--ds-text-subtle);"
          >
            <Plus size="16" class="mr-2 flex-shrink-0" />
            <span class="flex-1 text-left">Add folder</span>
            <kbd class="ml-auto px-1.5 py-0.5 text-xs rounded border" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border-bold); color: var(--ds-text-subtle);">
              {getShortcutDisplay('testCases', 'addFolder')}
            </kbd>
          </button>
        </div>
      </div>
    </div>

  <!-- Right Content - Test Cases -->
  <div class="flex-1 min-w-0 px-10 py-6">
    <table class="min-w-full text-sm">
      <thead style="border-bottom: 1px solid var(--ds-border);">
            <tr>
              <th class="px-2 py-3 w-10"></th>
              <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide" style="color: var(--ds-text-subtle);">Title</th>
              <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide" style="color: var(--ds-text-subtle);">Labels</th>
              <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wide" style="color: var(--ds-text-subtle);">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each filteredTestCases($testCases, selectedLabelFilterId) as testCase, index}
              <tr
                class="hover:bg-[var(--ds-surface)] transition-colors draggable-test-case"
                style="border-top: 1px solid var(--ds-border);"
                data-test-case-id={testCase.id}
                use:makeDraggable={{ testCase }}
                ondblclick={() => showEditCaseForm(testCase)}
              >
                <td class="px-2 py-3 text-center">
                  <div class="drag-handle cursor-grab active:cursor-grabbing flex justify-center items-center" style="color: var(--ds-text-subtle);">
                    <GripVertical size="16" />
                  </div>
                </td>
                <td class="px-4 py-3 text-sm font-medium" style="color: {testCase.status === 'inactive' ? 'var(--ds-text-disabled)' : 'var(--ds-text)'};">
                  <div class="flex items-center gap-2">
                    <!-- Priority badge -->
                    <span
                      class="inline-flex items-center px-1.5 py-0.5 text-xs font-medium rounded text-white capitalize"
                      style="background-color: {getPriorityColor(testCase.priority || 'medium')};"
                    >
                      {testCase.priority || 'medium'}
                    </span>
                    <!-- Status badge for draft -->
                    {#if testCase.status === 'draft'}
                      <span class="inline-flex items-center px-1.5 py-0.5 text-xs font-medium rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                        Draft
                      </span>
                    {/if}
                    <span class={testCase.status === 'inactive' ? 'line-through' : ''}>{testCase.title}</span>
                    <!-- Duration badge -->
                    {#if formatDuration(testCase.estimated_duration)}
                      <span class="text-xs" style="color: var(--ds-text-subtle);">
                        ({formatDuration(testCase.estimated_duration)})
                      </span>
                    {/if}
                  </div>
                  {#if testCase.preconditions}
                    <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                      Preconditions: {testCase.preconditions}
                    </div>
                  {/if}
                </td>
                <td class="px-4 py-3 text-sm">
                  <div class="flex flex-wrap gap-1">
                    {#if testCase.labels && testCase.labels.length > 0}
                      {#each testCase.labels as label}
                        <span 
                          class="inline-flex items-center px-2 py-1 text-xs font-medium rounded-full text-white"
                          style="background-color: {label.color};"
                        >
                          {label.name}
                        </span>
                      {/each}
                    {:else}
                      <span class="text-xs" style="color: var(--ds-text-subtle);">No labels</span>
                    {/if}
                  </div>
                </td>
                <td class="px-4 py-3 text-sm text-right">
                  <div class="flex gap-2 items-center justify-end">
                    <a
                      href={`/workspaces/${workspaceId}/tests/cases/${testCase.id}/steps`}
                      class="inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded transition-colors"
                      style="background-color: var(--ds-background-neutral); color: var(--ds-text);"
                      onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
                      onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral)'}
                    >
                      Steps
                      <kbd class="px-1 py-0.5 text-[10px] rounded" style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border); color: var(--ds-text-subtle);">
                        {stepsShortcutMode && index < 9 ? index + 1 : 'S'}
                      </kbd>
                    </a>
                    <DropdownMenu
                      triggerIcon={MoreHorizontal}
                      showChevron={false}
                      iconOnly={true}
                      triggerClass="p-1.5 rounded transition-colors hover:bg-[var(--ds-background-neutral-hovered)]"
                      triggerStyle="color: var(--ds-text-subtle);"
                      placement="bottom"
                      items={buildTestCaseActions(testCase)}
                    />
                  </div>
                </td>
              </tr>
            {:else}
              <tr>
                <td colspan="4">
                  <EmptyState
                    icon={FileCheck}
                    title="No test cases found"
                    description={selectedLabelFilterId
                      ? "No test cases found with the selected label filter."
                      : "Click 'Add Test Case' to create your first test case in this folder."}
                  />
                </td>
              </tr>
            {/each}
      </tbody>
    </table>
  </div>
  </div>
</div>

<!-- Steps shortcut mode indicator -->
{#if stepsShortcutMode}
  <div class="fixed bottom-4 left-1/2 -translate-x-1/2 px-4 py-2 rounded-lg shadow-lg z-50"
       style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);">
    <span style="color: var(--ds-text);">Press <kbd class="px-1.5 py-0.5 mx-1 text-xs rounded" style="background-color: var(--ds-background-neutral); border: 1px solid var(--ds-border);">1-9</kbd> to open steps</span>
  </div>
{/if}

<!-- Folder Form Modal -->
<Modal 
  isOpen={showFolderForm} 
  on:close={() => showFolderForm = false}
  maxWidth="max-w-md"
>
  <div class="p-6">
    <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
      {editingFolder ? 'Edit' : 'Add'} Folder
    </h3>
    <form onsubmit={(e) => { e.preventDefault(); handleFolderSubmit(); }}>
      <div class="mb-4">
        <Label color="default" class="mb-2">Name</Label>
        <Input
          bind:value={folderFormData.name}
          required
          size="small"
        />
      </div>
      <div class="mb-4">
        <Label color="default" class="mb-2">Parent folder (optional)</Label>
        <Select bind:value={folderFormData.parent_id} size="small">
          <option value="">Top-level folder</option>
          {#each rootFolderOptions as option}
            <option value={option.id} disabled={editingFolder && option.id === editingFolder.id}>
              {option.name}
            </option>
          {/each}
        </Select>
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
          Subfolders can only be nested one level deep.
        </p>
      </div>
      <div class="mb-6">
        <Label color="default" class="mb-2">Description</Label>
        <Textarea
          bind:value={folderFormData.description}
          rows={3}
          size="small"
        />
      </div>
      <div class="flex gap-2 justify-end">
        <Button
          type="button"
          onclick={() => showFolderForm = false}
          variant="default"
          keyboardHint={getShortcutDisplay('testCases', 'cancelForm')}
        >
          Cancel
        </Button>
        <Button
          type="submit"
          variant="primary"
          size="medium"
          keyboardHint={getShortcutDisplay('testCases', 'submitForm')}
        >
          Save
        </Button>
      </div>
    </form>
  </div>
</Modal>

<!-- Test Case Form Modal -->
<Modal
  isOpen={showCaseForm}
  maxWidth="max-w-2xl"
  onSubmit={handleCaseSubmit}
  on:close={() => showCaseForm = false}
>
  <div class="p-6">
    <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
      {editingCase ? 'Edit' : 'Add'} Test Case
    </h3>
    <form onsubmit={(e) => { e.preventDefault(); handleCaseSubmit(); }}>
      <div class="mb-4">
        <Label color="default" class="mb-2">Title</Label>
        <Input
          bind:value={caseFormData.title}
          required
          size="small"
        />
      </div>

      <!-- Priority, Status, and Duration row -->
      <div class="grid grid-cols-3 gap-4 mb-4">
        <div>
          <Label color="default" class="mb-2">Priority</Label>
          <Select bind:value={caseFormData.priority} size="small">
            {#each priorityOptions as option}
              <option value={option.value}>{option.label}</option>
            {/each}
          </Select>
        </div>
        <div>
          <Label color="default" class="mb-2">Status</Label>
          <Select bind:value={caseFormData.status} size="small">
            {#each statusOptions as option}
              <option value={option.value}>{option.label}</option>
            {/each}
          </Select>
        </div>
        <div>
          <Label color="default" class="mb-2">Estimated Duration</Label>
          <div class="flex items-center gap-2">
            <Input
              type="number"
              min="0"
              bind:value={caseFormData.estimated_hours}
              size="small"
              class="w-16"
            />
            <span class="text-sm" style="color: var(--ds-text-subtle);">h</span>
            <Input
              type="number"
              min="0"
              max="59"
              bind:value={caseFormData.estimated_minutes}
              size="small"
              class="w-16"
            />
            <span class="text-sm" style="color: var(--ds-text-subtle);">m</span>
          </div>
        </div>
      </div>

      <div class="mb-6">
        <Label color="default" class="mb-2">Preconditions</Label>
        <Textarea
          bind:value={caseFormData.preconditions}
          rows={3}
          placeholder="Describe the conditions that must be met before running this test case..."
          size="small"
        />
      </div>

      <!-- Information for new test cases -->
      {#if !editingCase}
        <div class="mb-6">
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            After creating this test case, you can add individual test steps with specific actions, data, and expected results for precise test execution.
          </p>
        </div>
      {/if}
      <div class="flex gap-2 justify-end">
        <Button
          type="button"
          variant="outline"
          onclick={() => showCaseForm = false}
          keyboardHint="Esc"
        >
          Cancel
        </Button>
        <Button
          type="submit"
          variant="primary"
          keyboardHint="↵"
        >
          {editingCase ? 'Save' : 'Create'}
        </Button>
      </div>
    </form>
  </div>
</Modal>


<!-- Delete Test Case Confirmation Dialog -->
<ConfirmDialog
  bind:show={showDeleteTestCaseConfirmation}
  title="Delete Test Case"
  message="Are you sure you want to delete this test case? This action cannot be undone."
  confirmText="Delete Test Case"
  cancelText="Cancel"
  variant="danger"
  on:confirm={confirmDeleteTestCase}
  on:cancel={() => {
    showDeleteTestCaseConfirmation = false;
    testCaseToDelete = null;
  }}
/>

<!-- Delete Folder Confirmation Dialog -->
<ConfirmDialog
  bind:show={showDeleteFolderConfirmation}
  title="Delete Folder"
  message="Are you sure you want to delete this folder? This action cannot be undone."
  confirmText="Delete Folder"
  cancelText="Cancel"
  variant="danger"
  on:confirm={confirmDeleteFolder}
  on:cancel={() => {
    showDeleteFolderConfirmation = false;
    folderToDelete = null;
  }}
/>

<!-- Test Case Labels Modal -->
<Modal
  isOpen={showLabelsModal && selectedTestCase}
  maxWidth="max-w-2xl"
  on:close={closeLabelsModal}
>
  <div class="max-h-[80vh] flex flex-col">
    <!-- Header -->
    <div class="flex items-center justify-between p-6 border-b shrink-0" style="border-color: var(--ds-border);">
      <div>
        <h3 class="text-xl font-semibold" style="color: var(--ds-text);">
          Manage Labels: {selectedTestCase?.title}
        </h3>
        <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">
          Click labels to assign/remove them from this test case
        </div>
      </div>
      <button
        onclick={closeLabelsModal}
        class="p-2 hover:bg-[var(--ds-background-neutral-hovered)] rounded-full transition-colors"
        aria-label="Close labels modal"
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
              Search existing labels
            </Label>
            <Input
              placeholder="Search labels..."
              bind:value={labelSearchQuery}
              size="small"
            />
            <div class="flex items-center justify-between pt-2 text-sm" style="color: var(--ds-text-subtle);">
              <span>Can't find what you need?</span>
              <Button
                variant="ghost"
                onclick={showCreateLabelFormModal}
                icon={Plus}
                size="small"
                style="color: var(--ds-interactive);"
              >
                New Label
              </Button>
            </div>
          </div>

          <!-- Create New Label Form -->
          {#if showCreateLabelForm}
            <div class="bg-gray-50 rounded p-4 border" style="background-color: var(--ds-background-neutral); border-color: var(--ds-border);">
              <h4 class="font-medium mb-3" style="color: var(--ds-text);">Create New Label</h4>
              <form onsubmit={(e) => { e.preventDefault(); handleCreateLabel(); }} class="space-y-3">
                <div>
                  <Label class="block text-xs font-medium mb-1">Name</Label>
                  <Input
                    bind:value={newLabelData.name}
                    required
                    placeholder="Enter label name..."
                    size="small"
                  />
                </div>
                <div class="flex gap-3">
                  <div class="flex-1">
                    <Label class="block text-xs font-medium mb-1">Color</Label>
                    <div class="flex items-center gap-3">
                      <!-- Color Preview Circle -->
                      <div
                        class="w-8 h-8 rounded-full border-2 flex-shrink-0"
                        style="background-color: {newLabelData.color}; border-color: var(--ds-border-bold);"
                      ></div>
                      
                      <!-- Color Palette -->
                      <div class="flex flex-wrap gap-1.5">
                        {#each ['#EF4444', '#F59E0B', '#10B981', '#3B82F6', '#8B5CF6', '#EC4899', '#6B7280', '#DC2626', '#F97316', '#059669', '#0EA5E9', '#7C3AED', '#DB2777', '#4B5563'] as color}
                          <button
                            type="button"
                            onclick={() => newLabelData.color = color}
                            class="w-6 h-6 rounded-full border-2 transition-all hover:scale-110 {newLabelData.color === color ? 'ring-2' : ''}"
                            style="background-color: {color}; border-color: {newLabelData.color === color ? 'var(--ds-border-bold)' : 'var(--ds-border)'}; {newLabelData.color === color ? '--tw-ring-color: var(--ds-border);' : ''}"
                            aria-label="Select color {color}"
                          ></button>
                        {/each}
                        
                        <!-- Custom Color Input -->
                        <div class="relative">
                          <input
                            type="color"
                            bind:value={newLabelData.color}
                            class="w-6 h-6 rounded-full border-2 cursor-pointer opacity-0 absolute inset-0"
                            style="border-color: var(--ds-border);"
                            aria-label="Custom color picker"
                          />
                          <div class="w-6 h-6 rounded-full border-2 cursor-pointer flex items-center justify-center text-xs font-bold" style="border-color: var(--ds-border); color: var(--ds-text-subtle); background: linear-gradient(45deg, #ff0000 25%, #ffff00 25%, #ffff00 50%, #00ff00 50%, #00ff00 75%, #0000ff 75%);">
                            +
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                  <div class="flex-2">
                    <Label class="block text-xs font-medium mb-1">Description</Label>
                    <Input
                      bind:value={newLabelData.description}
                      placeholder="Optional description..."
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
                    Create
                  </Button>
                  <Button
                    type="button"
                    variant="default"
                    onclick={() => showCreateLabelForm = false}
                    size="small"
                  >
                    Cancel
                  </Button>
                </div>
              </form>
            </div>
          {/if}

          <!-- Labels List -->
          {#if $testLabels && $testLabels.length > 0}
            {@const filtered = filteredLabels($testLabels, labelSearchQuery)}
            {#if filtered.length > 0}
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                {#each filtered as label}
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
                        Assigned
                      </div>
                    {:else}
                      <div class="text-xs px-2 py-1 rounded" style="color: var(--ds-text-subtle); background-color: var(--ds-background-neutral);">
                        Click to assign
                      </div>
                    {/if}
                  </button>
                {/each}
              </div>
            {:else}
              <EmptyState
                icon={Tags}
                title="No labels match your search"
                description="Try adjusting your search or create a new label."
              />
            {/if}
          {:else}
            <EmptyState
              icon={Tags}
              title="No labels available"
              description="Create your first label to get started with organizing your test cases."
            />
          {/if}
        </div>
      </div>

    <!-- Footer -->
    <div class="border-t p-4 shrink-0" style="border-color: var(--ds-border); background-color: var(--ds-background-neutral);">
      <div class="flex justify-between items-center">
        <div class="text-sm" style="color: var(--ds-text-subtle);">
          {selectedTestCaseLabels.length} label{selectedTestCaseLabels.length !== 1 ? 's' : ''} assigned
        </div>
        <Button
          onclick={closeLabelsModal}
          variant="primary"
          size="medium"
        >
          Done
        </Button>
      </div>
    </div>
  </div>
</Modal>
