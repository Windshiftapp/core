<script>
  import Input from '../../../lib/Input.svelte'

  let textValue = $state('')
  let emailValue = $state('')
  let numberValue = $state('')
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Input</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Text input component with size variants and various input types.
  </p>

  <!-- Basic Input -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Input</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">
          Text Input
        </label>
        <Input placeholder="Enter text..." bind:value={textValue} />
        <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
          Value: {textValue || '(empty)'}
        </p>
      </div>
    </div>
  </section>

  <!-- Input Types -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Input Types</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Email</label>
        <Input type="email" placeholder="user@example.com" bind:value={emailValue} />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Password</label>
        <Input type="password" placeholder="Enter password..." />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Number</label>
        <Input type="number" placeholder="0" min={0} max={100} bind:value={numberValue} />
      </div>
    </div>
  </section>

  <!-- Sizes -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sizes</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Small</label>
        <Input size="small" placeholder="Small input" />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Medium (default)</label>
        <Input size="medium" placeholder="Medium input" />
      </div>
    </div>
  </section>

  <!-- States -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">States</h2>
    <div class="max-w-md space-y-4">
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Normal</label>
        <Input placeholder="Normal input" />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Disabled</label>
        <Input placeholder="Disabled input" disabled />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">With Value</label>
        <Input value="Pre-filled value" />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1" style="color: var(--ds-text);">Required</label>
        <Input placeholder="Required field" required />
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
            <td class="p-2"><code>type</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'text'</td>
          </tr>
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
            <td class="p-2"><code>size</code></td>
            <td class="p-2">'small' | 'medium'</td>
            <td class="p-2">'medium'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>required</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr>
            <td class="p-2"><code>min/max/step</code></td>
            <td class="p-2">number</td>
            <td class="p-2">undefined</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
