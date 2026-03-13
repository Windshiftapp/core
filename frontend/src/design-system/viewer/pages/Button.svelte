<script>
  import Button from '../../../lib/components/Button.svelte'
  import { Settings, ArrowRight, Plus, Download, Trash2 } from 'lucide-svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Button</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Button component with multiple variants, sizes, and states.
  </p>

  <!-- Variants -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Variants</h2>
    <div class="flex flex-wrap gap-3">
      <Button variant="primary">Primary</Button>
      <Button variant="default">Default</Button>
      <Button variant="secondary">Secondary</Button>
      <Button variant="danger">Danger</Button>
      <Button variant="selected">Selected</Button>
      <Button variant="ghost">Ghost</Button>
      <Button variant="link">Link</Button>
    </div>
  </section>

  <!-- Sizes -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sizes</h2>
    <div class="flex items-center gap-3">
      <Button variant="primary" size="small">Small</Button>
      <Button variant="primary" size="medium">Medium</Button>
      <Button variant="primary" size="large">Large</Button>
    </div>
  </section>

  <!-- With Icons -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Icons</h2>
    <div class="flex flex-wrap gap-3">
      <Button variant="primary" icon={Plus}>Add Item</Button>
      <Button variant="default" icon={Settings}>Settings</Button>
      <Button variant="primary" icon={ArrowRight} iconPosition="right">Continue</Button>
      <Button variant="secondary" icon={Download}>Download</Button>
      <Button variant="danger" icon={Trash2}>Delete</Button>
    </div>
  </section>

  <!-- States -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">States</h2>
    <div class="space-y-4">
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Normal vs Disabled</h3>
        <div class="flex gap-3">
          <Button variant="primary">Normal</Button>
          <Button variant="primary" disabled>Disabled</Button>
        </div>
      </div>
      <div>
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">Loading State</h3>
        <div class="flex gap-3">
          <Button variant="primary" loading>Loading...</Button>
          <Button variant="default" loading>Please wait</Button>
        </div>
      </div>
    </div>
  </section>

  <!-- With Keyboard Hints -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Keyboard Hints</h2>
    <div class="flex flex-wrap gap-3">
      <Button variant="primary" keyboardHint="C">Create</Button>
      <Button variant="default" keyboardHint="⌘K">Search</Button>
      <Button variant="secondary" keyboardHint="Esc">Cancel</Button>
    </div>
  </section>

  <!-- Full Width -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Full Width</h2>
    <div class="max-w-md space-y-3">
      <Button variant="primary" fullWidth>Full Width Primary</Button>
      <Button variant="default" fullWidth>Full Width Default</Button>
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
            <td class="p-2"><code>variant</code></td>
            <td class="p-2">'primary' | 'default' | 'secondary' | 'danger' | 'selected' | 'ghost' | 'link'</td>
            <td class="p-2">'default'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>size</code></td>
            <td class="p-2">'small' | 'medium' | 'large'</td>
            <td class="p-2">'medium'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>loading</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>icon</code></td>
            <td class="p-2">Lucide icon component</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>iconPosition</code></td>
            <td class="p-2">'left' | 'right'</td>
            <td class="p-2">'left'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>fullWidth</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr>
            <td class="p-2"><code>keyboardHint</code></td>
            <td class="p-2">string</td>
            <td class="p-2">null</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
