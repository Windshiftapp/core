<script>
  // Windshift AI mark: a swirling orbit around a four-point sparkle.
  // Lucide-compatible geometry (24x24 grid, 2px stroke, currentColor).
  let {
    size = 24,
    strokeWidth = 2,
    class: className = '',
  } = $props();
</script>

<svg
  xmlns="http://www.w3.org/2000/svg"
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width={strokeWidth}
  stroke-linecap="round"
  stroke-linejoin="round"
  class={className}
  role="img"
  aria-label="Windshift AI"
>
  <path d="M3.63 9.28A8.8 8.8 0 0 1 13.98 3.43" />
  <path d="M20.37 14.72A8.8 8.8 0 0 1 10.02 20.57" />
  <path
    d="M12 7.5Q13.08 10.92 16.5 12Q13.08 13.08 12 16.5Q10.92 13.08 7.5 12Q10.92 10.92 12 7.5Z"
  />
</svg>
