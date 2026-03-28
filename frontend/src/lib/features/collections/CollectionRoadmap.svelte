<script>
  import { onMount } from 'svelte';
  import { untrack } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { collectionStore, reloadCollection, refreshCollectionItem } from '../../stores/collectionContext.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import { workspaceDataStore } from '../../stores/index.js';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import Select from '../../components/Select.svelte';
  import ItemDetail from '../items/ItemDetail.svelte';
  import { Settings, ChevronLeft, ChevronRight, Diamond, ChevronDown, Circle, GitBranch } from 'lucide-svelte';
  import { getVisibleColor } from '../../utils/colorUtils.js';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import { SYSTEM_FIELDS } from '../../stores/fieldConfig.js';
    import Button from '../../components/Button.svelte';

  // Props
  let { workspaceId, collectionId = null } = $props();

  // Reference data from shared workspace store
  let workspace = $derived(workspaceDataStore.workspace);
  let statuses = $derived(workspaceDataStore.statuses);
  let itemTypes = $derived(workspaceDataStore.itemTypes || []);

  // Gradient
  const styles = useGradientStyles();

  // State
  let loading = $state(true);
  let currentCollectionName = $state('Default');

  // Settings
  let showSettings = $state(false);
  let boardConfig = $state(null);
  let boardConfigId = $state(null);
  let roadmapConfig = $state({ start_field_id: 'due_date', end_field_id: '', dependency_link_type_id: null });
  let linkTypes = $state([]);
  let customFields = $state([]);
  let screenFields = $state([]);

  // Zoom: 'week' | 'month' | 'quarter'
  let zoom = $state('month');

  // Timeline scroll offset (in days from reference date)
  let scrollOffset = $state(0);

  // Drag state (bar move/resize)
  let dragInfo = $state(null);

  // Item detail modal
  let showItemModal = $state(false);
  let selectedItemId = $state(null);

  // Links/dependencies
  let itemLinks = $state({});

  // Timeline computation
  let timelineContainer = $state(null);
  let containerWidth = $state(1200);

  // --- Tree panel state ---
  let treePanelWidth = $state(280);
  let isResizingPanel = $state(false);
  let resizeStartX = $state(0);
  let resizeStartWidth = $state(0);
  let expandedItems = $state(new Set());
  let scheduleDragInfo = $state(null); // { itemId, itemTitle, startX, startY, currentX, currentY, active }
  let treeScrollContainer = $state(null);
  let timelineScrollContainer = $state(null);

  // Derived: date field options from screen-configured fields
  let dateFieldOptions = $derived.by(() => {
    const opts = [];
    for (const sf of screenFields) {
      if (sf.field_type === 'system') {
        const sysDef = SYSTEM_FIELDS.find(f => f.identifier === sf.field_identifier);
        if (sysDef?.type === 'date') {
          opts.push({ value: sysDef.identifier, label: sysDef.name });
        }
      } else if (sf.field_type === 'custom' && sf.custom_field_id) {
        const cf = customFields.find(c => String(c.id) === String(sf.custom_field_id));
        if (cf?.field_type === 'date') {
          opts.push({ value: `cf_${cf.id}`, label: cf.name });
        }
      }
    }
    return opts;
  });

  let linkTypeOptions = $derived.by(() => {
    const opts = [{ value: '', label: t('collections.roadmapNone') }];
    for (const lt of linkTypes) {
      opts.push({ value: String(lt.id), label: lt.name });
    }
    return opts;
  });

  // Zoom config
  let zoomConfig = $derived.by(() => {
    if (zoom === 'week') return { columnDays: 1, headerFormat: 'week', visibleColumns: 42 };
    if (zoom === 'quarter') return { columnDays: 30, headerFormat: 'quarter', visibleColumns: 12 };
    return { columnDays: 7, headerFormat: 'month', visibleColumns: 20 };
  });

  // Snap granularity in days: week/month zoom → 1 day, quarter zoom → 7 days
  let snapDays = $derived(zoom === 'quarter' ? 7 : 1);

  // Reference date (start of visible range)
  let referenceDate = $derived.by(() => {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const d = new Date(today);
    if (zoom === 'quarter') {
      d.setDate(1);
      d.setMonth(d.getMonth() + scrollOffset);
      return d;
    }
    d.setDate(d.getDate() + scrollOffset * zoomConfig.columnDays);
    if (zoom === 'week' || zoom === 'month') {
      const day = d.getDay();
      const diff = day === 0 ? -6 : 1 - day;
      d.setDate(d.getDate() + diff);
    }
    return d;
  });

  // Generate column dates (extended to cover all items in both directions)
  let columns = $derived.by(() => {
    let rightCount = zoomConfig.visibleColumns;
    let prependCount = 0;

    if (!collectionStore.loading && roadmapConfig.start_field_id) {
      for (const item of collectionStore.items) {
        const startVal = getDateValue(item, roadmapConfig.start_field_id);
        const endVal = roadmapConfig.end_field_id ? getDateValue(item, roadmapConfig.end_field_id) : null;
        if (startVal) {
          const pos = dateToColPos(new Date(startVal));
          if (pos < -prependCount) prependCount = Math.ceil(Math.abs(pos)) + 1;
        }
        const farVal = endVal || startVal;
        if (farVal) {
          const d = new Date(farVal);
          d.setDate(d.getDate() + 1);
          const pos = dateToColPos(d);
          if (pos > rightCount - 1) rightCount = Math.ceil(pos) + 2;
        }
      }
    }

    const cols = [];
    for (let i = -prependCount; i < rightCount; i++) {
      const d = new Date(referenceDate);
      if (zoom === 'quarter') {
        d.setMonth(referenceDate.getMonth() + i);
      } else {
        d.setDate(d.getDate() + i * zoomConfig.columnDays);
      }
      cols.push(d);
    }
    return cols;
  });

  // Grid offset: number of prepended columns before referenceDate
  let gridOffset = $derived.by(() => {
    if (columns.length === 0) return 0;
    return -dateToColPos(columns[0]);
  });

  // End date of visible range
  let endDate = $derived.by(() => {
    if (columns.length === 0) return referenceDate;
    const last = columns[columns.length - 1];
    const d = new Date(last);
    if (zoom === 'quarter') {
      d.setMonth(d.getMonth() + 1);
    } else {
      d.setDate(d.getDate() + zoomConfig.columnDays);
    }
    return d;
  });

  // Convert a date to a column position (fractional column index)
  function dateToColPos(date) {
    if (zoom === 'quarter') {
      const refYear = referenceDate.getFullYear();
      const refMonth = referenceDate.getMonth();
      const year = date.getFullYear();
      const month = date.getMonth();
      const monthOffset = (year - refYear) * 12 + (month - refMonth);
      const monthStart = new Date(year, month, 1);
      const nextMonth = new Date(year, month + 1, 1);
      const daysInMonth = (nextMonth.getTime() - monthStart.getTime()) / (1000 * 60 * 60 * 24);
      const dayInMonth = (date.getTime() - monthStart.getTime()) / (1000 * 60 * 60 * 24);
      return monthOffset + dayInMonth / daysInMonth;
    }
    return (date.getTime() - referenceDate.getTime()) / (1000 * 60 * 60 * 24) / zoomConfig.columnDays;
  }

  // Today column index
  let todayColumnIndex = $derived.by(() => {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    return dateToColPos(today);
  });

  // Month boundary positions (for month zoom: marks the 1st of each month)
  let monthBoundaries = $derived.by(() => {
    if (zoom !== 'month' || columns.length === 0) return [];
    const boundaries = [];
    const refTime = referenceDate.getTime();
    const msPerDay = 1000 * 60 * 60 * 24;
    // Find all month-starts within the visible range
    const first = columns[0];
    const last = columns[columns.length - 1];
    const d = new Date(first.getFullYear(), first.getMonth(), 1);
    if (d < first) d.setMonth(d.getMonth() + 1);
    while (d <= last) {
      const colPos = (d.getTime() - refTime) / msPerDay / zoomConfig.columnDays;
      boundaries.push({ px: (colPos + gridOffset) * colWidth, label: d.toLocaleDateString(undefined, { month: 'short' }) });
      d.setMonth(d.getMonth() + 1);
    }
    return boundaries;
  });

  // Get header labels
  let headerLabels = $derived.by(() => {
    if (zoom === 'week') {
      return columns.map(d => ({
        label: d.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric' }),
        sublabel: '',
        date: d
      }));
    }
    if (zoom === 'month') {
      return columns.map(d => ({
        label: `${d.getDate()}`,
        sublabel: d.toLocaleDateString(undefined, { month: 'short' }),
        date: d
      }));
    }
    return columns.map(d => ({
      label: d.toLocaleDateString(undefined, { month: 'short' }),
      sublabel: String(d.getFullYear()),
      date: d
    }));
  });

  // Month groups for header row
  let monthGroups = $derived.by(() => {
    const groups = [];
    if (zoom === 'month' && columns.length > 0) {
      // Use actual calendar month boundaries for pixel-accurate positioning
      const refTime = referenceDate.getTime();
      const msPerDay = 1000 * 60 * 60 * 24;
      const totalPx = columns.length * colWidth;

      const lastCol = columns[columns.length - 1];
      const endVisible = new Date(lastCol);
      endVisible.setDate(endVisible.getDate() + zoomConfig.columnDays);

      let d = new Date(columns[0].getFullYear(), columns[0].getMonth(), 1);
      while (d <= endVisible) {
        const nextMonth = new Date(d.getFullYear(), d.getMonth() + 1, 1);
        const leftDays = (d.getTime() - refTime) / msPerDay;
        const rightDays = (nextMonth.getTime() - refTime) / msPerDay;
        const leftPx = Math.max(0, (leftDays / zoomConfig.columnDays + gridOffset) * colWidth);
        const rightPx = Math.min(totalPx, (rightDays / zoomConfig.columnDays + gridOffset) * colWidth);
        if (rightPx > leftPx) {
          groups.push({
            label: d.toLocaleDateString(undefined, { month: 'long', year: 'numeric' }),
            leftPx,
            widthPx: rightPx - leftPx,
          });
        }
        d = nextMonth;
      }
    } else {
      // Week/quarter: column-aligned grouping
      let currentMonth = '';
      for (let i = 0; i < columns.length; i++) {
        const d = columns[i];
        const key = zoom === 'quarter'
          ? `Q${Math.floor(d.getMonth() / 3) + 1} ${d.getFullYear()}`
          : d.toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
        if (key !== currentMonth) {
          groups.push({ label: key, leftPx: i * colWidth, widthPx: colWidth });
          currentMonth = key;
        } else {
          groups[groups.length - 1].widthPx += colWidth;
        }
      }
    }
    return groups;
  });

  // --- Tree helpers ---
  let allItemsSorted = $derived.by(() => {
    if (collectionStore.loading) return [];
    return [...collectionStore.items].sort((a, b) => (a.level || 0) - (b.level || 0) || a.id - b.id);
  });

  function getRootItems() {
    return allItemsSorted.filter(item => item.parent_id === null);
  }

  function getItemsByParent(parentId) {
    return allItemsSorted.filter(item => item.parent_id === parentId);
  }

  function hasChildren(itemId) {
    return allItemsSorted.some(item => item.parent_id === itemId);
  }

  function toggleExpanded(itemId) {
    if (expandedItems.has(itemId)) {
      expandedItems.delete(itemId);
    } else {
      expandedItems.add(itemId);
    }
    expandedItems = new Set(expandedItems);
  }

  function getIndentLevel(level) {
    return `${level * 20}px`;
  }

  function getItemTypeInfo(item) {
    if (!item.item_type_id || !itemTypes.length) {
      const fallback = [
        { icon: GitBranch, color: '#9333ea', label: 'Epic' },
        { icon: Circle, color: '#2563eb', label: 'Feature' },
        { icon: Circle, color: '#16a34a', label: 'Story' },
        { icon: Circle, color: '#ea580c', label: 'Task' },
        { icon: Circle, color: '#6b7280', label: 'Subtask' }
      ];
      return fallback[Math.min(item.level || 0, fallback.length - 1)];
    }
    const it = itemTypes.find(type => type.id === item.item_type_id);
    if (it) {
      return {
        icon: itemTypeIconMap[it.icon] || itemTypeIconMap.FileText || Circle,
        color: it.color,
        label: it.name
      };
    }
    return { icon: Circle, color: '#6b7280', label: 'Unknown' };
  }

  function renderTreeItems(parentId = null, level = 0, result = []) {
    const items = parentId === null ? getRootItems() : getItemsByParent(parentId);
    for (const item of items) {
      result.push({
        ...item,
        treeLevel: level,
        hasChildren: hasChildren(item.id),
      });
      if (expandedItems.has(item.id)) {
        renderTreeItems(item.id, level + 1, result);
      }
    }
    return result;
  }

  let treeData = $derived.by(() => {
    if (allItemsSorted.length === 0) return [];
    return renderTreeItems();
  });

  function itemHasDate(item) {
    const start = getDateValue(item, roadmapConfig.start_field_id);
    const end = roadmapConfig.end_field_id ? getDateValue(item, roadmapConfig.end_field_id) : null;
    return !!(start || end);
  }

  // Auto-expand root items with children on first load
  $effect(() => {
    if (!collectionStore.loading && collectionStore.items.length > 0) {
      untrack(() => {
        if (expandedItems.size === 0) {
          const roots = getRootItems();
          const newExpanded = new Set();
          for (const r of roots) {
            if (hasChildren(r.id)) newExpanded.add(r.id);
          }
          if (newExpanded.size > 0) expandedItems = newExpanded;
        }
      });
    }
  });

  // Items with computed bar positions
  let roadmapItems = $derived.by(() => {
    if (!roadmapConfig.start_field_id) return [];

    const sourceItems = collectionStore.loading ? [] : collectionStore.items;
    return sourceItems
      .map(item => {
        const start = getDateValue(item, roadmapConfig.start_field_id);
        const end = roadmapConfig.end_field_id ? getDateValue(item, roadmapConfig.end_field_id) : null;

        if (!start && !end) return null;

        const startDate = start ? new Date(start) : null;
        const endDate = end ? new Date(end) : null;

        if (startDate) startDate.setHours(0, 0, 0, 0);
        if (endDate) endDate.setHours(0, 0, 0, 0);

        let barStart, barEnd, isMilestone;

        if (startDate && endDate) {
          barStart = dateToColPos(startDate);
          const endPlusOne = new Date(endDate);
          endPlusOne.setDate(endPlusOne.getDate() + 1);
          barEnd = dateToColPos(endPlusOne);
          isMilestone = false;
        } else {
          const d = startDate || endDate;
          barStart = dateToColPos(d);
          const dPlusOne = new Date(d);
          dPlusOne.setDate(dPlusOne.getDate() + 1);
          barEnd = dateToColPos(dPlusOne);
          isMilestone = true;
        }

        const status = statuses.find(s => s.id === item.status_id);
        const color = status?.color || '#6b7280';

        return {
          ...item,
          barStart,
          barEnd,
          isMilestone,
          statusColor: color,
          startDate: startDate?.toISOString()?.split('T')[0],
          endDate: endDate?.toISOString()?.split('T')[0],
        };
      })
      .filter(Boolean)
      .sort((a, b) => a.barStart - b.barStart);
  });

  // O(1) lookup map for scheduled items
  let roadmapItemMap = $derived(new Map(roadmapItems.map(i => [i.id, i])));

  // Dependency arrows data
  let dependencyArrows = $derived.by(() => {
    if (!roadmapConfig.dependency_link_type_id) return [];
    const linkTypeId = Number(roadmapConfig.dependency_link_type_id);
    const arrows = [];
    const itemMap = new Map(roadmapItems.map(i => [i.id, i]));

    for (const [itemId, links] of Object.entries(itemLinks)) {
      for (const link of links) {
        if (link.link_type_id !== linkTypeId) continue;
        const source = itemMap.get(link.source_id);
        const target = itemMap.get(link.target_id);
        if (source && target) {
          arrows.push({ source, target });
        }
      }
    }
    return arrows;
  });

  // Helper: extract date value from item
  function getDateValue(item, fieldId) {
    if (fieldId?.startsWith('cf_')) {
      const cfId = fieldId.replace('cf_', '');
      return item.custom_field_values?.[cfId] ?? null;
    }
    return item[fieldId] ?? null;
  }

  // Column width in pixels
  let colWidth = $derived.by(() => {
    if (zoom === 'week') return 40;
    if (zoom === 'month') return 60;
    return 80;
  });

  let totalWidth = $derived(colWidth * columns.length);

  // Row height
  const ROW_HEIGHT = 40;

  // Sync collection name and load links when items change
  $effect(() => {
    if (!collectionStore.loading) {
      currentCollectionName = collectionStore.collectionName;
      untrack(() => loadLinksForItems(collectionStore.items));
    }
  });

  // Load links for visible items
  async function loadLinksForItems(items) {
    if (!roadmapConfig.dependency_link_type_id) return;
    const newLinks = {};
    const promises = items.map(async (item) => {
      try {
        const result = await api.links.getForItem('items', item.id);
        newLinks[item.id] = result || [];
      } catch {
        newLinks[item.id] = [];
      }
    });
    await Promise.all(promises);
    itemLinks = newLinks;
  }

  // Load config data
  async function loadConfig() {
    try {
      const config = await api.collections.getBoardConfiguration(collectionId, workspaceId);
      boardConfig = config;
      boardConfigId = config.id;
      if (config.roadmap_config) {
        roadmapConfig = {
          start_field_id: config.roadmap_config.start_field_id || 'due_date',
          end_field_id: config.roadmap_config.end_field_id || '',
          dependency_link_type_id: config.roadmap_config.dependency_link_type_id || null,
        };
      }
    } catch {
      // No config yet
    }
  }

  async function loadReferenceData() {
    try {
      const [lt, cf] = await Promise.all([
        api.linkTypes.getAll(),
        api.customFields.getAll(),
      ]);
      linkTypes = lt || [];
      customFields = cf?.data || cf || [];

      // Load screen fields to determine available date fields
      let screenId = null;
      if (workspace?.configuration_set_id) {
        const configSet = await api.configurationSets.get(workspace.configuration_set_id);
        screenId = configSet?.edit_screen_id || configSet?.create_screen_id || configSet?.view_screen_id;
      }
      if (!screenId) screenId = 1;
      const screen = await api.screens.get(screenId);
      screenFields = screen?.fields || [];
    } catch (e) {
      console.error('Failed to load reference data:', e);
    }
  }

  // Save roadmap config
  async function saveConfig() {
    const payload = {
      roadmap_config: {
        start_field_id: roadmapConfig.start_field_id,
        end_field_id: roadmapConfig.end_field_id,
        dependency_link_type_id: roadmapConfig.dependency_link_type_id ? Number(roadmapConfig.dependency_link_type_id) : null,
      },
    };

    try {
      if (boardConfigId) {
        await api.collections.updateBoardConfiguration(collectionId, boardConfigId, payload);
      } else {
        const result = await api.collections.createBoardConfiguration(collectionId, workspaceId, payload);
        boardConfigId = result.id;
      }
    } catch (e) {
      console.error('Failed to save roadmap config:', e);
    }
  }

  function onStartFieldChange(val) {
    roadmapConfig.start_field_id = val;
    saveConfig();
  }

  function onEndFieldChange(val) {
    roadmapConfig.end_field_id = val;
    saveConfig();
  }

  function onLinkTypeChange(val) {
    roadmapConfig.dependency_link_type_id = val || null;
    saveConfig();
    if (val) loadLinksForItems(collectionStore.items);
  }

  // Navigation
  function scrollLeft() {
    scrollOffset -= Math.floor(zoomConfig.visibleColumns / 2);
  }

  function scrollRight() {
    scrollOffset += Math.floor(zoomConfig.visibleColumns / 2);
  }

  function goToToday() {
    scrollOffset = 0;
  }

  // Item click
  function openItem(itemId) {
    selectedItemId = itemId;
    showItemModal = true;
  }

  // --- Bar drag handlers (existing move/resize) ---
  function onBarPointerDown(e, item, mode) {
    if (e.button !== 0) return;
    e.preventDefault();
    e.stopPropagation();

    dragInfo = {
      itemId: item.id,
      mode,
      startX: e.clientX,
      origStart: item.barStart,
      origEnd: item.barEnd,
      origStartDate: item.startDate,
      origEndDate: item.endDate,
    };

    window.addEventListener('pointermove', onDragMove);
    window.addEventListener('pointerup', onDragEnd);
  }

  function onDragMove(e) {
    if (!dragInfo) return;
    const dx = e.clientX - dragInfo.startX;
    const pxPerDay = zoom === 'quarter'
      ? colWidth / 30
      : colWidth / zoomConfig.columnDays;
    const rawDays = dx / pxPerDay;
    dragInfo.daysDelta = Math.round(rawDays / snapDays) * snapDays;
  }

  async function onDragEnd() {
    window.removeEventListener('pointermove', onDragMove);
    window.removeEventListener('pointerup', onDragEnd);

    if (!dragInfo || dragInfo.daysDelta === undefined || dragInfo.daysDelta === 0) {
      dragInfo = null;
      return;
    }

    const { itemId, mode, daysDelta, origStartDate, origEndDate } = dragInfo;
    dragInfo = null;

    const actualDays = daysDelta;
    const updateData = {};

    if (mode === 'move' || mode === 'resize-left') {
      if (origStartDate) {
        const newStart = new Date(origStartDate);
        newStart.setDate(newStart.getDate() + actualDays);
        setDateUpdate(updateData, roadmapConfig.start_field_id, newStart);
      } else if (mode === 'resize-left' && origEndDate && roadmapConfig.start_field_id) {
        // Milestone with only end date → expand left: set start = end - |days|
        const newStart = new Date(origEndDate);
        newStart.setDate(newStart.getDate() + actualDays);
        setDateUpdate(updateData, roadmapConfig.start_field_id, newStart);
      }
    }
    if (mode === 'move' || mode === 'resize-right') {
      if (origEndDate) {
        const newEnd = new Date(origEndDate);
        newEnd.setDate(newEnd.getDate() + actualDays);
        setDateUpdate(updateData, roadmapConfig.end_field_id, newEnd);
      } else if (mode === 'resize-right' && origStartDate && roadmapConfig.end_field_id) {
        // Milestone with only start date → expand right: set end = start + days
        const newEnd = new Date(origStartDate);
        newEnd.setDate(newEnd.getDate() + actualDays);
        setDateUpdate(updateData, roadmapConfig.end_field_id, newEnd);
      }
    }

    if (Object.keys(updateData).length > 0) {
      try {
        await api.items.update(itemId, updateData);
        reloadCollection();
      } catch (e) {
        console.error('Failed to update item dates:', e);
        reloadCollection();
      }
    }
  }

  function setDateUpdate(data, fieldId, date) {
    const dateStr = date.toISOString().split('T')[0];
    if (fieldId?.startsWith('cf_')) {
      const cfId = fieldId.replace('cf_', '');
      if (!data.custom_field_values) data.custom_field_values = {};
      data.custom_field_values[cfId] = dateStr;
    } else {
      data[fieldId] = dateStr;
    }
  }

  function getDragOffset(item) {
    if (!dragInfo || dragInfo.itemId !== item.id) return { startOffset: 0, endOffset: 0 };
    const divisor = zoom === 'quarter' ? 30 : zoomConfig.columnDays;
    const delta = (dragInfo.daysDelta || 0) / divisor;
    if (dragInfo.mode === 'move') return { startOffset: delta, endOffset: delta };
    if (dragInfo.mode === 'resize-left') return { startOffset: delta, endOffset: 0 };
    if (dragInfo.mode === 'resize-right') return { startOffset: 0, endOffset: delta };
    return { startOffset: 0, endOffset: 0 };
  }

  // SVG arrow path between two items (using treeData indices)
  function getArrowPath(source, target, sourceIndex, targetIndex) {
    const x1 = (source.barEnd + gridOffset) * colWidth;
    const y1 = sourceIndex * ROW_HEIGHT + ROW_HEIGHT / 2;
    const x2 = (target.barStart + gridOffset) * colWidth;
    const y2 = targetIndex * ROW_HEIGHT + ROW_HEIGHT / 2;

    const midX = (x1 + x2) / 2;
    return `M ${x1} ${y1} C ${midX} ${y1}, ${midX} ${y2}, ${x2} ${y2}`;
  }

  // --- Panel resize handlers ---
  function onPanelResizeStart(e) {
    if (e.button !== 0) return;
    e.preventDefault();
    isResizingPanel = true;
    resizeStartX = e.clientX;
    resizeStartWidth = treePanelWidth;
    window.addEventListener('pointermove', onPanelResizeMove);
    window.addEventListener('pointerup', onPanelResizeEnd);
  }

  function onPanelResizeMove(e) {
    if (!isResizingPanel) return;
    const dx = e.clientX - resizeStartX;
    treePanelWidth = Math.max(200, Math.min(500, resizeStartWidth + dx));
  }

  function onPanelResizeEnd() {
    isResizingPanel = false;
    window.removeEventListener('pointermove', onPanelResizeMove);
    window.removeEventListener('pointerup', onPanelResizeEnd);
  }

  // --- Drag-to-schedule handlers ---
  function onTreeItemDragStart(e, item) {
    if (e.button !== 0) return;
    if (itemHasDate(item)) return; // Only drag unscheduled items
    e.preventDefault();

    scheduleDragInfo = {
      itemId: item.id,
      itemTitle: item.title,
      startX: e.clientX,
      startY: e.clientY,
      currentX: e.clientX,
      currentY: e.clientY,
      active: false,
    };

    window.addEventListener('pointermove', onScheduleDragMove);
    window.addEventListener('pointerup', onScheduleDragEnd);
  }

  function onScheduleDragMove(e) {
    if (!scheduleDragInfo) return;
    const dx = e.clientX - scheduleDragInfo.startX;
    const dy = e.clientY - scheduleDragInfo.startY;

    // Activate after 5px threshold
    if (!scheduleDragInfo.active && Math.sqrt(dx * dx + dy * dy) > 5) {
      scheduleDragInfo.active = true;
    }

    if (scheduleDragInfo.active) {
      scheduleDragInfo = { ...scheduleDragInfo, currentX: e.clientX, currentY: e.clientY };
    }
  }

  async function onScheduleDragEnd(e) {
    window.removeEventListener('pointermove', onScheduleDragMove);
    window.removeEventListener('pointerup', onScheduleDragEnd);

    if (!scheduleDragInfo || !scheduleDragInfo.active) {
      scheduleDragInfo = null;
      return;
    }

    const { itemId } = scheduleDragInfo;
    scheduleDragInfo = null;

    // Determine drop position relative to the timeline
    if (!timelineScrollContainer) return;
    const timelineRect = timelineScrollContainer.getBoundingClientRect();
    const dropX = e.clientX - timelineRect.left + timelineScrollContainer.scrollLeft;

    if (dropX < 0) return; // Dropped outside timeline

    const dayOffset = dropX / colWidth - gridOffset;
    const dropDate = new Date(referenceDate);
    if (zoom === 'quarter') {
      const monthOffset = Math.floor(dayOffset);
      const frac = dayOffset - monthOffset;
      dropDate.setMonth(dropDate.getMonth() + monthOffset);
      const nextM = new Date(dropDate.getFullYear(), dropDate.getMonth() + 1, 1);
      const dim = (nextM.getTime() - new Date(dropDate.getFullYear(), dropDate.getMonth(), 1).getTime()) / (1000 * 60 * 60 * 24);
      dropDate.setDate(1 + Math.round(frac * dim / snapDays) * snapDays);
    } else {
      dropDate.setDate(dropDate.getDate() + Math.round(dayOffset * zoomConfig.columnDays));
    }

    const updateData = {};
    setDateUpdate(updateData, roadmapConfig.start_field_id, dropDate);

    // If end field is configured, default span = 7 days
    if (roadmapConfig.end_field_id) {
      const endDate = new Date(dropDate);
      endDate.setDate(endDate.getDate() + 7);
      setDateUpdate(updateData, roadmapConfig.end_field_id, endDate);
    }

    if (Object.keys(updateData).length > 0) {
      try {
        await api.items.update(itemId, updateData);
        reloadCollection();
      } catch (err) {
        console.error('Failed to schedule item:', err);
      }
    }
  }

  // --- Scroll sync ---
  function syncTreeScroll(e) {
    if (timelineScrollContainer && timelineScrollContainer.scrollTop !== e.target.scrollTop) {
      timelineScrollContainer.scrollTop = e.target.scrollTop;
    }
  }

  function syncTimelineScroll(e) {
    if (treeScrollContainer && treeScrollContainer.scrollTop !== e.target.scrollTop) {
      treeScrollContainer.scrollTop = e.target.scrollTop;
    }
  }

  onMount(async () => {
    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
      await workspaceDataStore.initialize(workspaceId);
    }
    await collectionStore.setItemsPage(1, 500);
    await Promise.all([loadConfig(), loadReferenceData()]);
    loading = false;
  });
</script>

{#if loading}
  <div class="flex items-center justify-center min-h-screen" style={styles.backgroundStyle}>
    <div class="animate-pulse" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
      {t('collections.roadmapSettings')}...
    </div>
  </div>
{:else}
  <div class="h-screen flex flex-col overflow-hidden" style="{styles.backgroundStyle} {styles.contextVars} background-attachment: scroll; overscroll-behavior: none;" class:roadmap-dragging={isResizingPanel || scheduleDragInfo?.active}>
    <div class="p-6 flex-1 flex flex-col min-h-0">
      <!-- Header -->
      <div class="mb-6">
        <ViewHeader
          workspaceName={workspace?.name || ''}
          collection={currentCollectionName}
          viewName={t('collections.roadmap')}
          itemCount={treeData.length}
        >
          {#snippet actions()}
            <div class="flex rounded" style="background-color: var(--ctx-surface, var(--ds-background-neutral)); backdrop-filter: var(--ctx-backdrop, none);">
              <Button
                size="sm"
                onclick={() => showSettings = !showSettings}
              >
                <div class="flex items-center gap-2">
                  <Settings class="w-4 h-4" />
                  {t('collections.roadmapSettings')}
                </div>
              </Button>
            </div>
          {/snippet}
        </ViewHeader>
      </div>

      <!-- Toolbar: Zoom + Navigation + Settings toggle -->
      <div class="flex items-center justify-between mb-4 gap-3">
        <div class="flex items-center gap-2">
          <!-- Zoom controls -->
          <div class="flex rounded overflow-hidden" style="border: 1px solid var(--ctx-border, var(--ds-border)); background-color: var(--ctx-surface, var(--ds-surface));">
            {#each [['week', t('collections.roadmapZoomWeek')], ['month', t('collections.roadmapZoomMonth')], ['quarter', t('collections.roadmapZoomQuarter')]] as [z, label]}
              <button
                class="px-3 py-1.5 text-xs font-medium transition-colors"
                style={zoom === z ? 'background-color: var(--ds-accent-blue-subtle); color: var(--ds-text-info);' : `color: var(--ds-text-subtle);`}
                onclick={() => zoom = z}
              >
                {label}
              </button>
            {/each}
          </div>

          <!-- Navigation -->
          <button
            class="p-1.5 rounded transition-colors"
            style="color: var(--ctx-text-subtle, var(--ds-text-subtle));"
            onclick={scrollLeft}
          >
            <ChevronLeft class="w-4 h-4" />
          </button>
          <button
            class="px-2 py-1 text-xs font-medium rounded transition-colors"
            style="color: var(--ctx-text-subtle, var(--ds-text-subtle)); border: 1px solid var(--ctx-border, var(--ds-border));"
            onclick={goToToday}
          >
            {t('collections.roadmapToday')}
          </button>
          <button
            class="p-1.5 rounded transition-colors"
            style="color: var(--ctx-text-subtle, var(--ds-text-subtle));"
            onclick={scrollRight}
          >
            <ChevronRight class="w-4 h-4" />
          </button>
        </div>

      </div>

      <!-- Settings panel -->
      {#if showSettings}
        <div
          class="mb-4 p-4 rounded-lg"
          style="background-color: var(--ctx-surface, var(--ds-surface)); border: 1px solid var(--ctx-border, var(--ds-border)); backdrop-filter: var(--ctx-backdrop, none);"
        >
          <div class="grid grid-cols-3 gap-4">
            <div>
              <label class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('collections.roadmapStartField')}</label>
              <Select
                value={roadmapConfig.start_field_id}
                options={dateFieldOptions}
                size="small"
                onchange={onStartFieldChange}
              />
            </div>
            <div>
              <label class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('collections.roadmapEndField')}</label>
              <Select
                value={roadmapConfig.end_field_id}
                options={[{ value: '', label: t('collections.roadmapNone') }, ...dateFieldOptions]}
                size="small"
                onchange={onEndFieldChange}
              />
            </div>
            <div>
              <label class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('collections.roadmapDependencyLinkType')}</label>
              <Select
                value={roadmapConfig.dependency_link_type_id ? String(roadmapConfig.dependency_link_type_id) : ''}
                options={linkTypeOptions}
                size="small"
                onchange={onLinkTypeChange}
              />
            </div>
          </div>
        </div>
      {/if}

      <!-- No config state -->
      {#if !roadmapConfig.start_field_id}
        <div class="flex flex-col items-center justify-center py-20" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
          <p class="text-sm">{t('collections.roadmapNoConfig')}</p>
          <button
            class="mt-3 px-4 py-2 text-sm font-medium rounded transition-colors"
            style="background-color: var(--ds-accent-blue-subtle); color: var(--ds-text-info);"
            onclick={() => showSettings = true}
          >
            {t('collections.roadmapSettings')}
          </button>
        </div>
      {:else}
        <!-- Split layout: Tree Panel + Resize Handle + Timeline -->
        <div
          class="rounded-lg overflow-hidden flex flex-1 min-h-0"
          style="border: 1px solid var(--ctx-border, var(--ds-border)); background-color: var(--ctx-surface, var(--ds-surface)); backdrop-filter: var(--ctx-backdrop, none);"
        >
          <!-- Left: Tree Panel -->
          <div
            class="shrink-0 flex flex-col"
            style="width: {treePanelWidth}px; border-right: 1px solid var(--ds-border);"
          >
            <!-- Tree header row 1 (month groups height) -->
            <div
              class="flex items-center px-3 shrink-0"
              style="height: 29px; border-bottom: 1px solid var(--ds-border);"
            >
              <span class="text-xs font-medium" style="color: var(--ds-text-subtle);">
                Items ({treeData.length})
              </span>
            </div>
            <!-- Tree header row 2 (column labels height) -->
            <div
              class="shrink-0"
              style="height: 29px; border-bottom: 1px solid var(--ds-border);"
            ></div>
            <!-- Tree body (scrollable, synced) -->
            <div
              class="flex-1 overflow-y-auto overflow-x-hidden tree-scroll"
              bind:this={treeScrollContainer}
              onscroll={syncTreeScroll}
            >
              {#each treeData as item (item.id)}
                {@const typeInfo = getItemTypeInfo(item)}
                {@const scheduled = itemHasDate(item)}
                {@const TypeIcon = typeInfo.icon}
                <button
                  class="flex items-center gap-1.5 w-full text-left group/tree-row"
                  style="height: {ROW_HEIGHT}px; padding-left: calc(8px + {getIndentLevel(item.treeLevel)}); padding-right: 8px; border-bottom: 1px solid var(--ds-border-subtle, var(--ds-border));"
                  onclick={() => openItem(item.id)}
                  onpointerdown={(e) => onTreeItemDragStart(e, item)}
                >
                  <!-- Expand/collapse chevron -->
                  {#if item.hasChildren}
                    <span
                      class="shrink-0 flex items-center justify-center w-4 h-4 rounded transition-colors"
                      style="color: var(--ds-text-subtle);"
                      role="button"
                      tabindex="-1"
                      onclick={(e) => { e.stopPropagation(); toggleExpanded(item.id); }}
                    >
                      {#if expandedItems.has(item.id)}
                        <ChevronDown class="w-3 h-3" />
                      {:else}
                        <ChevronRight class="w-3 h-3" />
                      {/if}
                    </span>
                  {:else}
                    <span class="w-4 shrink-0"></span>
                  {/if}

                  <!-- Type icon -->
                  <span class="shrink-0 flex items-center justify-center w-4 h-4" style="color: {typeInfo.color};">
                    <TypeIcon class="w-3.5 h-3.5" />
                  </span>

                  <!-- Item key -->
                  {#if item.item_key}
                    <span class="text-xs shrink-0" style="color: var(--ds-text-subtle);">{item.item_key}</span>
                  {/if}

                  <!-- Title -->
                  <span class="text-sm truncate flex-1 group-hover/tree-row:underline" style="color: var(--ds-text);">{item.title}</span>

                  <!-- Scheduled indicator -->
                  {#if scheduled}
                    <span class="shrink-0 w-2 h-2 rounded-full" style="background-color: var(--ds-accent-blue);"></span>
                  {/if}
                </button>
              {/each}
            </div>
          </div>

          <!-- Resize handle -->
          <div
            class="shrink-0 resize-handle"
            style="width: 4px; cursor: col-resize; background-color: transparent;"
            onpointerdown={onPanelResizeStart}
            role="separator"
            tabindex="-1"
          ></div>

          <!-- Right: Timeline -->
          <div class="flex-1 flex flex-col min-w-0">
            <div
              class="flex-1 overflow-x-auto overflow-y-auto timeline-scroll"
              style="touch-action: pan-x pan-y;"
              bind:this={timelineScrollContainer}
              onscroll={syncTimelineScroll}
            >
              <div style="min-width: {totalWidth}px;">
                <!-- Header row: month/quarter groups -->
                <div class="relative sticky top-0 z-30" style="height: 29px; border-bottom: 1px solid var(--ctx-border, var(--ds-border)); background-color: var(--ctx-surface, var(--ds-surface));">
                  {#each monthGroups as group}
                    <div
                      class="absolute text-xs font-medium text-center py-1.5 px-1 truncate"
                      style="left: {group.leftPx}px; width: {group.widthPx}px; color: var(--ds-text-subtle); border-left: 1px solid var(--ds-border);"
                    >
                      {group.label}
                    </div>
                  {/each}
                </div>

                <!-- Header row: column labels -->
                <div class="flex sticky top-[29px] z-30" style="border-bottom: 1px solid var(--ctx-border, var(--ds-border)); background-color: var(--ctx-surface, var(--ds-surface));">
                  {#each headerLabels as col, i}
                    <div
                      class="text-center py-1.5 text-xs shrink-0"
                      style="width: {colWidth}px; color: var(--ds-text-subtlest); border-left: 1px solid var(--ds-border-subtle, var(--ds-border));"
                    >
                      {col.label}
                    </div>
                  {/each}
                </div>

                <!-- Body: rows aligned 1:1 with treeData -->
                <div class="relative">
                  {#each treeData as item, rowIndex (item.id)}
                    {@const roadmapItem = roadmapItemMap.get(item.id)}
                    <div
                      class="relative"
                      style="height: {ROW_HEIGHT}px; border-bottom: 1px solid var(--ds-border-subtle, var(--ds-border));"
                    >
                      <!-- Grid lines -->
                      {#each columns as _, ci}
                        <div
                          class="absolute top-0 bottom-0"
                          style="left: {ci * colWidth}px; width: 1px; background-color: var(--ds-border-subtle, var(--ds-border)); opacity: 0.3;"
                        ></div>
                      {/each}

                      <!-- Month boundary lines (month zoom only) -->
                      {#each monthBoundaries as boundary}
                        <div
                          class="absolute top-0 bottom-0"
                          style="left: {boundary.px}px; width: 1px; border-left: 1px dashed var(--ds-text-subtlest, var(--ds-text-subtle)); opacity: 0.5; z-index: 4;"
                        ></div>
                      {/each}

                      <!-- Today line -->
                      {#if (todayColumnIndex + gridOffset) >= 0 && (todayColumnIndex + gridOffset) < columns.length}
                        <div
                          class="absolute top-0 bottom-0"
                          style="left: {(todayColumnIndex + gridOffset) * colWidth}px; width: 2px; background-color: var(--ds-accent-red); opacity: 0.6; z-index: 5;"
                        ></div>
                      {/if}

                      <!-- Bar / milestone (only if item is scheduled) -->
                      {#if roadmapItem}
                        {@const offset = getDragOffset(roadmapItem)}
                        {@const adjStart = roadmapItem.barStart + offset.startOffset}
                        {@const adjEnd = roadmapItem.barEnd + offset.endOffset}
                        {@const barLeftPx = (adjStart + gridOffset) * colWidth}
                        {@const barWidthPx = Math.max((adjEnd - adjStart) * colWidth, 8)}
                        {@const visibleColor = getVisibleColor(roadmapItem.statusColor)}

                        {#if roadmapItem.isMilestone && !roadmapConfig.end_field_id}
                          <!-- Diamond milestone (no end field configured, can't expand) -->
                          <div
                            class="absolute flex items-center justify-center"
                            style="left: {barLeftPx + barWidthPx / 2 - 8}px; top: {(ROW_HEIGHT - 16) / 2}px; width: 16px; height: 16px; transform: rotate(45deg); background-color: {visibleColor}; border-radius: 2px; z-index: 10; cursor: grab;"
                            role="button"
                            tabindex="0"
                            onpointerdown={(e) => onBarPointerDown(e, roadmapItem, 'move')}
                          ></div>
                        {:else}
                          <!-- Range bar OR expandable milestone (thin bar with resize handles) -->
                          <div
                            class="absolute flex items-center rounded group/bar"
                            style="left: {barLeftPx}px; width: {barWidthPx}px; top: {(ROW_HEIGHT - 24) / 2}px; height: 24px; background-color: {visibleColor}; opacity: 0.85; z-index: 10; cursor: grab;"
                            role="button"
                            tabindex="0"
                            onpointerdown={(e) => onBarPointerDown(e, roadmapItem, 'move')}
                          >
                            <div
                              class="absolute left-0 top-0 bottom-0 w-2 cursor-col-resize opacity-0 group-hover/bar:opacity-100 rounded-l"
                              style="background-color: rgba(0,0,0,0.2);"
                              role="separator"
                              tabindex="-1"
                              onpointerdown={(e) => { e.stopPropagation(); onBarPointerDown(e, roadmapItem, 'resize-left'); }}
                            ></div>

                            <span class="text-xs font-medium px-2 truncate" style="color: white; text-shadow: 0 1px 2px rgba(0,0,0,0.3);">
                              {#if barWidthPx > 60}
                                {roadmapItem.title}
                              {/if}
                            </span>

                            <div
                              class="absolute right-0 top-0 bottom-0 w-2 cursor-col-resize opacity-0 group-hover/bar:opacity-100 rounded-r"
                              style="background-color: rgba(0,0,0,0.2);"
                              role="separator"
                              tabindex="-1"
                              onpointerdown={(e) => { e.stopPropagation(); onBarPointerDown(e, roadmapItem, 'resize-right'); }}
                            ></div>
                          </div>
                        {/if}
                      {/if}
                    </div>
                  {/each}

                  <!-- SVG overlay for dependency arrows -->
                  {#if dependencyArrows.length > 0}
                    <svg
                      class="absolute pointer-events-none"
                      style="left: 0; top: 0; width: {totalWidth}px; height: {treeData.length * ROW_HEIGHT}px; z-index: 20;"
                    >
                      {#each dependencyArrows as arrow}
                        {@const sourceIdx = treeData.findIndex(i => i.id === arrow.source.id)}
                        {@const targetIdx = treeData.findIndex(i => i.id === arrow.target.id)}
                        {#if sourceIdx !== -1 && targetIdx !== -1}
                          <path
                            d={getArrowPath(arrow.source, arrow.target, sourceIdx, targetIdx)}
                            fill="none"
                            stroke="var(--ds-text-subtle)"
                            stroke-width="1.5"
                            stroke-dasharray="4,3"
                            opacity="0.6"
                          />
                          {@const tx = (arrow.target.barStart + gridOffset) * colWidth}
                          {@const ty = targetIdx * ROW_HEIGHT + ROW_HEIGHT / 2}
                          <polygon
                            points="{tx},{ty} {tx - 6},{ty - 4} {tx - 6},{ty + 4}"
                            fill="var(--ds-text-subtle)"
                            opacity="0.6"
                          />
                        {/if}
                      {/each}
                    </svg>
                  {/if}
                </div>
              </div>
            </div>
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}

<!-- Drag-to-schedule ghost -->
{#if scheduleDragInfo?.active}
  <div
    class="fixed pointer-events-none z-50 px-3 py-1.5 rounded shadow-lg text-xs font-medium truncate max-w-[200px]"
    style="left: {scheduleDragInfo.currentX + 12}px; top: {scheduleDragInfo.currentY - 8}px; background-color: var(--ds-accent-blue); color: white;"
  >
    {scheduleDragInfo.itemTitle}
  </div>
{/if}

<!-- Item Detail Modal -->
{#if showItemModal && selectedItemId}
  <ItemDetail
    itemId={selectedItemId}
    isModal={true}
    onclose={() => {
      const id = selectedItemId;
      showItemModal = false;
      selectedItemId = null;
      refreshCollectionItem(id);
    }}
  />
{/if}

<style>
  /* Custom scrollbar for timeline */
  .timeline-scroll::-webkit-scrollbar,
  .tree-scroll::-webkit-scrollbar {
    width: 6px;
    height: 8px;
  }
  .timeline-scroll::-webkit-scrollbar-track,
  .tree-scroll::-webkit-scrollbar-track {
    background: transparent;
  }
  .timeline-scroll::-webkit-scrollbar-thumb,
  .tree-scroll::-webkit-scrollbar-thumb {
    background-color: var(--ds-border);
    border-radius: 4px;
  }

  /* Prevent scroll chaining / overscroll navigation */
  .timeline-scroll {
    overscroll-behavior: contain;
  }
  .tree-scroll {
    overscroll-behavior-y: contain;
  }

  /* Resize handle highlight */
  .resize-handle:hover,
  .resize-handle:active {
    background-color: var(--ds-accent-blue) !important;
    opacity: 0.5;
  }

  /* Disable text selection during drag/resize */
  .roadmap-dragging {
    user-select: none;
  }
</style>
