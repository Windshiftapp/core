<script>
  import { onMount } from 'svelte';
  import { untrack } from 'svelte';
  import { fly } from 'svelte/transition';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import { resolveScreenId } from '../../utils/screenResolution.js';
  import { navigate } from '../../router.js';
  import { collectionStore, reloadCollection, refreshCollectionItem } from '../../stores/collectionContext.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import { workspaceDataStore } from '../../stores/index.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import StaticViewBackground from '../../layout/StaticViewBackground.svelte';
  import SubFilterBar from './SubFilterBar.svelte';
  import Select from '../../components/Select.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import ItemDetail from '../items/ItemDetail.svelte';
  import RoadmapItemPreview from './RoadmapItemPreview.svelte';
  import { buildHierarchyDatePatches, projectHierarchyDates } from './roadmapHierarchyDates.js';
  import { Settings, ChevronLeft, ChevronRight, Diamond, ChevronDown, CalendarClock, RotateCcw } from '@lucide/svelte';
  import { getVisibleColor } from '../../utils/colorUtils.js';
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import { SYSTEM_FIELDS } from '../../stores/fieldConfig.js';
  import Button from '../../components/Button.svelte';
  import LazyRender from '../../components/LazyRender.svelte';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { useEventListener, useResizeObserver } from 'runed';

  // Props
  let { workspaceId, collectionId = null } = $props();

  // Reference data from shared workspace store
  let workspace = $derived(workspaceDataStore.workspace);
  let statuses = $derived(workspaceDataStore.statuses);
  let itemTypes = $derived(workspaceDataStore.itemTypes || []);

  // Gradient
  const styles = useGradientStyles();

  // The workspace-default roadmap config (collectionId == null) is gated to
  // workspace admins on the backend — see board_configuration.go
  // checkWorkspaceWriteAccess. Collection-specific roadmap config remains
  // owner-gated server-side, so the frontend trusts that path.
  let canConfigure = $derived(
    collectionId != null || workspacePermissions.canAdminWorkspace(workspaceId)
  );

  // State
  let loading = $state(true);
  let currentCollectionName = $state('Default');

  // Settings panel toggle
  let settingsOpen = $state(false);
  let settingsPanel = $state(null);
  let settingsButton = $state(null);

  function onSettingsClickOutside(e) {
    const portalOwner = e.target?.closest?.('[data-popover-owner]')?.dataset?.popoverOwner;
    if (portalOwner === 'roadmap-settings') return;
    if (settingsPanel && !settingsPanel.contains(e.target) && settingsButton && !settingsButton.contains(e.target)) {
      settingsOpen = false;
    }
  }
  useEventListener(
    () => (settingsOpen ? document : null),
    'pointerdown',
    onSettingsClickOutside
  );

  let boardConfig = $state(null);
  let boardConfigId = $state(null);
  let roadmapConfig = $state({ start_field_id: 'due_date', end_field_id: '', dependency_link_type_id: null });
  let roadmapCardFields = $derived(boardConfig?.card_fields || []);
  let linkTypes = $state([]);
  let customFields = $state([]);
  let screenFields = $state([]);

  // Hierarchy scheduling is a local roadmap view mode. It only uses the
  // canonical scheduling fields and never writes merely because it is enabled.
  let hierarchyMode = $state('off');
  let adjustHierarchyDates = $state(false);
  let hierarchyDateItems = $state([]);
  let hierarchyDatesLoading = $state(false);
  let hierarchyDatesLoaded = $state(false);
  let hierarchyDatesError = $state('');
  let hierarchyLoadGeneration = 0;

  let hierarchyFieldsReady = $derived(
    roadmapConfig.start_field_id === 'start_date' && roadmapConfig.end_field_id === 'end_date'
  );
  let hierarchyProjectionActive = $derived(
    hierarchyMode !== 'off' && hierarchyFieldsReady && hierarchyDatesLoaded && !hierarchyDatesError
  );
  let hierarchyProjection = $derived.by(() =>
    hierarchyProjectionActive
      ? projectHierarchyDates(hierarchyDateItems, hierarchyMode)
      : new Map()
  );

  function setHierarchyMode(mode) {
    if (mode !== 'off' && !hierarchyFieldsReady) return;
    hierarchyMode = mode;
  }

  async function loadHierarchyDates(rootIds) {
    const generation = ++hierarchyLoadGeneration;
    hierarchyDatesLoading = true;
    hierarchyDatesLoaded = false;
    hierarchyDatesError = '';
    try {
      const result = await api.items.getRoadmapHierarchyDates(rootIds);
      if (generation !== hierarchyLoadGeneration) return;
      if (result?.truncated) {
        hierarchyDateItems = [];
        adjustHierarchyDates = false;
        hierarchyDatesError = t('collections.roadmapHierarchyTruncated');
        return;
      }
      hierarchyDateItems = result?.items || [];
      hierarchyDatesLoaded = true;
    } catch (err) {
      if (generation !== hierarchyLoadGeneration) return;
      hierarchyDateItems = [];
      adjustHierarchyDates = false;
      hierarchyDatesError = t('collections.roadmapHierarchyLoadError');
      console.error('Failed to load roadmap hierarchy dates:', err);
    } finally {
      if (generation === hierarchyLoadGeneration) hierarchyDatesLoading = false;
    }
  }

  $effect(() => {
    const enabled = hierarchyMode !== 'off' && hierarchyFieldsReady && !collectionStore.loading;
    const roots = enabled
      ? collectionStore.items
          .filter((item) => item?.id != null)
          .map((item) => item.id)
      : [];
    // Include visible dates so an external item refresh reloads the full projection.
    const signature = enabled
      ? collectionStore.items
          .map((item) => `${item.id}:${item.parent_id ?? ''}:${item.start_date ?? ''}:${item.end_date ?? ''}`)
          .join('|')
      : '';
    if (!enabled) {
      hierarchyLoadGeneration++;
      hierarchyDateItems = [];
      hierarchyDatesLoaded = false;
      hierarchyDatesLoading = false;
      hierarchyDatesError = '';
      return;
    }
    void signature;
    untrack(() => loadHierarchyDates(roots));
  });

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
  const ROADMAP_LINK_CHUNK = 200; // ids per batched /links/batch request (server cap 500)

  // Timeline computation
  let containerWidth = $state(1200);
  let timelineScrollLeft = $state(0);

  const QUARTER_PLANNING_MONTHS = 60;
  const ZOOM_COLUMN_WIDTHS = { week: 40, month: 60, quarter: 80 };

  // --- Tree panel state ---
  let treePanelWidth = $state(280);
  let isResizingPanel = $state(false);
  let resizeStartX = $state(0);
  let resizeStartWidth = $state(0);
  let expandedItems = $state(new Set());
  let schedulePreview = $state(null);
  let schedulingItemIds = $state(new Set());
  let treeScrollContainer = $state(null);
  let timelineScrollContainer = $state(null);

  // Drag listeners (attached only while their respective drag state is active)
  useEventListener(() => (dragInfo ? window : null), 'pointermove', onDragMove);
  useEventListener(() => (dragInfo ? window : null), 'pointerup', onDragEnd);
  useEventListener(() => (isResizingPanel ? window : null), 'pointermove', onPanelResizeMove);
  useEventListener(() => (isResizingPanel ? window : null), 'pointerup', onPanelResizeEnd);

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

  // Zoom config. The quarter view keeps a five-year planning horizon and all
  // views add columns until the timeline fills its available width.
  let zoomConfig = $derived.by(() => {
    const base = zoom === 'week'
      ? { columnDays: 1, headerFormat: 'week', baseVisibleColumns: 42, navigationColumns: 21 }
      : zoom === 'quarter'
        ? { columnDays: 30, headerFormat: 'quarter', baseVisibleColumns: QUARTER_PLANNING_MONTHS, navigationColumns: 6 }
        : { columnDays: 7, headerFormat: 'month', baseVisibleColumns: 20, navigationColumns: 10 };
    const viewportColumns = Math.ceil(containerWidth / ZOOM_COLUMN_WIDTHS[zoom]);
    return {
      ...base,
      visibleColumns: Math.max(base.baseVisibleColumns, viewportColumns),
    };
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
        const { start: startVal, end: endVal } = getEffectiveDateRange(item);
        if (startVal) {
          const pos = dateToColPos(parseRoadmapDate(startVal));
          if (pos < -prependCount) prependCount = Math.ceil(Math.abs(pos)) + 1;
        }
        const farVal = endVal || startVal;
        if (farVal) {
          const d = parseRoadmapDate(farVal);
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
    const itemIds = new Set(allItemsSorted.map(i => i.id));
    return allItemsSorted.filter(item => item.parent_id === null || !itemIds.has(item.parent_id));
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
        { iconName: 'GitBranch', color: '#9333ea', label: 'Epic' },
        { iconName: 'Circle', color: '#2563eb', label: 'Feature' },
        { iconName: 'Circle', color: '#16a34a', label: 'Story' },
        { iconName: 'Circle', color: '#ea580c', label: 'Task' },
        { iconName: 'Circle', color: '#6b7280', label: 'Subtask' }
      ];
      return fallback[Math.min(item.level || 0, fallback.length - 1)];
    }
    const it = itemTypes.find(type => type.id === item.item_type_id);
    if (it) {
      return {
        iconName: it.icon,
        color: it.color,
        label: it.name
      };
    }
    return { iconName: 'Circle', color: '#6b7280', label: 'Unknown' };
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
    const { start, end } = getEffectiveDateRange(item);
    return !!(start || end);
  }

  let unscheduledItemCount = $derived(
    collectionStore.loading ? 0 : collectionStore.items.filter((item) => !itemHasDate(item)).length,
  );

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
        const effectiveRange = getEffectiveDateRange(item);
        const { start, end } = effectiveRange;

        if (!start && !end) return null;

        const startDate = start ? parseRoadmapDate(start) : null;
        const endDate = end ? parseRoadmapDate(end) : null;

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
          startDate: startDate ? formatDateValue(startDate) : null,
          endDate: endDate ? formatDateValue(endDate) : null,
          hierarchyAdjusted: effectiveRange.adjusted,
          isSummary: effectiveRange.summary,
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

  function getEffectiveDateRange(item) {
    if (hierarchyProjectionActive) {
      const projected = hierarchyProjection.get(Number(item.id));
      if (projected) {
        return {
          start: projected.startDate,
          end: projected.endDate,
          adjusted: projected.adjusted,
          summary: projected.summary,
        };
      }
    }
    return {
      start: getDateValue(item, roadmapConfig.start_field_id),
      end: roadmapConfig.end_field_id ? getDateValue(item, roadmapConfig.end_field_id) : null,
      adjusted: false,
      summary: false,
    };
  }

  function parseRoadmapDate(value) {
    const match = String(value).match(/^(\d{4})-(\d{2})-(\d{2})/);
    if (match) return new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
    const date = new Date(value);
    date.setHours(0, 0, 0, 0);
    return date;
  }

  // Column width in pixels
  let colWidth = $derived(ZOOM_COLUMN_WIDTHS[zoom]);

  let totalWidth = $derived(colWidth * columns.length);

  // Row height
  const ROW_HEIGHT = 40;
  const DEFAULT_SCHEDULE_DAYS = 7;

  // Sync collection name and load links when items change
  $effect(() => {
    if (!collectionStore.loading) {
      currentCollectionName = collectionStore.collectionName;
      untrack(() => loadLinksForItems(collectionStore.items));
    }
  });

  // Load links for visible items in batched requests instead of one per item.
  async function loadLinksForItems(items) {
    if (!roadmapConfig.dependency_link_type_id) return;
    const ids = items.filter((i) => i?.id != null).map((i) => i.id);
    const newLinks = {};
    for (const id of ids) newLinks[id] = [];
    const chunks = [];
    for (let i = 0; i < ids.length; i += ROADMAP_LINK_CHUNK) {
      chunks.push(ids.slice(i, i + ROADMAP_LINK_CHUNK));
    }
    const groupsPerChunk = await Promise.all(
      chunks.map(async (chunk) => {
        try {
          return await api.links.getForItems(chunk);
        } catch {
          return {};
        }
      })
    );
    for (const groups of groupsPerChunk) {
      for (const [id, group] of Object.entries(groups)) {
        newLinks[id] = group?.outgoing || [];
      }
    }
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

      // Load screen fields to determine available date fields. The roadmap
      // is workspace-scoped (not bound to a single item-type), so resolve
      // against the config-set defaults; mode='edit' falls back through
      // create_screen_id when no edit screen is configured.
      let screenId = null;
      if (workspace?.configuration_set_id) {
        const configSet = await api.configurationSets.get(workspace.configuration_set_id);
        screenId = resolveScreenId(configSet, null, 'edit');
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
      columns: boardConfig?.columns || [],
      backlog_status_ids: boardConfig?.backlog_status_ids || [],
      list_columns: boardConfig?.list_columns || [],
      card_fields: boardConfig?.card_fields || [],
      roadmap_config: {
        start_field_id: roadmapConfig.start_field_id,
        end_field_id: roadmapConfig.end_field_id,
        dependency_link_type_id: roadmapConfig.dependency_link_type_id ? Number(roadmapConfig.dependency_link_type_id) : null,
      },
      show_rightmost_column_last_50: Boolean(boardConfig?.show_rightmost_column_last_50),
      completed_item_retention_days: boardConfig?.completed_item_retention_days ?? null,
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
    if (val !== 'start_date') {
      hierarchyMode = 'off';
      adjustHierarchyDates = false;
    }
    saveConfig();
  }

  function onEndFieldChange(val) {
    roadmapConfig.end_field_id = val;
    if (val !== 'end_date') {
      hierarchyMode = 'off';
      adjustHierarchyDates = false;
    }
    saveConfig();
  }

  function onLinkTypeChange(val) {
    roadmapConfig.dependency_link_type_id = val || null;
    saveConfig();
    if (val) loadLinksForItems(collectionStore.items);
  }

  // Navigation
  function scrollLeft() {
    scrollOffset -= zoomConfig.navigationColumns;
  }

  function scrollRight() {
    scrollOffset += zoomConfig.navigationColumns;
  }

  function goToToday() {
    scrollOffset = 0;
    timelineScrollLeft = 0;
    if (timelineScrollContainer) timelineScrollContainer.scrollLeft = 0;
  }

  // Item click
  function openItem(itemId) {
    selectedItemId = itemId;
    showItemModal = true;
  }

  // --- Bar drag handlers (existing move/resize) ---
  function onBarPointerDown(e, item, mode) {
    if (e.button !== 0 || (hierarchyMode === 'rollup' && item.isSummary)) return;
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
        const newStart = parseRoadmapDate(origStartDate);
        newStart.setDate(newStart.getDate() + actualDays);
        setDateUpdate(updateData, roadmapConfig.start_field_id, newStart);
      } else if (mode === 'resize-left' && origEndDate && roadmapConfig.start_field_id) {
        // Milestone with only end date → expand left: set start = end - |days|
        const newStart = parseRoadmapDate(origEndDate);
        newStart.setDate(newStart.getDate() + actualDays);
        setDateUpdate(updateData, roadmapConfig.start_field_id, newStart);
      }
    }
    if (mode === 'move' || mode === 'resize-right') {
      if (origEndDate) {
        const newEnd = parseRoadmapDate(origEndDate);
        newEnd.setDate(newEnd.getDate() + actualDays);
        setDateUpdate(updateData, roadmapConfig.end_field_id, newEnd);
      } else if (mode === 'resize-right' && origStartDate && roadmapConfig.end_field_id) {
        // Milestone with only start date → expand right: set end = start + days
        const newEnd = parseRoadmapDate(origStartDate);
        newEnd.setDate(newEnd.getDate() + actualDays);
        setDateUpdate(updateData, roadmapConfig.end_field_id, newEnd);
      }
    }

    if (Object.keys(updateData).length > 0) {
      try {
        await persistRoadmapDateEdit(itemId, updateData);
      } catch (e) {
        console.error('Failed to update item dates:', e);
        errorToast(e?.message || t('collections.roadmapDateUpdateError'));
        reloadCollection();
      }
    }
  }

  function formatDateValue(date) {
    return [
      date.getFullYear(),
      String(date.getMonth() + 1).padStart(2, '0'),
      String(date.getDate()).padStart(2, '0'),
    ].join('-');
  }

  function setDateUpdate(data, fieldId, date) {
    const dateStr = formatDateValue(date);
    if (fieldId?.startsWith('cf_')) {
      const cfId = fieldId.replace('cf_', '');
      if (!data.custom_field_values) data.custom_field_values = {};
      data.custom_field_values[cfId] = dateStr;
    } else {
      data[fieldId] = dateStr;
    }
  }

  async function persistRoadmapDateEdit(itemId, updateData) {
    if (adjustHierarchyDates && hierarchyMode !== 'off' && hierarchyFieldsReady) {
      if (!hierarchyProjectionActive) {
        throw new Error(t('collections.roadmapHierarchyNotReady'));
      }
      const fields = {};
      if (Object.hasOwn(updateData, 'start_date')) fields.start_date = updateData.start_date;
      if (Object.hasOwn(updateData, 'end_date')) fields.end_date = updateData.end_date;
      const patches = buildHierarchyDatePatches({
        items: hierarchyDateItems,
        editedItemId: itemId,
        fields,
        mode: hierarchyMode,
      });
      await api.items.bulkPatch(patches);
      reloadCollection();
      await loadHierarchyDates(collectionStore.items.map((item) => item.id));
      return;
    }
    await api.items.update(itemId, updateData);
    reloadCollection();
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
    resizeStartX = e.clientX;
    resizeStartWidth = treePanelWidth;
    isResizingPanel = true;
  }

  function onPanelResizeMove(e) {
    if (!isResizingPanel) return;
    const dx = e.clientX - resizeStartX;
    treePanelWidth = Math.max(200, Math.min(500, resizeStartWidth + dx));
  }

  function onPanelResizeEnd() {
    isResizingPanel = false;
  }

  // --- Click-to-schedule handlers ---
  function dateAtTimelineX(timelineX) {
    const dayOffset = timelineX / colWidth - gridOffset;
    const date = new Date(referenceDate);
    if (zoom === 'quarter') {
      const monthOffset = Math.floor(dayOffset);
      const frac = dayOffset - monthOffset;
      date.setMonth(date.getMonth() + monthOffset);
      const nextMonth = new Date(date.getFullYear(), date.getMonth() + 1, 1);
      const monthStart = new Date(date.getFullYear(), date.getMonth(), 1);
      const daysInMonth = (nextMonth.getTime() - monthStart.getTime()) / (1000 * 60 * 60 * 24);
      date.setDate(1 + Math.floor(frac * daysInMonth / snapDays) * snapDays);
    } else {
      const days = Math.floor(dayOffset * zoomConfig.columnDays / snapDays) * snapDays;
      date.setDate(date.getDate() + days);
    }
    return date;
  }

  function scheduleRange(startDate) {
    const start = new Date(startDate);
    const end = roadmapConfig.end_field_id ? new Date(startDate) : null;
    if (end) end.setDate(end.getDate() + DEFAULT_SCHEDULE_DAYS - 1);

    const endExclusive = new Date(end || start);
    endExclusive.setDate(endExclusive.getDate() + 1);

    return {
      start,
      end,
      leftPx: (dateToColPos(start) + gridOffset) * colWidth,
      widthPx: Math.max((dateToColPos(endExclusive) - dateToColPos(start)) * colWidth, 8),
    };
  }

  function scheduleRangeAtPointer(e) {
    const rowRect = e.currentTarget.getBoundingClientRect();
    return scheduleRange(dateAtTimelineX(e.clientX - rowRect.left));
  }

  function onSchedulePointerMove(e, item) {
    if (e.pointerType === 'touch' || itemHasDate(item) || schedulingItemIds.has(item.id)) return;
    schedulePreview = { itemId: item.id, ...scheduleRangeAtPointer(e) };
  }

  function onSchedulePointerLeave(itemId) {
    if (schedulePreview?.itemId === itemId) schedulePreview = null;
  }

  async function scheduleItem(item, range) {
    if (itemHasDate(item) || schedulingItemIds.has(item.id)) return;

    const updateData = {};
    setDateUpdate(updateData, roadmapConfig.start_field_id, range.start);

    if (range.end) setDateUpdate(updateData, roadmapConfig.end_field_id, range.end);

    if (Object.keys(updateData).length > 0) {
      schedulingItemIds = new Set(schedulingItemIds).add(item.id);
      schedulePreview = null;
      try {
        await persistRoadmapDateEdit(item.id, updateData);
        await refreshCollectionItem(item.id);
      } catch (err) {
        console.error('Failed to schedule item:', err);
        errorToast(err?.message || t('collections.roadmapDateUpdateError'));
        reloadCollection();
      } finally {
        const nextSchedulingIds = new Set(schedulingItemIds);
        nextSchedulingIds.delete(item.id);
        schedulingItemIds = nextSchedulingIds;
      }
    }
  }

  function onScheduleClick(e, item) {
    if (itemHasDate(item) || schedulingItemIds.has(item.id)) return;
    if (e.detail !== 0) {
      scheduleItem(item, schedulePreview?.itemId === item.id ? schedulePreview : scheduleRangeAtPointer(e));
      return;
    }
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    scheduleItem(item, scheduleRange(today));
  }

  // --- Scroll sync ---
  function syncTreeScroll(e) {
    if (timelineScrollContainer && timelineScrollContainer.scrollTop !== e.target.scrollTop) {
      timelineScrollContainer.scrollTop = e.target.scrollTop;
    }
  }

  function syncTimelineScroll(e) {
    timelineScrollLeft = e.target.scrollLeft;
    if (treeScrollContainer && treeScrollContainer.scrollTop !== e.target.scrollTop) {
      treeScrollContainer.scrollTop = e.target.scrollTop;
    }
  }

  let isAwayFromToday = $derived(scrollOffset !== 0 || timelineScrollLeft > 0);

  useResizeObserver(() => timelineScrollContainer, (entries) => {
    const entry = entries[0];
    if (entry) containerWidth = entry.contentRect.width;
  });

  onMount(async () => {
    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
      await workspaceDataStore.initialize(workspaceId);
    } else {
      await workspaceDataStore.initializeGlobal();
    }
    await Promise.all([loadConfig(), loadReferenceData()]);
    loading = false;
  });
</script>

{#if loading}
  <StaticViewBackground
    backgroundStyle={styles.backgroundStyle}
    contextVars={styles.contextVars}
    contentClass="flex items-center justify-center min-h-screen"
  >
    <div class="animate-pulse" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
      {t('collections.roadmapSettings')}...
    </div>
  </StaticViewBackground>
{:else}
  <StaticViewBackground
    backgroundStyle={styles.backgroundStyle}
    contextVars={styles.contextVars}
    class="h-screen flex flex-col overflow-hidden {isResizingPanel ? 'roadmap-dragging' : ''}"
    contentClass="p-6 flex-1 flex flex-col min-h-0 min-w-0 overflow-hidden"
    rootStyle="width: 100%; min-width: 0; max-width: 100%; overscroll-behavior: none;"
    testid="roadmap-view"
  >
      <!-- Header -->
      <div class="relative z-50 mb-6">
        <ViewHeader
          workspaceName={workspace?.name || ''}
          collection={currentCollectionName === 'Default' ? t('common.default') : currentCollectionName}
          viewName={t('collections.roadmap')}
          itemCount={treeData.length}
        >
          {#snippet actions()}
            <div class="relative flex rounded" style="background-color: var(--ctx-surface, var(--ds-background-neutral)); backdrop-filter: var(--ctx-backdrop, none);">
              <button
                bind:this={settingsButton}
                onclick={() => settingsOpen = !settingsOpen}
                data-testid="roadmap-settings-button"
                class="inline-flex items-center gap-2 px-3 py-1.5 text-sm font-medium rounded transition-colors"
                style="color: var(--ds-text); background-color: var(--ctx-surface, var(--ds-surface));"
              >
                <Settings class="w-4 h-4" />
                {t('collections.roadmapSettings')}
              </button>
              {#if settingsOpen}
                <div
                  bind:this={settingsPanel}
                  class="absolute right-0 top-full mt-1 rounded-lg shadow-xl z-[60] p-4"
                  style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border); min-width: 300px;"
                  data-testid="roadmap-settings-panel"
                  transition:fly={{ y: -4, duration: 150 }}
                >
                  <div class="flex flex-col gap-3">
                    <div>
                      <div class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('collections.roadmapStartField')}</div>
                      <Select
                        id="roadmap-start-field"
                        value={roadmapConfig.start_field_id}
                        options={dateFieldOptions}
                        size="small"
                        disabled={!canConfigure}
                        portalOwner="roadmap-settings"
                        onchange={onStartFieldChange}
                      />
                    </div>
                    <div>
                      <div class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('collections.roadmapEndField')}</div>
                      <Select
                        id="roadmap-end-field"
                        value={roadmapConfig.end_field_id}
                        options={[{ value: '', label: t('collections.roadmapNone') }, ...dateFieldOptions]}
                        size="small"
                        disabled={!canConfigure}
                        portalOwner="roadmap-settings"
                        onchange={onEndFieldChange}
                      />
                    </div>
                    <div>
                      <div class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('collections.roadmapDependencyLinkType')}</div>
                      <Select
                        id="roadmap-dependency-link-type"
                        value={roadmapConfig.dependency_link_type_id ? String(roadmapConfig.dependency_link_type_id) : ''}
                        options={linkTypeOptions}
                        size="small"
                        disabled={!canConfigure}
                        portalOwner="roadmap-settings"
                        onchange={onLinkTypeChange}
                      />
                    </div>
                    <div class="pt-3" style="border-top: 1px solid var(--ds-border);">
                      <div class="block text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">
                        {t('collections.roadmapHierarchyMode')}
                      </div>
                      <div class="grid grid-cols-3 gap-1" role="group" aria-label={t('collections.roadmapHierarchyMode')}>
                        {#each [
                          ['off', t('collections.roadmapHierarchyOff')],
                          ['rollup', t('collections.roadmapRollup')],
                          ['rolldown', t('collections.roadmapRolldown')]
                        ] as [mode, label]}
                          <button
                            type="button"
                            data-testid="roadmap-hierarchy-{mode}"
                            aria-pressed={hierarchyMode === mode}
                            disabled={mode !== 'off' && !hierarchyFieldsReady}
                            class="rounded px-2 py-1.5 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-40"
                            style={hierarchyMode === mode
                              ? 'background-color: var(--ds-accent-blue-subtle); color: var(--ds-text-info); border: 1px solid var(--ds-border-focused);'
                              : 'background-color: var(--ds-background-neutral); color: var(--ds-text-subtle); border: 1px solid transparent;'}
                            onclick={() => setHierarchyMode(mode)}
                          >
                            {label}
                          </button>
                        {/each}
                      </div>
                      {#if !hierarchyFieldsReady}
                        <p class="mt-2 text-xs leading-4" style="color: var(--ds-text-subtle);">
                          {t('collections.roadmapHierarchyCanonicalHint')}
                        </p>
                      {:else if hierarchyDatesError}
                        <p class="mt-2 text-xs leading-4" style="color: var(--ds-text-danger);" role="alert">
                          {hierarchyDatesError}
                        </p>
                      {/if}
                    </div>
                    <div class="flex items-start justify-between gap-4">
                      <div>
                        <div class="text-sm" style="color: var(--ds-text);">
                          {t('collections.roadmapAdjustRelatedDates')}
                        </div>
                        <p class="mt-0.5 text-xs leading-4" style="color: var(--ds-text-subtle);">
                          {hierarchyMode === 'rolldown'
                            ? t('collections.roadmapRolldownAdjustmentHint')
                            : t('collections.roadmapRollupAdjustmentHint')}
                        </p>
                      </div>
                      <Toggle
                        bind:checked={adjustHierarchyDates}
                        size="small"
                        id="roadmap-adjust-related-dates"
                        dataTestid="roadmap-adjust-related-dates"
                        disabled={hierarchyMode === 'off' || hierarchyDatesLoading || Boolean(hierarchyDatesError)}
                      />
                    </div>
                  </div>
                </div>
              {/if}
            </div>
          {/snippet}
        </ViewHeader>
      </div>

      <!-- Controls Bar -->
      <div class="relative z-40 flex items-center mb-4">
        <SubFilterBar {workspaceId} />
      </div>

      <!-- Toolbar: Zoom + Navigation + Settings toggle -->
      <div class="flex items-center justify-between mb-4 gap-3">
        <div class="flex items-center gap-2">
          <!-- Zoom controls -->
          <div class="flex rounded overflow-hidden" style="border: 1px solid var(--ctx-border, var(--ds-border)); background-color: var(--ctx-surface, var(--ds-surface));">
            {#each [['week', t('collections.roadmapZoomWeek')], ['month', t('collections.roadmapZoomMonth')], ['quarter', t('collections.roadmapZoomQuarter')]] as [z, label]}
              <button
                data-testid="roadmap-zoom-{z}"
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
            data-testid="roadmap-scroll-left"
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
            data-testid="roadmap-scroll-right"
            class="p-1.5 rounded transition-colors"
            style="color: var(--ctx-text-subtle, var(--ds-text-subtle));"
            onclick={scrollRight}
          >
            <ChevronRight class="w-4 h-4" />
          </button>
        </div>

        <div class="flex items-center justify-end">
          {#if isAwayFromToday}
            <button
              data-testid="roadmap-return-today"
              class="inline-flex items-center gap-1.5 rounded px-2 py-1.5 text-xs font-medium transition-colors"
              style="color: var(--ds-text-subtle); border: 1px solid var(--ds-border-subtle, var(--ds-border)); background-color: transparent;"
              title={t('collections.roadmapToday')}
              aria-label={t('collections.roadmapToday')}
              onclick={goToToday}
            >
              <RotateCcw class="h-3.5 w-3.5" />
              {t('collections.roadmapToday')}
            </button>
          {/if}
        </div>

      </div>

      {#if roadmapConfig.start_field_id && unscheduledItemCount > 0}
        <div
          class="mb-4 flex items-center gap-2 rounded-md px-3 py-2 text-xs"
          style="background-color: var(--ds-background-neutral-hovered); color: var(--ctx-text, var(--ds-text));"
          data-testid="roadmap-unscheduled-hint"
        >
          <CalendarClock class="h-4 w-4 shrink-0" />
          <span>{t('collections.roadmapUnscheduledHint', { count: unscheduledItemCount })}</span>
        </div>
      {/if}


      <!-- No config state -->
      {#if !roadmapConfig.start_field_id}
        <div class="flex flex-col items-center justify-center py-20" style="color: var(--ctx-text-subtle, var(--ds-text-subtle));">
          <p class="text-sm">{t('collections.roadmapNoConfig')}</p>
          <button
            class="mt-3 px-4 py-2 text-sm font-medium rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            style="background-color: var(--ds-accent-blue-subtle); color: var(--ds-text-info);"
            onclick={() => settingsOpen = true}
          >
            {t('collections.roadmapSettings')}
          </button>
        </div>
      {:else}
        <!-- Split layout: Tree Panel + Resize Handle + Timeline -->
        <div
          class="rounded-lg overflow-hidden flex flex-1 min-h-0 min-w-0 w-full"
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
                {t('collections.roadmapItems', { count: treeData.length })}
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
                <button
                  data-testid="roadmap-item-{item.id}"
                  class="flex items-center gap-1.5 w-full text-left group/tree-row"
                  style="height: {ROW_HEIGHT}px; padding-left: calc(8px + {getIndentLevel(item.treeLevel)}); padding-right: 8px; border-bottom: 1px solid var(--ds-border-subtle, var(--ds-border));"
                  onclick={() => openItem(item.id)}
                >
                  <LazyRender height={ROW_HEIGHT} class="flex items-center gap-1.5 w-full">
                    {#snippet children()}
                      <!-- Expand/collapse chevron -->
                      {#if item.hasChildren}
                        <span
                          class="shrink-0 flex items-center justify-center w-4 h-4 rounded transition-colors"
                          style="color: var(--ds-text-subtle);"
                          role="button"
                          tabindex="-1"
                          onclick={(e) => { e.stopPropagation(); toggleExpanded(item.id); }}
                          onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); toggleExpanded(item.id); } }}
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
                      <ItemTypeIcon
                        icon={typeInfo.iconName}
                        color={typeInfo.color}
                        title={typeInfo.label}
                      />

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
                    {/snippet}
                  </LazyRender>
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
              data-testid="roadmap-timeline-scroll"
              bind:this={timelineScrollContainer}
              onscroll={syncTimelineScroll}
            >
              <div data-testid="roadmap-timeline-content" style="min-width: {totalWidth}px;">
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
                      <LazyRender height={ROW_HEIGHT}>
                        {#snippet children()}
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

                          {#if !roadmapItem}
                            <button
                              type="button"
                              data-testid="roadmap-schedule-row-{item.id}"
                              class="absolute inset-0 w-full roadmap-schedule-row"
                              aria-label={t('collections.roadmapScheduleItem', { title: item.title })}
                              onpointermove={(e) => onSchedulePointerMove(e, item)}
                              onpointerleave={() => onSchedulePointerLeave(item.id)}
                              onclick={(e) => onScheduleClick(e, item)}
                            ></button>
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
                              <RoadmapItemPreview
                                item={roadmapItem}
                                workspace={workspace}
                                itemTypes={itemTypes}
                                cardFields={roadmapCardFields}
                                priorities={workspaceDataStore.priorities}
                                statuses={statuses}
                                iterations={workspaceDataStore.iterations}
                                projects={workspaceDataStore.projects}
                                labels={workspaceDataStore.labels}
                                customFieldDefinitions={customFields}
                                users={workspaceDataStore.users}
                                onpointerdown={(e) => onBarPointerDown(e, roadmapItem, 'move')}
                                onopen={openItem}
                              >
                                {#snippet children()}
                                  <div
                                    data-testid="roadmap-bar-{item.id}"
                                    data-start-date={roadmapItem.startDate || ''}
                                    data-end-date={roadmapItem.endDate || ''}
                                    data-hierarchy-summary={roadmapItem.isSummary ? 'true' : 'false'}
                                    data-hierarchy-adjusted={roadmapItem.hierarchyAdjusted ? 'true' : 'false'}
                                    class="absolute flex items-center justify-center"
                                    style="left: {barLeftPx + barWidthPx / 2 - 8}px; top: {(ROW_HEIGHT - 16) / 2}px; width: 16px; height: 16px; transform: rotate(45deg); background-color: {visibleColor}; border-radius: 2px; z-index: 10; cursor: {roadmapItem.isSummary ? 'default' : 'grab'};"
                                    role="button"
                                    tabindex="0"
                                  ></div>
                                {/snippet}
                              </RoadmapItemPreview>
                            {:else}
                              <!-- Range bar OR expandable milestone (thin bar with resize handles) -->
                              <RoadmapItemPreview
                                item={roadmapItem}
                                workspace={workspace}
                                itemTypes={itemTypes}
                                cardFields={roadmapCardFields}
                                priorities={workspaceDataStore.priorities}
                                statuses={statuses}
                                iterations={workspaceDataStore.iterations}
                                projects={workspaceDataStore.projects}
                                labels={workspaceDataStore.labels}
                                customFieldDefinitions={customFields}
                                users={workspaceDataStore.users}
                                onpointerdown={(e) => onBarPointerDown(e, roadmapItem, 'move')}
                                onopen={openItem}
                              >
                                {#snippet children()}
                                  <div
                                    data-testid="roadmap-bar-{item.id}"
                                    data-start-date={roadmapItem.startDate || ''}
                                    data-end-date={roadmapItem.endDate || ''}
                                    data-hierarchy-summary={roadmapItem.isSummary ? 'true' : 'false'}
                                    data-hierarchy-adjusted={roadmapItem.hierarchyAdjusted ? 'true' : 'false'}
                                    class="absolute flex items-center rounded group/bar"
                                    style="left: {barLeftPx}px; width: {barWidthPx}px; top: {(ROW_HEIGHT - 24) / 2}px; height: 24px; background-color: {visibleColor}; opacity: {roadmapItem.isSummary ? '0.65' : '0.85'}; z-index: 10; cursor: {roadmapItem.isSummary ? 'default' : 'grab'}; border: {roadmapItem.isSummary ? '1px dashed currentColor' : 'none'};"
                                    role="button"
                                    tabindex="0"
                                  >
                                {#if !roadmapItem.isSummary}
                                  <div
                                    data-testid="roadmap-resize-left-{item.id}"
                                    class="absolute left-0 top-0 bottom-0 w-2 cursor-col-resize opacity-0 group-hover/bar:opacity-100 rounded-l"
                                    style="background-color: rgba(0,0,0,0.2);"
                                    role="separator"
                                    tabindex="-1"
                                    onpointerdown={(e) => { e.stopPropagation(); onBarPointerDown(e, roadmapItem, 'resize-left'); }}
                                  ></div>
                                {/if}

                                <span class="text-xs font-medium px-2 truncate" style="color: white; text-shadow: 0 1px 2px rgba(0,0,0,0.3);">
                                  {#if barWidthPx > 60}
                                    {roadmapItem.title}
                                  {/if}
                                </span>

                                {#if !roadmapItem.isSummary}
                                  <div
                                    data-testid="roadmap-resize-right-{item.id}"
                                    class="absolute right-0 top-0 bottom-0 w-2 cursor-col-resize opacity-0 group-hover/bar:opacity-100 rounded-r"
                                    style="background-color: rgba(0,0,0,0.2);"
                                    role="separator"
                                    tabindex="-1"
                                    onpointerdown={(e) => { e.stopPropagation(); onBarPointerDown(e, roadmapItem, 'resize-right'); }}
                                  ></div>
                                {/if}
                                  </div>
                                {/snippet}
                              </RoadmapItemPreview>
                            {/if}
                          {:else if schedulePreview?.itemId === item.id}
                            {@const previewColor = getVisibleColor(statuses.find(s => s.id === item.status_id)?.color || '#6b7280')}
                            {@const previewStart = schedulePreview.start.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
                            {@const previewEnd = schedulePreview.end?.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
                            <div
                              data-testid="roadmap-schedule-preview-{item.id}"
                              data-start-date={formatDateValue(schedulePreview.start)}
                              data-end-date={schedulePreview.end ? formatDateValue(schedulePreview.end) : ''}
                              class="absolute rounded schedule-preview"
                              style="left: {schedulePreview.leftPx}px; width: {schedulePreview.widthPx}px; top: {(ROW_HEIGHT - 24) / 2}px; height: 24px; border-color: {previewColor}; background-color: color-mix(in srgb, {previewColor} 22%, transparent);"
                              aria-label={previewEnd ? `${previewStart} – ${previewEnd}` : previewStart}
                              title={previewEnd ? `${previewStart} – ${previewEnd}` : previewStart}
                            ></div>
                          {/if}
                        {/snippet}
                      </LazyRender>
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
  </StaticViewBackground>
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

  .roadmap-schedule-row {
    cursor: crosshair;
    z-index: 1;
    border: 0;
    background: transparent;
  }

  .roadmap-schedule-row:focus-visible {
    outline: 2px solid var(--ds-accent-blue);
    outline-offset: -2px;
  }

  .schedule-preview {
    z-index: 9;
    pointer-events: none;
    border-width: 1px;
    border-style: dashed;
    opacity: 0.9;
    transition: left 80ms ease-out;
  }

  /* Disable text selection during drag/resize */
  :global(.roadmap-dragging) {
    user-select: none;
  }
</style>
