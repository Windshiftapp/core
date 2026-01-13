<script>
  import Label from '../../../lib/Label.svelte'
  import Input from '../../../lib/Input.svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Label</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Form label component for labeling inputs with support for required indicators, sizes, and color variants.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <div class="space-y-4">
      <div>
        <Label for="basic-input" class="mb-2">Username</Label>
        <Input id="basic-input" placeholder="Enter username" />
      </div>
      <div>
        <Label for="required-input" required class="mb-2">Email Address</Label>
        <Input id="required-input" type="email" placeholder="you@example.com" />
      </div>
    </div>
  </section>

  <!-- Color Variants -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Color Variants</h2>
    <div class="flex gap-8">
      <div>
        <Label color="subtle" class="mb-2">Subtle (default)</Label>
        <p class="text-xs" style="color: var(--ds-text-subtle);">Uses --ds-text-subtle</p>
      </div>
      <div>
        <Label color="default" class="mb-2">Default</Label>
        <p class="text-xs" style="color: var(--ds-text-subtle);">Uses --ds-text</p>
      </div>
    </div>
  </section>

  <!-- Sizes -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Sizes</h2>
    <div class="flex gap-8 items-end">
      <div>
        <Label size="xs">Extra Small (xs)</Label>
      </div>
      <div>
        <Label size="sm">Small (sm) - default</Label>
      </div>
      <div>
        <Label size="md">Medium (md)</Label>
      </div>
    </div>
  </section>

  <!-- With Required Indicator -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Required Indicator</h2>
    <div class="flex gap-8">
      <div>
        <Label>Optional Field</Label>
      </div>
      <div>
        <Label required>Required Field</Label>
      </div>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Form Example</h2>
    <div
      class="p-6 rounded-lg max-w-md"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <div class="space-y-4">
        <div>
          <Label for="form-name" required class="mb-2">Full Name</Label>
          <Input id="form-name" placeholder="John Doe" />
        </div>
        <div>
          <Label for="form-email" required class="mb-2">Email</Label>
          <Input id="form-email" type="email" placeholder="john@example.com" />
        </div>
        <div>
          <Label for="form-phone" class="mb-2">Phone (Optional)</Label>
          <Input id="form-phone" type="tel" placeholder="+1 555 123 4567" />
        </div>
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
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Description</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>for</code></td>
            <td class="p-2">string | null</td>
            <td class="p-2">null</td>
            <td class="p-2">ID of associated input element</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>required</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
            <td class="p-2">Show red asterisk indicator</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>size</code></td>
            <td class="p-2">'xs' | 'sm' | 'md'</td>
            <td class="p-2">'sm'</td>
            <td class="p-2">Font size of the label</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>color</code></td>
            <td class="p-2">'subtle' | 'default'</td>
            <td class="p-2">'subtle'</td>
            <td class="p-2">Color variant</td>
          </tr>
          <tr>
            <td class="p-2"><code>class</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
            <td class="p-2">Additional CSS classes (use for margins)</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
