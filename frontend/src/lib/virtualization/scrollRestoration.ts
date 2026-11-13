import type { ScrollAnchor } from './types';

const snapshots = new Map<string, ScrollAnchor>();
let audience: string | null = null;
let audienceRevision = 0;

export function virtualAudienceRevision() {
  return audienceRevision;
}

export function activateVirtualAudience(next: string) {
  if (audience !== next) {
    snapshots.clear();
    audienceRevision++;
  }
  audience = next;
}

export function rememberVirtualAnchor(
  identity: string,
  anchor: ScrollAnchor | null,
  revision = audienceRevision
) {
  if (revision !== audienceRevision) return;
  snapshots.delete(identity);
  if (anchor) snapshots.set(identity, anchor);
  while (snapshots.size > 20) snapshots.delete(snapshots.keys().next().value as string);
}

export function recalledVirtualAnchor(identity: string, revision = audienceRevision) {
  if (revision !== audienceRevision) return;
  const anchor = snapshots.get(identity);
  if (anchor) rememberVirtualAnchor(identity, anchor);
  return anchor;
}

export function clearVirtualAnchors() {
  snapshots.clear();
  audience = null;
  audienceRevision++;
}
