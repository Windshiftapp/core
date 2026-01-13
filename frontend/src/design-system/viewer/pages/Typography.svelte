<script>
  // Font sizes
  const fontSizes = [
    { name: '--text-xs', size: '0.75rem', px: '12px' },
    { name: '--text-sm', size: '0.875rem', px: '14px' },
    { name: '--text-base', size: '1rem', px: '16px' },
    { name: '--text-lg', size: '1.125rem', px: '18px' },
    { name: '--text-xl', size: '1.25rem', px: '20px' },
    { name: '--text-2xl', size: '1.5rem', px: '24px' },
    { name: '--text-3xl', size: '1.875rem', px: '30px' },
    { name: '--text-4xl', size: '2.25rem', px: '36px' },
  ]

  // Font weights
  const fontWeights = [
    { name: '--font-normal', weight: '400', label: 'Normal' },
    { name: '--font-medium', weight: '500', label: 'Medium' },
    { name: '--font-semibold', weight: '600', label: 'Semibold' },
    { name: '--font-bold', weight: '700', label: 'Bold' },
  ]

  // Line heights
  const lineHeights = [
    { name: '--leading-none', value: '1', description: 'None' },
    { name: '--leading-tight', value: '1.25', description: 'Tight' },
    { name: '--leading-snug', value: '1.375', description: 'Snug' },
    { name: '--leading-normal', value: '1.5', description: 'Normal' },
    { name: '--leading-relaxed', value: '1.625', description: 'Relaxed' },
    { name: '--leading-loose', value: '2', description: 'Loose' },
  ]
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Typography</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Typography tokens for consistent text styling.
  </p>

  <!-- Font Family -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Font Family</h2>
    <div
      class="p-6 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <p class="text-lg mb-2" style="color: var(--ds-text);">
        Geist Sans - Primary Font
      </p>
      <code class="text-sm" style="color: var(--ds-text-subtle);">
        'Geist', -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif
      </code>
      <div class="mt-4 pt-4" style="border-top: 1px solid var(--ds-border);">
        <p style="color: var(--ds-text);">
          The quick brown fox jumps over the lazy dog. 0123456789
        </p>
      </div>
    </div>
  </section>

  <!-- Font Sizes -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Font Sizes</h2>
    <div class="space-y-3">
      {#each fontSizes as size}
        <div
          class="flex items-baseline gap-4 p-4 rounded-lg"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <span class="flex-1" style="font-size: var({size.name}); color: var(--ds-text);">
            The quick brown fox
          </span>
          <code class="text-xs shrink-0" style="color: var(--ds-text-subtle);">
            {size.name}
          </code>
          <span class="text-xs shrink-0 w-12 text-right" style="color: var(--ds-text-subtle);">
            {size.px}
          </span>
        </div>
      {/each}
    </div>
  </section>

  <!-- Font Weights -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Font Weights</h2>
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      {#each fontWeights as weight}
        <div
          class="p-4 rounded-lg text-center"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <p class="text-lg mb-2" style="font-weight: var({weight.name}); color: var(--ds-text);">
            {weight.label}
          </p>
          <code class="text-xs" style="color: var(--ds-text-subtle);">
            {weight.name}
          </code>
          <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
            {weight.weight}
          </p>
        </div>
      {/each}
    </div>
  </section>

  <!-- Line Heights -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Line Heights</h2>
    <div class="grid grid-cols-2 md:grid-cols-3 gap-4">
      {#each lineHeights as leading}
        <div
          class="p-4 rounded-lg"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <p
            class="text-sm mb-3"
            style="line-height: var({leading.name}); color: var(--ds-text);"
          >
            This text demonstrates the {leading.description.toLowerCase()} line height. Notice the spacing between lines.
          </p>
          <code class="text-xs" style="color: var(--ds-text-subtle);">
            {leading.name}: {leading.value}
          </code>
        </div>
      {/each}
    </div>
  </section>

  <!-- Type Hierarchy Example -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Type Hierarchy</h2>
    <div
      class="p-6 rounded-lg space-y-4"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <h1 style="font-size: var(--text-4xl); font-weight: var(--font-bold); line-height: var(--leading-tight); color: var(--ds-text);">
        Page Title (4xl, bold)
      </h1>
      <h2 style="font-size: var(--text-2xl); font-weight: var(--font-semibold); line-height: var(--leading-snug); color: var(--ds-text);">
        Section Heading (2xl, semibold)
      </h2>
      <h3 style="font-size: var(--text-xl); font-weight: var(--font-semibold); line-height: var(--leading-snug); color: var(--ds-text);">
        Subsection Heading (xl, semibold)
      </h3>
      <p style="font-size: var(--text-base); line-height: var(--leading-normal); color: var(--ds-text);">
        Body text uses the base size with normal line height. This is the default for most content.
      </p>
      <p style="font-size: var(--text-sm); color: var(--ds-text-subtle); line-height: var(--leading-normal);">
        Helper text uses a smaller size and subtle color. Use for secondary information.
      </p>
      <p style="font-size: var(--text-xs); color: var(--ds-text-disabled); line-height: var(--leading-normal);">
        Caption text is the smallest, for labels or less important info.
      </p>
    </div>
  </section>
</div>
