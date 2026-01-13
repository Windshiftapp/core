<script>
  // Spacing scale
  const spacingScale = [
    { name: '0', value: '0px' },
    { name: '0-5', value: '2px' },
    { name: '1', value: '4px' },
    { name: '1-5', value: '6px' },
    { name: '2', value: '8px' },
    { name: '2-5', value: '10px' },
    { name: '3', value: '12px' },
    { name: '4', value: '16px' },
    { name: '5', value: '20px' },
    { name: '6', value: '24px' },
    { name: '8', value: '32px' },
    { name: '10', value: '40px' },
    { name: '12', value: '48px' },
    { name: '16', value: '64px' },
  ]

  // Border radius scale
  const radiusScale = [
    { name: 'none', value: '0' },
    { name: 'sm', value: '2px' },
    { name: 'md', value: '4px' },
    { name: 'lg', value: '6px' },
    { name: 'xl', value: '8px' },
    { name: '2xl', value: '12px' },
    { name: '3xl', value: '16px' },
    { name: 'full', value: '9999px' },
  ]

  // Shadow scale
  const shadowScale = [
    { name: 'sm', description: 'Subtle elevation' },
    { name: 'md', description: 'Default elevation' },
    { name: 'lg', description: 'High elevation' },
    { name: 'xl', description: 'Modal/dropdown' },
    { name: '2xl', description: 'Maximum elevation' },
  ]

  // Z-index scale
  const zIndexScale = [
    { name: 'base', value: '0', description: 'Base layer' },
    { name: 'dropdown', value: '100', description: 'Dropdown menus' },
    { name: 'sticky', value: '200', description: 'Sticky headers' },
    { name: 'fixed', value: '300', description: 'Fixed elements' },
    { name: 'modal-backdrop', value: '400', description: 'Modal backdrop' },
    { name: 'modal', value: '500', description: 'Modal content' },
    { name: 'popover', value: '600', description: 'Popovers' },
    { name: 'tooltip', value: '700', description: 'Tooltips' },
    { name: 'toast', value: '800', description: 'Toast notifications' },
  ]
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Spacing</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Spacing, radius, shadow, and z-index tokens for consistent layouts.
  </p>

  <!-- Spacing Scale -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-2" style="color: var(--ds-text);">Spacing Scale</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Based on a 4px grid (0.25rem base unit)
    </p>
    <div class="space-y-2">
      {#each spacingScale as space}
        <div
          class="flex items-center gap-4 p-2 rounded"
          style="background-color: var(--ds-surface-raised);"
        >
          <div class="w-16 shrink-0">
            <code class="text-xs" style="color: var(--ds-text);">--space-{space.name}</code>
          </div>
          <div class="flex-1 flex items-center">
            <div
              class="h-4"
              style="width: var(--space-{space.name}); background-color: var(--ds-interactive);"
            ></div>
          </div>
          <div class="w-12 text-right shrink-0">
            <span class="text-xs" style="color: var(--ds-text-subtle);">{space.value}</span>
          </div>
        </div>
      {/each}
    </div>
  </section>

  <!-- Border Radius -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Border Radius</h2>
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      {#each radiusScale as radius}
        <div
          class="p-4 text-center"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <div
            class="w-16 h-16 mx-auto mb-3"
            style="background-color: var(--ds-interactive); border-radius: var(--radius-{radius.name});"
          ></div>
          <code class="text-xs block" style="color: var(--ds-text);">--radius-{radius.name}</code>
          <span class="text-xs" style="color: var(--ds-text-subtle);">{radius.value}</span>
        </div>
      {/each}
    </div>
  </section>

  <!-- Shadows -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Shadows</h2>
    <div class="grid grid-cols-2 md:grid-cols-5 gap-6">
      {#each shadowScale as shadow}
        <div class="text-center">
          <div
            class="w-20 h-20 mx-auto mb-3 rounded-lg"
            style="background-color: var(--ds-surface-raised); box-shadow: var(--shadow-{shadow.name});"
          ></div>
          <code class="text-xs block" style="color: var(--ds-text);">--shadow-{shadow.name}</code>
          <span class="text-xs" style="color: var(--ds-text-subtle);">{shadow.description}</span>
        </div>
      {/each}
    </div>
  </section>

  <!-- Z-Index -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Z-Index Scale</h2>
    <div class="space-y-2">
      {#each zIndexScale as z}
        <div
          class="flex items-center gap-4 p-3 rounded"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <code class="text-xs w-40 shrink-0" style="color: var(--ds-text);">--z-{z.name}</code>
          <span class="flex-1 text-sm" style="color: var(--ds-text);">{z.description}</span>
          <span class="text-xs font-mono shrink-0" style="color: var(--ds-text-subtle);">
            {z.value}
          </span>
        </div>
      {/each}
    </div>
  </section>

  <!-- Spacing Usage Examples -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Usage Examples</h2>

    <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
      <!-- Padding Example -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Padding</h3>
        <div class="flex gap-3 flex-wrap">
          {#each ['2', '4', '6', '8'] as space}
            <div
              style="padding: var(--space-{space}); background-color: var(--ds-accent-blue-subtle); border: 1px dashed var(--ds-accent-blue);"
            >
              <div
                class="text-xs text-center whitespace-nowrap"
                style="background-color: var(--ds-surface-raised); padding: 4px;"
              >
                p-{space}
              </div>
            </div>
          {/each}
        </div>
      </div>

      <!-- Gap Example -->
      <div>
        <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text-subtle);">Gap</h3>
        <div class="flex gap-6">
          {#each ['1', '2', '4'] as space}
            <div>
              <div class="flex" style="gap: var(--space-{space});">
                {#each [1, 2, 3] as _}
                  <div
                    class="w-6 h-6 rounded"
                    style="background-color: var(--ds-interactive);"
                  ></div>
                {/each}
              </div>
              <code class="text-xs mt-2 block text-center" style="color: var(--ds-text-subtle);">
                gap-{space}
              </code>
            </div>
          {/each}
        </div>
      </div>
    </div>
  </section>
</div>
