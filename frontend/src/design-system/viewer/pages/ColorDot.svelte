<script>
  import ColorDot from '../../../lib/ColorDot.svelte'

  const colors = [
    { name: 'Gray', hex: '#9CA3AF' },
    { name: 'Red', hex: '#EF4444' },
    { name: 'Orange', hex: '#F97316' },
    { name: 'Yellow', hex: '#EAB308' },
    { name: 'Green', hex: '#22C55E' },
    { name: 'Blue', hex: '#3B82F6' },
    { name: 'Purple', hex: '#8B5CF6' },
    { name: 'Pink', hex: '#EC4899' }
  ];
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">ColorDot</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Small colored dot indicator for status, categories, or visual markers.
  </p>

  <!-- Sizes -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sizes</h2>
    <div class="flex items-center gap-8">
      <div class="text-center">
        <div class="flex items-center justify-center h-8">
          <ColorDot color="#3B82F6" size="xs" />
        </div>
        <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">xs (6px)</p>
      </div>
      <div class="text-center">
        <div class="flex items-center justify-center h-8">
          <ColorDot color="#3B82F6" size="sm" />
        </div>
        <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">sm (8px)</p>
      </div>
      <div class="text-center">
        <div class="flex items-center justify-center h-8">
          <ColorDot color="#3B82F6" size="md" />
        </div>
        <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">md (12px)</p>
      </div>
    </div>
  </section>

  <!-- Colors -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Colors</h2>
    <div class="flex flex-wrap gap-4">
      {#each colors as color}
        <div class="flex items-center gap-2">
          <ColorDot color={color.hex} size="sm" />
          <span class="text-sm" style="color: var(--ds-text);">{color.name}</span>
        </div>
      {/each}
    </div>
  </section>

  <!-- Common Use Cases -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Common Use Cases</h2>
    <div class="space-y-6">
      <!-- Status indicator -->
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Status Indicators</h3>
        <div class="flex flex-wrap gap-4">
          <div class="flex items-center gap-2">
            <ColorDot color="#22C55E" size="sm" />
            <span class="text-sm" style="color: var(--ds-text);">Online</span>
          </div>
          <div class="flex items-center gap-2">
            <ColorDot color="#EAB308" size="sm" />
            <span class="text-sm" style="color: var(--ds-text);">Away</span>
          </div>
          <div class="flex items-center gap-2">
            <ColorDot color="#EF4444" size="sm" />
            <span class="text-sm" style="color: var(--ds-text);">Busy</span>
          </div>
          <div class="flex items-center gap-2">
            <ColorDot color="#9CA3AF" size="sm" />
            <span class="text-sm" style="color: var(--ds-text);">Offline</span>
          </div>
        </div>
      </div>

      <!-- Category markers -->
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Category Markers</h3>
        <div class="flex flex-wrap gap-4">
          <div class="flex items-center gap-2">
            <ColorDot color="#8B5CF6" size="sm" />
            <span class="text-sm" style="color: var(--ds-text);">Design</span>
          </div>
          <div class="flex items-center gap-2">
            <ColorDot color="#3B82F6" size="sm" />
            <span class="text-sm" style="color: var(--ds-text);">Development</span>
          </div>
          <div class="flex items-center gap-2">
            <ColorDot color="#22C55E" size="sm" />
            <span class="text-sm" style="color: var(--ds-text);">Marketing</span>
          </div>
          <div class="flex items-center gap-2">
            <ColorDot color="#F97316" size="sm" />
            <span class="text-sm" style="color: var(--ds-text);">Sales</span>
          </div>
        </div>
      </div>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Context</h2>
    <div
      class="p-4 rounded-lg space-y-3"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      {#each [
        { name: 'John Smith', status: 'online', statusColor: '#22C55E', role: 'Developer' },
        { name: 'Jane Doe', status: 'away', statusColor: '#EAB308', role: 'Designer' },
        { name: 'Bob Wilson', status: 'offline', statusColor: '#9CA3AF', role: 'Manager' }
      ] as user}
        <div class="flex items-center gap-3 py-2 border-b last:border-b-0" style="border-color: var(--ds-border);">
          <div class="w-8 h-8 rounded-full flex items-center justify-center text-white text-sm font-medium" style="background-color: var(--ds-interactive);">
            {user.name.split(' ').map(n => n[0]).join('')}
          </div>
          <div class="flex-1">
            <div class="flex items-center gap-2">
              <span class="text-sm font-medium" style="color: var(--ds-text);">{user.name}</span>
              <ColorDot color={user.statusColor} size="xs" />
            </div>
            <p class="text-xs" style="color: var(--ds-text-subtle);">{user.role}</p>
          </div>
        </div>
      {/each}
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
            <td class="p-2"><code>color</code></td>
            <td class="p-2">string (hex color)</td>
            <td class="p-2">'#9CA3AF'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>size</code></td>
            <td class="p-2">'xs' | 'sm' | 'md'</td>
            <td class="p-2">'sm'</td>
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

  <!-- Size Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Size Reference</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Size</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Dimensions</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Use Case</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>xs</code></td>
            <td class="p-2">6x6px (w-1.5 h-1.5)</td>
            <td class="p-2">Inline status next to names</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>sm</code></td>
            <td class="p-2">8x8px (w-2 h-2)</td>
            <td class="p-2">List items, category indicators</td>
          </tr>
          <tr>
            <td class="p-2"><code>md</code></td>
            <td class="p-2">12x12px (w-3 h-3)</td>
            <td class="p-2">Larger visual emphasis</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
