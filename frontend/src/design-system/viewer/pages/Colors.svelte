<script>
  // Surface colors
  const surfaceColors = [
    { name: '--ds-surface', description: 'Default page background' },
    { name: '--ds-surface-hovered', description: 'Hovered surface' },
    { name: '--ds-surface-pressed', description: 'Pressed surface' },
    { name: '--ds-surface-raised', description: 'Elevated surfaces (cards)' },
    { name: '--ds-surface-raised-hovered', description: 'Hovered elevated surface' },
    { name: '--ds-surface-overlay', description: 'Modals, dropdowns' },
    { name: '--ds-surface-disabled', description: 'Disabled backgrounds' },
    { name: '--ds-surface-selected', description: 'Selected items' },
  ]

  // Text colors
  const textColors = [
    { name: '--ds-text', description: 'Primary text' },
    { name: '--ds-text-subtle', description: 'Secondary/helper text' },
    { name: '--ds-text-disabled', description: 'Disabled text' },
    { name: '--ds-text-inverse', description: 'Text on dark backgrounds' },
    { name: '--ds-text-link', description: 'Link text' },
  ]

  // Border colors
  const borderColors = [
    { name: '--ds-border', description: 'Default border' },
    { name: '--ds-border-bold', description: 'Emphasized border' },
    { name: '--ds-border-focused', description: 'Focus ring color' },
  ]

  // Interactive colors
  const interactiveColors = [
    { name: '--ds-interactive', description: 'Primary interactive (buttons)' },
    { name: '--ds-interactive-hovered', description: 'Hovered state' },
    { name: '--ds-interactive-pressed', description: 'Pressed state' },
    { name: '--ds-interactive-subtle', description: 'Subtle background' },
  ]

  // Feedback colors
  const feedbackColors = [
    { name: '--ds-danger', description: 'Danger/error', subtle: '--ds-danger-subtle' },
    { name: '--ds-success', description: 'Success', subtle: '--ds-success-subtle' },
    { name: '--ds-warning', description: 'Warning', subtle: '--ds-warning-subtle' },
    { name: '--ds-info', description: 'Information', subtle: '--ds-info-subtle' },
  ]

  // Accent colors
  const accentColors = ['blue', 'green', 'purple', 'teal', 'orange', 'pink', 'gray']

  // Primary palette
  const primaryShades = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900]
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Colors</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Design system color tokens for consistent styling across the application.
  </p>

  <!-- Primary Brand Palette -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
      Primary Brand Palette
    </h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Based on Windshift brand color #2874BB
    </p>
    <div class="grid grid-cols-5 md:grid-cols-10 gap-2">
      {#each primaryShades as shade}
        <div class="text-center">
          <div
            class="h-12 rounded-lg mb-1 border"
            style="background-color: var(--color-primary-{shade}); border-color: var(--ds-border);"
          ></div>
          <code class="text-xs" style="color: var(--ds-text-subtle);">{shade}</code>
        </div>
      {/each}
    </div>
  </section>

  <!-- Surface Colors -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
      Surface Colors
    </h2>
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      {#each surfaceColors as color}
        <div class="rounded-lg overflow-hidden border" style="border-color: var(--ds-border);">
          <div class="h-16" style="background-color: var({color.name});"></div>
          <div class="p-3" style="background-color: var(--ds-surface-raised);">
            <code class="text-xs block mb-1" style="color: var(--ds-text);">{color.name}</code>
            <span class="text-xs" style="color: var(--ds-text-subtle);">{color.description}</span>
          </div>
        </div>
      {/each}
    </div>
  </section>

  <!-- Text Colors -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
      Text Colors
    </h2>
    <div
      class="space-y-3 p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      {#each textColors as color}
        <div class="flex items-center justify-between">
          <span style="color: var({color.name});" class="font-medium">
            {color.description}
          </span>
          <code class="text-xs" style="color: var(--ds-text-subtle);">{color.name}</code>
        </div>
      {/each}
    </div>
  </section>

  <!-- Border Colors -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
      Border Colors
    </h2>
    <div class="grid grid-cols-3 gap-4">
      {#each borderColors as color}
        <div
          class="p-4 rounded-lg text-center"
          style="border: 2px solid var({color.name}); background-color: var(--ds-surface-raised);"
        >
          <code class="text-xs block mb-1" style="color: var(--ds-text);">{color.name}</code>
          <span class="text-xs" style="color: var(--ds-text-subtle);">{color.description}</span>
        </div>
      {/each}
    </div>
  </section>

  <!-- Interactive Colors -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
      Interactive Colors
    </h2>
    <div class="flex flex-wrap gap-3">
      {#each interactiveColors as color}
        <div
          class="px-4 py-3 rounded-lg text-center min-w-36"
          style="background-color: var({color.name}); color: white;"
        >
          <code class="text-xs block">{color.name}</code>
          <span class="text-xs opacity-80">{color.description}</span>
        </div>
      {/each}
    </div>
  </section>

  <!-- Feedback Colors -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
      Feedback Colors
    </h2>
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      {#each feedbackColors as color}
        <div class="rounded-lg overflow-hidden border" style="border-color: var(--ds-border);">
          <div class="flex">
            <div class="flex-1 h-12" style="background-color: var({color.name});"></div>
            <div class="flex-1 h-12" style="background-color: var({color.subtle});"></div>
          </div>
          <div class="p-3" style="background-color: var(--ds-surface-raised);">
            <span class="text-sm font-medium" style="color: var(--ds-text);">{color.description}</span>
          </div>
        </div>
      {/each}
    </div>
  </section>

  <!-- Accent Colors -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
      Accent Colors
    </h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      For badges, tags, and category indicators
    </p>
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      {#each accentColors as color}
        <div class="rounded-lg overflow-hidden border" style="border-color: var(--ds-border);">
          <div class="flex">
            <div class="flex-1 h-10" style="background-color: var(--ds-accent-{color});"></div>
            <div class="flex-1 h-10" style="background-color: var(--ds-accent-{color}-subtle);"></div>
          </div>
          <div class="p-2 text-center" style="background-color: var(--ds-surface-raised);">
            <code class="text-xs" style="color: var(--ds-text);">{color}</code>
          </div>
        </div>
      {/each}
    </div>
  </section>
</div>
