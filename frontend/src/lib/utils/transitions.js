/**
 * Custom Svelte transition utilities for Windshift
 * Provides reusable, accessible animations with spring physics
 */

import { backOut, cubicOut, quintOut } from 'svelte/easing';

/**
 * Check if user prefers reduced motion
 * @returns {boolean}
 */
export function prefersReducedMotion() {
  if (typeof window === 'undefined') return false;
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/**
 * Get motion-safe duration (returns 0 if reduced motion preferred)
 * @param {number} duration
 * @returns {number}
 */
export function getMotionSafeDuration(duration) {
  return prefersReducedMotion() ? 0 : duration;
}

/**
 * Staggered fly transition for lists and grids
 * Creates a cascading entrance effect
 *
 * @example
 * {#each items as item, index}
 *   <div in:staggerFly={{ index, y: 20 }}>
 *     {item.name}
 *   </div>
 * {/each}
 */
export function staggerFly(
  _node,
  { index = 0, y = 20, x = 0, delay = 0, duration = 300, stagger = 50, easing = cubicOut }
) {
  if (prefersReducedMotion()) {
    return { duration: 0 };
  }

  return {
    delay: delay + index * stagger,
    duration,
    easing,
    css: (t) => `
      transform: translate(${(1 - t) * x}px, ${(1 - t) * y}px);
      opacity: ${t};
    `,
  };
}

/**
 * Spring-like scale transition with overshoot
 * Great for modals, cards, and interactive elements
 *
 * @example
 * <div in:springScale={{ start: 0.9 }}>
 *   Modal content
 * </div>
 */
export function springScale(_node, { delay = 0, duration = 400, start = 0.95, easing = backOut }) {
  if (prefersReducedMotion()) {
    return { duration: 0 };
  }

  return {
    delay,
    duration,
    easing,
    css: (t) => `
      transform: scale(${start + (1 - start) * t});
      opacity: ${t};
    `,
  };
}

/**
 * Fade with blur effect (glassmorphism entrance)
 * Creates a dreamy, soft entrance
 *
 * @example
 * <div in:fadeBlur={{ blur: 8 }}>
 *   Glass panel
 * </div>
 */
export function fadeBlur(_node, { delay = 0, duration = 300, blur = 4, easing = cubicOut }) {
  if (prefersReducedMotion()) {
    return { duration: 0 };
  }

  return {
    delay,
    duration,
    easing,
    css: (t) => `
      opacity: ${t};
      filter: blur(${(1 - t) * blur}px);
    `,
  };
}

/**
 * Card lift transition for hover states
 * Use with in:cardLift for hover-triggered effects
 *
 * @example
 * <div in:cardLift={{ y: 4 }}>
 *   Lifted card
 * </div>
 */
export function cardLift(_node, { delay = 0, duration = 200, y = 4, easing = cubicOut }) {
  if (prefersReducedMotion()) {
    return { duration: 0 };
  }

  return {
    delay,
    duration,
    easing,
    css: (t) => `
      transform: translateY(${(1 - t) * y * -1}px);
    `,
  };
}

/**
 * Smooth slide transition with fade
 * Good for sidebars and panels
 *
 * @example
 * <aside in:slideIn={{ direction: 'left' }}>
 */
export function slideIn(
  _node,
  {
    delay = 0,
    duration = 300,
    direction = 'right', // 'left', 'right', 'up', 'down'
    distance = 20,
    easing = quintOut,
  }
) {
  if (prefersReducedMotion()) {
    return { duration: 0 };
  }

  const transforms = {
    left: `translateX(-${distance}px)`,
    right: `translateX(${distance}px)`,
    up: `translateY(-${distance}px)`,
    down: `translateY(${distance}px)`,
  };

  const startTransform = transforms[direction] || transforms.right;

  return {
    delay,
    duration,
    easing,
    css: (t) => `
      transform: ${t === 1 ? 'none' : startTransform.replace(/\d+/, (d) => (1 - t) * parseInt(d, 10))};
      opacity: ${t};
    `,
  };
}

/**
 * Pop transition with scale and opacity
 * Good for tooltips, popovers, and quick reveals
 */
export function pop(_node, { delay = 0, duration = 200, start = 0.9, easing = backOut }) {
  if (prefersReducedMotion()) {
    return { duration: 0 };
  }

  return {
    delay,
    duration,
    easing,
    css: (t) => `
      transform: scale(${start + (1 - start) * t});
      opacity: ${t};
    `,
  };
}

/**
 * Animate a counter from one value to another
 * Use in component scripts for animated numbers
 *
 * @example
 * import { animateCounter } from './transitions.js';
 *
 * let displayValue = 0;
 * $: animateCounter(targetValue, (v) => displayValue = v);
 */
export function animateCounter(target, callback, duration = 1000) {
  if (prefersReducedMotion()) {
    callback(target);
    return;
  }

  const start = 0;
  const startTime = performance.now();

  function update(currentTime) {
    const elapsed = currentTime - startTime;
    const progress = Math.min(elapsed / duration, 1);

    // Ease-out curve for natural feel
    const easeOut = 1 - (1 - progress) ** 3;
    const value = Math.round(start + (target - start) * easeOut);

    callback(value);

    if (progress < 1) {
      requestAnimationFrame(update);
    }
  }

  requestAnimationFrame(update);
}

/**
 * Create a spring-based indicator position animation
 * Returns keyframe config for Web Animations API
 */
export function createIndicatorSlide(fromY, toY, duration = 200) {
  return {
    keyframes: [{ transform: `translateY(${fromY}px)` }, { transform: `translateY(${toY}px)` }],
    options: {
      duration: prefersReducedMotion() ? 0 : duration,
      easing: 'cubic-bezier(0.34, 1.56, 0.64, 1)', // Spring easing
      fill: 'forwards',
    },
  };
}
