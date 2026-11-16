import { twMerge } from 'tailwind-merge';

/**
 * Lightweight class-name composition helper for Svelte components.
 *
 * Accepts strings, arrays, objects ({class: bool}), and falsy values.
 * Resolves Tailwind conflicts within the same modifier scope. Later ordinary
 * utilities win; important utilities retain CSS precedence. Custom classes and
 * distinct responsive/state variants remain independent.
 *
 * @example
 *   cn('px-4 py-2', isActive && 'bg-ds-interactive', { 'opacity-50': disabled }, className)
 */
export function cn(...args) {
  return twMerge(
    args
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
      .join(' ')
  );
}
