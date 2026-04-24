<script>
  import seagullPaths from './seagull-paths.json';

  // Shape IDs: 1,2,5,7 = large wingspan; 8,9,10,11 = mid; 3,4,6 = small/distant.
  // Traced silhouettes face left by default; flip=true mirrors to face right.
  const gulls = [
    { shape: 5, top: 10, left: 80, size: 80, opacity: 0.4, flip: false, rotate: -4 },
    { shape: 2, top: 16, left: 6, size: 64, opacity: 0.38, flip: true, rotate: 3 },
    { shape: 8, top: 46, left: 84, size: 60, opacity: 0.4, flip: false, rotate: -6 },
    { shape: 10, top: 36, left: 4, size: 48, opacity: 0.38, flip: true, rotate: 4 },
    { shape: 11, top: 6, left: 38, size: 30, opacity: 0.3, flip: true, rotate: -8 },
    { shape: 6, top: 28, left: 14, size: 26, opacity: 0.3, flip: true, rotate: 4 },
    { shape: 3, top: 4, left: 62, size: 20, opacity: 0.28, flip: false, rotate: 0 },
    { shape: 4, top: 42, left: 92, size: 18, opacity: 0.28, flip: false, rotate: 0 },
  ];

  function shapeFor(id) {
    return seagullPaths.find((p) => p.id === id);
  }
</script>

<div class="absolute inset-y-0 left-1/2 -translate-x-1/2 w-full max-w-6xl pointer-events-none z-[2] overflow-hidden" aria-hidden="true">
  {#each gulls as g}
    {@const s = shapeFor(g.shape)}
    {#if s}
      <svg
        class="absolute text-white"
        style="top: {g.top}%; left: {g.left}%; width: {g.size}px; height: auto; opacity: {g.opacity}; transform: rotate({g.rotate}deg) scaleX({g.flip ? -1 : 1});"
        viewBox={s.viewBox}
        aria-hidden="true"
      >
        <g transform={s.transform} fill="currentColor" stroke="none">
          <path d={s.d} />
        </g>
      </svg>
    {/if}
  {/each}
</div>
