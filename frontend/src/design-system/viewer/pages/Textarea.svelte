<script>
  import Textarea from '../../../lib/components/Textarea.svelte'

  let basicValue = $state('')
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Textarea</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Multi-line text input component with size variants and row configuration.
  </p>

  <!-- Basic Textarea -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Textarea</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">
          Description
        </label>
        <Textarea placeholder="Enter a description..." bind:value={basicValue} />
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
          {basicValue.length} characters
        </p>
      </div>
    </div>
  </section>

  <!-- Sizes -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sizes</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Small</label>
        <Textarea size="small" placeholder="Small textarea" />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Medium (default)</label>
        <Textarea size="medium" placeholder="Medium textarea" />
      </div>
    </div>
  </section>

  <!-- Row Variants -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Row Variants</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">2 Rows</label>
        <Textarea rows={2} placeholder="Short textarea" />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">3 Rows (default)</label>
        <Textarea rows={3} placeholder="Default height" />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">6 Rows</label>
        <Textarea rows={6} placeholder="Tall textarea for longer content" />
      </div>
    </div>
  </section>

  <!-- States -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">States</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Normal</label>
        <Textarea placeholder="Normal textarea" />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Disabled</label>
        <Textarea placeholder="Disabled textarea" disabled />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">With Value</label>
        <Textarea value="This textarea has pre-filled content that can be edited by the user." />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Required</label>
        <Textarea placeholder="Required field" required />
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
            <td class="p-2"><code>value</code></td>
            <td class="p-2">string (bindable)</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>placeholder</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>rows</code></td>
            <td class="p-2">number</td>
            <td class="p-2">3</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>size</code></td>
            <td class="p-2">'small' | 'medium'</td>
            <td class="p-2">'medium'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr>
            <td class="p-2"><code>required</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
