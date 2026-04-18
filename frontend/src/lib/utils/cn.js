/**
 * Lightweight class-name composition helper for Svelte components.
 *
 * Accepts strings, arrays, objects ({class: bool}), and falsy values.
 * Filters falsy, flattens, dedupes whitespace. Does NOT resolve Tailwind
 * conflicts — when a consumer passes `class` overrides, place their classes
 * LAST in the cn() call so they win in the source order.
 *
 * @example
 *   cn('px-4 py-2', isActive && 'bg-blue-500', { 'opacity-50': disabled }, className)
 */
export function cn(...args) {
  return args
    .flat(Infinity)
    .flatMap((a) => {
      if (!a) return [];
      if (typeof a === 'string') return a;
      if (typeof a === 'object') {
        return Object.entries(a)
          .filter(([, v]) => v)
          .map(([k]) => k);
      }
      return [];
    })
    .join(' ')
    .split(/\s+/)
    .filter(Boolean)
    .join(' ');
}
