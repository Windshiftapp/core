import { links as linksApi } from '../api/configuration.js';
import { BACKGROUND_READ } from '../api/readScheduler.js';

const BATCH_SIZE = 200;
const EMPTY_GROUP = Object.freeze({ outgoing: [], incoming: [] });

function primaryField(fieldId, fieldOptions) {
  try {
    const options = JSON.parse(fieldOptions || '{}');
    const mirrorOf = Number(options.mirror_of_field_id);
    return {
      id: Number.isInteger(mirrorOf) && mirrorOf > 0 ? mirrorOf : Number(fieldId),
      mirror: Number.isInteger(mirrorOf) && mirrorOf > 0,
    };
  } catch {
    return { id: Number(fieldId), mirror: false };
  }
}

/** Batched, permission-filtered custom-field links for visible list rows. */
export class CollectionFieldLinksStore {
  byItem = $state({});

  /** @type {Map<number, Promise<void>>} */
  #pending = new Map();
  #generation = 0;

  getFieldLinks(itemId, fieldId, fieldOptions = '{}') {
    const id = Number(itemId);
    const group = this.byItem[id] ?? EMPTY_GROUP;
    const field = primaryField(fieldId, fieldOptions);
    const links = [...group.outgoing, ...group.incoming];

    return links.filter((link) => {
      if (Number(link.custom_field_id) !== field.id) return false;
      if (field.mirror) return link.target_type === 'item' && Number(link.target_id) === id;
      return link.source_type === 'item' && Number(link.source_id) === id;
    });
  }

  async loadForItems(itemIds) {
    const ids = [...new Set((itemIds || []).map(Number).filter((id) => id > 0))];
    const waits = ids.map((id) => this.#pending.get(id)).filter(Boolean);
    const missing = ids.filter((id) => !this.byItem[id] && !this.#pending.has(id));

    const chunks = [];
    for (let index = 0; index < missing.length; index += BATCH_SIZE) {
      chunks.push(missing.slice(index, index + BATCH_SIZE));
    }

    await Promise.all([...waits, ...chunks.map((chunk) => this.#loadChunk(chunk))]);
  }

  async refreshForItems(itemIds) {
    const ids = [...new Set((itemIds || []).map(Number).filter((id) => id > 0))];
    for (const id of ids) this.invalidate(id);
    await this.loadForItems(ids);
  }

  invalidate(itemId) {
    const id = Number(itemId);
    if (!id) return;
    const next = { ...this.byItem };
    delete next[id];
    this.byItem = next;
  }

  reset() {
    this.#generation += 1;
    this.#pending.clear();
    this.byItem = {};
  }

  #loadChunk(chunk) {
    const generation = this.#generation;
    const promise = linksApi
      .getForItems(chunk, { includeCustomFields: true, ...BACKGROUND_READ })
      .then((groups) => {
        if (generation !== this.#generation) return;
        const updates = {};
        for (const itemId of chunk) {
          const group = groups?.[itemId] ?? EMPTY_GROUP;
          updates[itemId] = {
            outgoing: Array.isArray(group.outgoing) ? group.outgoing : [],
            incoming: Array.isArray(group.incoming) ? group.incoming : [],
          };
        }
        this.byItem = { ...this.byItem, ...updates };
      })
      .catch((error) => {
        if (generation !== this.#generation) return;
        console.error('CollectionFieldLinksStore: failed to load links', error);
        const updates = Object.fromEntries(chunk.map((itemId) => [itemId, EMPTY_GROUP]));
        this.byItem = { ...this.byItem, ...updates };
      })
      .finally(() => {
        if (generation !== this.#generation) return;
        for (const itemId of chunk) this.#pending.delete(itemId);
      });

    for (const itemId of chunk) this.#pending.set(itemId, promise);
    return promise;
  }
}

export const collectionFieldLinks = new CollectionFieldLinksStore();
