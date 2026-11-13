export type RowKey = string;

export interface VirtualRow<T = unknown> {
  key: RowKey;
  value: T;
  estimatedSize: number;
  focusable?: boolean;
}

export interface RowPosition {
  key: string;
  index: number;
  start: number;
  end: number;
  size: number;
}

export type RenderToken =
  | { kind: 'row'; key: string; row: RowPosition }
  | { kind: 'gap'; key: string; size: number };

export interface ScrollAnchor {
  key: string;
  offset: number;
  index: number;
}

export type ScrollRoot = HTMLElement | Window;
export const VIRTUAL_SCROLL_CONTEXT = 'windshift:virtual-scroll-root';
export const VIRTUAL_ROW_CONTEXT = 'windshift:virtual-row';

export interface RowRetention {
  acquire(reason: string): () => void;
}
