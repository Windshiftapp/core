const listeners = new Set();

export function onCollectionsChanged(listener) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export async function withCollectionInvalidation(write) {
  const result = await write;
  for (const listener of listeners) listener();
  return result;
}
