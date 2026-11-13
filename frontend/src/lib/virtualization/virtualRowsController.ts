import { defaultRangeExtractor, measureElement, Virtualizer } from '@tanstack/virtual-core';
import { tick } from 'svelte';
import {
  indexRows,
  ROW_OVERSCAN,
  renderTokens,
  repairRowKeys,
  sparseIndexes,
  VIRTUAL_ROW_THRESHOLD,
} from './rowProjection';
import {
  recalledVirtualAnchor,
  rememberVirtualAnchor,
  virtualAudienceRevision,
} from './scrollRestoration';
import type { RowPosition, ScrollAnchor, ScrollRoot, VirtualRow } from './types';
import { listOrigin, refreshViewport, rootOffset, subscribeViewport } from './viewportRegistry';

export interface ControllerOptions {
  onChange: () => void;
}

export class VirtualRowsController<T = unknown> {
  rows: readonly VirtualRow<T>[] = [];
  indexes = new Map<string, number>();
  private paneNodes = new Map<string, Map<string, HTMLElement>>();
  private pins = new Map<string, Set<symbol>>();
  private root: ScrollRoot | null = null;
  private list: HTMLElement | null = null;
  private margin = 0;
  private width = 0;
  private identity = '';
  private audienceRevision = virtualAudienceRevision();
  private layout = '';
  private fixedSize: number | undefined;
  private enabled = true;
  private printing = false;
  private printAnchor: ScrollAnchor | null = null;
  private overscan = ROW_OVERSCAN;
  private generation = 0;
  private epoch = 0;
  private getKey = (index: number) => this.rows[index].key;
  private listObserver: ResizeObserver | null = null;
  private unmount: (() => void) | null = null;
  private destroyed = false;
  private changing = false;
  private warnedBadKeys = false;
  private change: () => void;
  readonly engine: Virtualizer<ScrollRoot, HTMLElement>;

  constructor({ onChange }: ControllerOptions) {
    this.change = onChange;
    this.engine = new Virtualizer(this.options());
  }

  get virtual() {
    return (
      this.enabled &&
      !this.printing &&
      this.rows.length > VIRTUAL_ROW_THRESHOLD &&
      typeof ResizeObserver !== 'undefined' &&
      this.root !== null
    );
  }

  get totalSize() {
    return this.engine.getTotalSize();
  }
  get pinnedKeys() {
    return [...this.pins.keys()];
  }
  get tokens() {
    if (!this.virtual) {
      return this.rows.map((row, index) => ({
        kind: 'row' as const,
        key: `${this.epoch}:row:${row.key}`,
        row: {
          key: row.key,
          index,
          start: 0,
          end: 0,
          size: row.estimatedSize,
        },
      }));
    }
    const rows = this.engine.getVirtualItems().map((row) => ({
      ...row,
      key: String(row.key),
      start: row.start - this.margin,
      end: row.end - this.margin,
    }));
    return renderTokens(rows, this.totalSize).map((token) => ({
      ...token,
      key: `${this.epoch}:${token.key}`,
    }));
  }

  get visibleKeys() {
    if (!this.virtual) return this.rows.map((row) => row.key);
    this.engine.calculateRange();
    const range = this.engine.range;
    return range ? this.rows.slice(range.startIndex, range.endIndex + 1).map((row) => row.key) : [];
  }

  private options() {
    return {
      count: this.rows.length,
      getScrollElement: () => this.root,
      getItemKey: this.getKey,
      estimateSize: (index: number) => this.fixedSize ?? this.rows[index].estimatedSize,
      scrollMargin: this.margin,
      overscan: this.overscan,
      enabled: this.virtual,
      onChange: () => {
        if (!this.changing && !this.destroyed) this.change();
      },
      rangeExtractor: (range: Parameters<typeof defaultRangeExtractor>[0]) =>
        sparseIndexes(defaultRangeExtractor(range), this.pins.keys(), this.indexes),
      observeElementRect: (
        _: unknown,
        callback: (rect: { width: number; height: number }) => void
      ) => {
        if (!this.root) return;
        return subscribeViewport(this.root, ({ width, height }) => {
          if (width !== this.width) {
            const changedWidth = this.width !== 0;
            this.width = width;
            if (changedWidth && !this.fixedSize) this.engine.measure();
            this.refreshLayout();
          }
          callback({ width, height });
        });
      },
      observeElementOffset: (
        _: unknown,
        callback: (offset: number, scrolling: boolean) => void
      ) => {
        if (!this.root) return;
        return subscribeViewport(this.root, ({ offset, scrolling }) => callback(offset, scrolling));
      },
      scrollToFn: (offset: number, { adjustments = 0 }: { adjustments?: number }) => {
        this.root?.scrollTo({ top: offset + adjustments, behavior: 'instant' });
        if (this.root) refreshViewport(this.root);
      },
      measureElement: (
        node: HTMLElement,
        entry: ResizeObserverEntry | undefined,
        instance: Virtualizer<ScrollRoot, HTMLElement>
      ) => {
        const size = measureElement(node, entry, instance);
        return size > 0
          ? size
          : (instance.itemSizeCache.get(this.rows[instance.indexFromElement(node)]?.key) ??
              this.rows[instance.indexFromElement(node)]?.estimatedSize ??
              48);
      },
    };
  }

  configure({
    rows,
    identity,
    enabled = true,
    fixedSize,
    layout = '',
    overscan = ROW_OVERSCAN,
  }: {
    rows: readonly VirtualRow<T>[];
    identity: string;
    enabled?: boolean;
    fixedSize?: number;
    layout?: string;
    overscan?: number;
  }) {
    const repaired = repairRowKeys(rows);
    if (repaired !== rows && !this.warnedBadKeys) {
      this.warnedBadKeys = true;
      console.warn(
        'VirtualRows: duplicate or empty occurrence keys; using positional fallback keys for malformed rows.'
      );
    }
    rows = repaired;
    const indexes = indexRows(rows);
    const changedIdentity = this.identity !== identity;
    const changedLayout = layout !== this.layout || fixedSize !== this.fixedSize;
    const wasVirtual = this.virtual;
    const anchor = this.captureAnchor();
    const oldRows = this.rows;
    if (changedIdentity && this.identity)
      rememberVirtualAnchor(this.identity, anchor, this.audienceRevision);
    this.changing = true;
    try {
      this.rows = rows;
      this.indexes = indexes;
      this.getKey = (index) => rows[index].key;
      this.enabled = enabled;
      this.fixedSize = fixedSize;
      this.overscan = overscan;
      if (changedIdentity) {
        this.generation++;
        this.epoch++;
        this.pins.clear();
        this.engine.cancelScroll();
        this.engine.measure();
      }
      for (const key of this.engine.itemSizeCache.keys()) {
        if (!this.indexes.has(String(key))) this.engine.itemSizeCache.delete(key);
      }
      for (const key of this.pins.keys()) if (!this.indexes.has(key)) this.pins.delete(key);
      this.engine.setOptions(this.options());
      this.engine._willUpdate();
      if (changedLayout) this.engine.measure();
      this.layout = layout;
      const previousIdentity = this.identity;
      this.identity = identity;
      if (changedIdentity) {
        const saved = recalledVirtualAnchor(identity, this.audienceRevision);
        if (saved) this.restoreAnchor(saved);
        else if (previousIdentity && this.root) this.engine.scrollToOffset(this.margin);
      } else if (anchor && oldRows !== rows) {
        const nextAnchor = this.survivingAnchor(anchor, oldRows);
        if (nextAnchor) this.restoreAnchor(nextAnchor);
      }
      if (anchor && !changedIdentity && (wasVirtual !== this.virtual || changedLayout)) {
        const generation = this.generation;
        void tick().then(() => {
          if (!this.destroyed && generation === this.generation) this.restoreAnchor(anchor);
        });
      }
    } finally {
      this.changing = false;
    }
    this.change();
  }

  connect(root: ScrollRoot | null, list: HTMLElement | null) {
    if (this.root === root && this.list === list) return;
    this.listObserver?.disconnect();
    this.root = root;
    this.list = list;
    this.margin = root && list ? listOrigin(root, list) : 0;
    this.engine.setOptions(this.options());
    this.engine._willUpdate();
    if (!this.unmount) this.unmount = this.engine._didMount();
    if (list && typeof ResizeObserver !== 'undefined') {
      this.listObserver = new ResizeObserver(() => this.refreshLayout());
      this.listObserver.observe(list);
    }
    this.change();
  }

  refreshLayout() {
    if (!this.root || !this.list || this.destroyed) return;
    const margin = listOrigin(this.root, this.list);
    if (Math.abs(margin - this.margin) < 0.5) return;
    const anchor = this.captureAnchor();
    this.margin = margin;
    this.engine.setOptions(this.options());
    if (anchor) this.restoreAnchor(anchor);
    this.change();
  }

  captureAnchor(): ScrollAnchor | null {
    if (!this.root || !this.rows.length) return null;
    if (!this.virtual) {
      const rootTop =
        'clientTop' in this.root ? this.root.getBoundingClientRect().top + this.root.clientTop : 0;
      for (const [index, row] of this.rows.entries()) {
        const rect = this.getNode(row.key)?.getBoundingClientRect();
        if (rect && rect.height > 0 && rect.bottom > rootTop)
          return { key: row.key, index, offset: rootTop - rect.top };
      }
      return null;
    }
    const position = this.engine.getVirtualItemForOffset(rootOffset(this.root));
    return position
      ? {
          key: String(position.key),
          index: position.index,
          offset: rootOffset(this.root) - position.start,
        }
      : null;
  }

  private survivingAnchor(anchor: ScrollAnchor, previous: readonly VirtualRow<T>[]) {
    if (this.indexes.has(anchor.key)) return anchor;
    const successor = previous.slice(anchor.index + 1).find((row) => this.indexes.has(row.key));
    const predecessor = previous
      .slice(0, anchor.index)
      .reverse()
      .find((row) => this.indexes.has(row.key));
    const key = successor?.key ?? predecessor?.key ?? this.rows[0]?.key;
    return key ? { key, index: this.indexes.get(key) ?? 0, offset: 0 } : null;
  }

  restoreAnchor(anchor: ScrollAnchor) {
    const index = this.indexes.get(anchor.key);
    if (index === undefined) return;
    if (!this.virtual) {
      const node = this.getNode(anchor.key);
      if (!node || !this.root) return;
      const rootTop =
        'clientTop' in this.root ? this.root.getBoundingClientRect().top + this.root.clientTop : 0;
      this.root.scrollTo({
        top: rootOffset(this.root) + node.getBoundingClientRect().top - rootTop + anchor.offset,
        behavior: 'instant',
      });
      return;
    }
    const offset = this.engine.getOffsetForIndex(index, 'start')?.[0];
    if (offset !== undefined) this.engine.scrollToOffset(offset + anchor.offset);
  }

  acquirePin(key: string, _reason: string) {
    if (!this.indexes.has(key)) return () => {};
    const owner = Symbol(key);
    const owners = this.pins.get(key) ?? new Set();
    owners.add(owner);
    this.pins.set(key, owners);
    this.updateRange();
    let released = false;
    return () => {
      if (released) return;
      released = true;
      owners.delete(owner);
      if (!owners.size && this.pins.get(key) === owners) this.pins.delete(key);
      if (!this.destroyed) this.updateRange();
    };
  }

  private updateRange() {
    this.engine.setOptions(this.options());
    this.change();
  }

  setOverscan(overscan: number) {
    this.overscan = overscan;
    this.updateRange();
  }

  getNode(key: string, pane?: string): HTMLElement | undefined {
    const panes = this.paneNodes.get(key);
    if (!panes) return;
    if (pane !== undefined) return panes.get(pane);
    for (const node of panes.values()) {
      if (this.list?.contains(node)) return node;
    }
    // A single-pane consumer can attach before its root/list is connected.
    // Never choose an arbitrary pane when a paired surface is ambiguous.
    if (!this.list && panes.size === 1) return panes.values().next().value;
  }

  ownsNode(node: HTMLElement) {
    const key = node.dataset.virtualKey;
    const pane = node.dataset.virtualPane;
    return key !== undefined && pane !== undefined && this.getNode(key, pane) === node;
  }

  attach(node: HTMLElement, key: string, index: number, pane = 'main') {
    node.dataset.virtualKey = key;
    node.dataset.virtualPane = pane;
    node.dataset.index = String(index);
    const panes = this.paneNodes.get(key) ?? new Map();
    panes.set(pane, node);
    this.paneNodes.set(key, panes);
    if (this.virtual && !this.fixedSize) this.engine.measureElement(node);
    return {
      update: (next: { key: string; index: number }) => {
        node.dataset.index = String(next.index);
        if (this.virtual && !this.fixedSize) this.engine.measureElement(node);
      },
      destroy: () => {
        if (panes.get(pane) === node) panes.delete(pane);
        if (!panes.size) this.paneNodes.delete(key);
        queueMicrotask(() => {
          if (!this.destroyed) this.engine.measureElement(null);
        });
      },
    };
  }

  async moveFocus(event: KeyboardEvent, key: string) {
    if (
      event.key !== 'Tab' ||
      event.defaultPrevented ||
      event.isComposing ||
      event.ctrlKey ||
      event.metaKey ||
      event.altKey
    )
      return;
    const node = event.currentTarget as HTMLElement | null;
    const controls = node ? focusableElements(node) : [];
    const edge = event.shiftKey ? controls[0] : controls.at(-1);
    if (!edge || edge !== node?.ownerDocument.activeElement) return;
    const direction = event.shiftKey ? -1 : 1;
    let index = (this.indexes.get(key) ?? -1) + direction;
    while (index >= 0 && index < this.rows.length && this.rows[index].focusable === false)
      index += direction;
    if (index < 0 || index >= this.rows.length) return;
    event.preventDefault();
    const target = this.rows[index].key;
    if ((await this.scrollToKey(target, { pane: node?.dataset.virtualPane })) !== 'scrolled')
      return;
    const next = this.paneNodes.get(target)?.get(node?.dataset.virtualPane ?? 'main');
    const nextControls = next ? focusableElements(next) : [];
    (event.shiftKey ? nextControls.at(-1) : nextControls[0])?.focus({ preventScroll: true });
  }

  async scrollToKey(
    key: string,
    {
      align = 'auto',
      signal,
      pane,
    }: { align?: 'start' | 'end' | 'center' | 'auto'; signal?: AbortSignal; pane?: string } = {}
  ) {
    const generation = this.generation;
    const index = this.indexes.get(key);
    if (index === undefined) return 'missing' as const;
    const release = this.acquirePin(key, 'scroll');
    try {
      for (let attempt = 0; attempt < 3; attempt++) {
        if (signal?.aborted || this.destroyed || generation !== this.generation)
          return 'cancelled' as const;
        if (this.virtual) this.engine.scrollToIndex(index, { align, behavior: 'instant' });
        else this.getNode(key)?.scrollIntoView?.({ block: 'nearest' });
        await tick();
      }
      return this.getNode(key, pane) ? ('scrolled' as const) : ('missing' as const);
    } finally {
      release();
    }
  }

  cancelScroll() {
    this.generation++;
    this.engine.cancelScroll();
  }

  setPrinting(printing: boolean) {
    if (printing === this.printing) return;
    if (printing) this.printAnchor = this.captureAnchor();
    this.printing = printing;
    this.engine.setOptions(this.options());
    this.engine._willUpdate();
    this.change();
    if (!printing && this.printAnchor) {
      const anchor = this.printAnchor;
      this.printAnchor = null;
      this.restoreAnchor(anchor);
    }
  }

  destroy() {
    if (this.destroyed) return;
    rememberVirtualAnchor(this.identity, this.captureAnchor(), this.audienceRevision);
    this.destroyed = true;
    this.generation++;
    this.engine.cancelScroll();
    this.listObserver?.disconnect();
    this.unmount?.();
    this.pins.clear();
    this.paneNodes.clear();
    this.engine.elementsCache.clear();
    this.engine.itemSizeCache.clear();
    this.rows = [];
    this.root = null;
    this.list = null;
  }
}

export type { RowPosition };

function focusableElements(node: HTMLElement) {
  const selector =
    'button:not([disabled]),a[href],input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';
  return [
    ...(node.matches(selector) ? [node] : []),
    ...node.querySelectorAll<HTMLElement>(selector),
  ].filter(
    (element) => !element.closest('[hidden],[inert],[aria-hidden="true"]') && element.tabIndex >= 0
  );
}
