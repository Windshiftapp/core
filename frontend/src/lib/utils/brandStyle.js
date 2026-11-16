import { darkenColor, getTextColorForBackground } from './colorUtils.js';

// Custom fills use independent opaque foregrounds for every interaction state.
export function brandStyle(color) {
  if (/^#[\da-f]{3}$/i.test(color ?? ''))
    color = `#${[...color.slice(1)].map((char) => char + char).join('')}`;
  if (!/^#[\da-f]{6}$/i.test(color ?? '')) return undefined;
  return [
    `--ds-brand-color: ${color}`,
    `--ds-text-on-interactive: ${getTextColorForBackground(color)}`,
    `--ds-text-on-interactive-hovered: ${getTextColorForBackground(darkenColor(color, 0.12))}`,
    `--ds-text-on-interactive-pressed: ${getTextColorForBackground(darkenColor(color, 0.18))}`,
  ].join('; ');
}
