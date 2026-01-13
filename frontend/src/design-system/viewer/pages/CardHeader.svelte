<script>
  import CardHeader from '../../../lib/CardHeader.svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">CardHeader</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Simple card header component for displaying titles within card-style containers.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <CardHeader title="Card Title" />
      <div class="h-24 flex items-center justify-center border border-dashed rounded" style="border-color: var(--ds-border);">
        <span class="text-sm" style="color: var(--ds-text-subtle);">Card content</span>
      </div>
    </div>
  </section>

  <!-- Multiple Cards -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Multiple Cards</h2>
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <CardHeader title="Overview" />
        <div class="h-32 flex items-center justify-center border border-dashed rounded" style="border-color: var(--ds-border);">
          <span class="text-sm" style="color: var(--ds-text-subtle);">Overview content</span>
        </div>
      </div>
      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <CardHeader title="Statistics" />
        <div class="h-32 flex items-center justify-center border border-dashed rounded" style="border-color: var(--ds-border);">
          <span class="text-sm" style="color: var(--ds-text-subtle);">Statistics content</span>
        </div>
      </div>
      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <CardHeader title="Recent Activity" />
        <div class="h-32 flex items-center justify-center border border-dashed rounded" style="border-color: var(--ds-border);">
          <span class="text-sm" style="color: var(--ds-text-subtle);">Activity content</span>
        </div>
      </div>
      <div
        class="p-4 rounded-lg"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <CardHeader title="Quick Actions" />
        <div class="h-32 flex items-center justify-center border border-dashed rounded" style="border-color: var(--ds-border);">
          <span class="text-sm" style="color: var(--ds-text-subtle);">Actions content</span>
        </div>
      </div>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <CardHeader title="Team Members" />
      <div class="space-y-2">
        {#each ['Alice Johnson', 'Bob Smith', 'Carol Williams'] as name}
          <div class="flex items-center gap-3 py-2 border-b last:border-b-0" style="border-color: var(--ds-border);">
            <div class="w-8 h-8 rounded-full flex items-center justify-center text-white text-sm font-medium" style="background-color: var(--ds-interactive);">
              {name.split(' ').map(n => n[0]).join('')}
            </div>
            <span class="text-sm" style="color: var(--ds-text);">{name}</span>
          </div>
        {/each}
      </div>
    </div>
  </section>

  <!-- Props Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Props Reference</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Prop</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Type</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Default</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>title</code></td>
            <td class="p-2">string</td>
            <td class="p-2">-</td>
          </tr>
          <tr>
            <td class="p-2"><code>class</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Usage Note -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Usage Note</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        CardHeader is a minimal component designed for simple card titles. For headers that need
        subtitles or action buttons, use <code>SectionHeader</code> instead.
      </p>
    </div>
  </section>
</div>
