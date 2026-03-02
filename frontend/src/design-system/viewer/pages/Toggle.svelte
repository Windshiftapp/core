<script>
  import Toggle from '$lib/components/Toggle.svelte';

  let toggle1 = $state(false);
  let toggle2 = $state(true);
  let toggle3 = $state(false);
  let sizeSmall = $state(false);
  let sizeMedium = $state(true);
  let sizeLarge = $state(false);
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Toggle</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Toggle switch component for boolean settings. Uses design system colors for dark mode support.
  </p>

  <!-- Basic Toggle -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Toggle</h2>
    <div class="flex flex-col gap-4">
      <div class="flex items-center gap-3">
        <Toggle bind:checked={toggle1} />
        <span class="text-sm" style="color: var(--ds-text);">
          Toggle is {toggle1 ? 'On' : 'Off'}
        </span>
      </div>
    </div>
  </section>

  <!-- Toggle States -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">States</h2>
    <div class="flex flex-col gap-4">
      <div class="flex items-center gap-3">
        <Toggle checked={false} disabled />
        <span class="text-sm" style="color: var(--ds-text-subtle);">Disabled Off</span>
      </div>
      <div class="flex items-center gap-3">
        <Toggle checked={true} disabled />
        <span class="text-sm" style="color: var(--ds-text-subtle);">Disabled On</span>
      </div>
    </div>
  </section>

  <!-- Sizes -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sizes</h2>
    <div class="flex flex-col gap-4">
      <div class="flex items-center gap-3">
        <Toggle bind:checked={sizeSmall} size="small" />
        <span class="text-sm" style="color: var(--ds-text);">Small</span>
      </div>
      <div class="flex items-center gap-3">
        <Toggle bind:checked={sizeMedium} size="medium" />
        <span class="text-sm" style="color: var(--ds-text);">Medium (default)</span>
      </div>
      <div class="flex items-center gap-3">
        <Toggle bind:checked={sizeLarge} size="large" />
        <span class="text-sm" style="color: var(--ds-text);">Large</span>
      </div>
    </div>
  </section>

  <!-- With Labels -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Labels</h2>
    <div class="flex flex-col gap-4">
      <Toggle bind:checked={toggle2} label="Enable notifications" labelPosition="left" />
      <Toggle bind:checked={toggle3} label="Dark mode" />
    </div>
  </section>

  <!-- Usage Guidelines -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Usage Guidelines</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <ul class="space-y-2 text-sm" style="color: var(--ds-text-subtle);">
        <li><strong style="color: var(--ds-text);">Off state:</strong> Uses <code>var(--ds-background-neutral)</code></li>
        <li><strong style="color: var(--ds-text);">On state:</strong> Uses <code>var(--ds-background-brand-bold)</code></li>
        <li><strong style="color: var(--ds-text);">Accessibility:</strong> Includes <code>role="switch"</code> and <code>aria-checked</code></li>
        <li><strong style="color: var(--ds-text);">Sizes:</strong> small, medium (default), large</li>
      </ul>
    </div>
  </section>

  <!-- Code Example -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Code Example</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <pre class="text-sm" style="color: var(--ds-text-subtle);"><code>{`<script>
  import Toggle from '$lib/components/Toggle.svelte';
  let enabled = $state(false);
</script>

<!-- Basic -->
<Toggle bind:checked={enabled} />

<!-- With label -->
<Toggle bind:checked={enabled} label="Enable feature" />

<!-- Disabled -->
<Toggle checked={true} disabled />

<!-- Size variants -->
<Toggle size="small" />
<Toggle size="medium" />
<Toggle size="large" />

<!-- With change handler -->
<Toggle bind:checked={enabled} onchange={() => {}} />`}</code></pre>
    </div>
  </section>
</div>
