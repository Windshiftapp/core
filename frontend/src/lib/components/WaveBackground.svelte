<script>
  let { className = '' } = $props();
</script>

<div
  class="wave-bg absolute left-0 right-0 pointer-events-none z-[1] {className}"
  style="top: 12%; bottom: 0;"
  aria-hidden="true"
>
  <svg
    class="absolute inset-0 w-full h-full"
    viewBox="0 0 1440 600"
    preserveAspectRatio="none"
  >
    <path
      fill="rgba(220, 240, 255, 0.06)"
      d="M0,210 C220,160 440,250 660,215 C880,180 1100,135 1260,175 C1360,200 1410,215 1440,210 L1440,600 L0,600 Z"
    />
    <path
      fill="rgba(220, 240, 255, 0.07)"
      d="M0,340 C200,295 400,370 600,340 C800,310 1000,265 1200,295 C1320,315 1400,335 1440,330 L1440,600 L0,600 Z"
    />
    <path
      fill="rgba(220, 240, 255, 0.08)"
      d="M0,450 C180,415 360,475 540,450 C720,425 900,385 1080,410 C1240,432 1360,455 1440,445 L1440,600 L0,600 Z"
    />
  </svg>
</div>

<style>
  .wave-bg {
    -webkit-mask-image: radial-gradient(
      ellipse 75% 65% at 50% 45%,
      rgba(0, 0, 0, 1) 0%,
      rgba(0, 0, 0, 0.9) 35%,
      rgba(0, 0, 0, 0.45) 70%,
      rgba(0, 0, 0, 0) 100%
    );
    mask-image: radial-gradient(
      ellipse 75% 65% at 50% 45%,
      rgba(0, 0, 0, 1) 0%,
      rgba(0, 0, 0, 0.9) 35%,
      rgba(0, 0, 0, 0.45) 70%,
      rgba(0, 0, 0, 0) 100%
    );
  }
</style>
