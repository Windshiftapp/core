<script>
  import ItemKey from '../../../lib/features/items/ItemKey.svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">ItemKey</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Standard display component for work item keys. Shows the workspace key with the item number in a consistent format.
    Styling is baked in: <code>text-xs font-mono</code> with <code>var(--ds-text-subtle)</code> color by default.
    Interactive variants (href or onClick) automatically add <code>hover:underline cursor-pointer</code>.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <div class="flex items-center gap-4">
      <ItemKey
        item={{ workspace_item_number: 42 }}
        workspace={{ key: 'PROJ' }}
      />
      <ItemKey
        item={{ workspace_item_number: 123 }}
        workspace={{ key: 'WIND' }}
      />
      <ItemKey
        item={{ workspace_item_number: 7 }}
        workspace={{ key: 'DEV' }}
      />
    </div>
  </section>

  <!-- Without Workspace -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Without Workspace</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      When no workspace is provided, falls back to "ITEM-" prefix.
    </p>
    <div class="flex items-center gap-4">
      <ItemKey item={{ workspace_item_number: 42 }} />
      <ItemKey item={{ workspace_item_number: 123 }} />
    </div>
  </section>

  <!-- As Link -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">As Link</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      When <code>href</code> is provided, renders as a clickable link with hover:underline.
    </p>
    <div class="flex items-center gap-4">
      <ItemKey
        item={{ workspace_item_number: 42 }}
        workspace={{ key: 'PROJ' }}
        href="#/workspace/PROJ/item/42"
      />
      <ItemKey
        item={{ workspace_item_number: 123 }}
        workspace={{ key: 'WIND' }}
        href="#/workspace/WIND/item/123"
      />
    </div>
  </section>

  <!-- With onClick (Button) -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With onClick (Button)</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      When <code>onClick</code> is provided without <code>href</code>, renders as a <code>&lt;button&gt;</code> with hover:underline.
    </p>
    <ItemKey
      item={{ workspace_item_number: 42 }}
      workspace={{ key: 'PROJ' }}
      onClick={() => alert('Clicked PROJ-42')}
    />
  </section>

  <!-- Custom Style Override -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Custom Style Override</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      Use the <code>style</code> prop to override the default color (e.g. for gradient backgrounds).
    </p>
    <div class="flex items-center gap-4 p-4 rounded-lg" style="background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);">
      <ItemKey
        item={{ workspace_item_number: 42 }}
        workspace={{ key: 'PROJ' }}
        style="color: rgba(255,255,255,0.7);"
      />
      <ItemKey
        item={{ workspace_item_number: 123 }}
        workspace={{ key: 'WIND' }}
        href="#"
        style="color: rgba(255,255,255,0.7);"
      />
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
        { id: 1, title: 'Implement user authentication', number: 42 },
        { id: 2, title: 'Fix navigation bug on mobile', number: 43 },
        { id: 3, title: 'Add dark mode support', number: 44 }
      ] as item}
        <div class="flex items-center gap-3 py-2 border-b last:border-b-0" style="border-color: var(--ds-border);">
          <ItemKey
            item={{ workspace_item_number: item.number }}
            workspace={{ key: 'PROJ' }}
          />
          <span class="text-sm" style="color: var(--ds-text);">{item.title}</span>
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
            <td class="p-2"><code>item</code></td>
            <td class="p-2">{`{ workspace_item_number: number }`}</td>
            <td class="p-2">-</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>workspace</code></td>
            <td class="p-2">{`{ key: string } | null`}</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>href</code></td>
            <td class="p-2">string</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onClick</code></td>
            <td class="p-2">function</td>
            <td class="p-2">null</td>
          </tr>
          <tr>
            <td class="p-2"><code>style</code></td>
            <td class="p-2">string</td>
            <td class="p-2">"color: var(--ds-text-subtle);"</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Format Examples -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Format Examples</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <p class="text-sm mb-2" style="color: var(--ds-text-subtle);">
        Output format depends on whether a workspace is provided:
      </p>
      <ul class="text-sm space-y-1" style="color: var(--ds-text);">
        <li>With workspace <code>{`{ key: 'PROJ' }`}</code>: <code>PROJ-42</code></li>
        <li>Without workspace: <code>ITEM-42</code></li>
      </ul>
    </div>
  </section>
</div>
