<script>
  import FormSectionTitle from '../../../lib/FormSectionTitle.svelte';
  import Label from '../../../lib/Label.svelte';
  import Input from '../../../lib/Input.svelte';
</script>

<div class="space-y-8">
  <div>
    <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">FormSectionTitle</h1>
    <p style="color: var(--ds-text-subtle);">
      Section title for forms with optional description.
      Use to divide forms into logical sections with clear headings.
    </p>
  </div>

  <!-- Basic -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic</h2>
    <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
      <FormSectionTitle title="General Settings" />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<FormSectionTitle title="General Settings" />`}
    </pre>
  </section>

  <!-- With Description -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Description</h2>
    <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
      <FormSectionTitle
        title="Advanced Options"
        description="Configure advanced settings for power users"
      />
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<FormSectionTitle
  title="Advanced Options"
  description="Configure advanced settings for power users"
/>`}
    </pre>
  </section>

  <!-- In Form Context -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">In Form Context</h2>
    <div class="p-6 rounded border space-y-6" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
      <FormSectionTitle
        title="Account Information"
        description="Basic details about your account"
      />
      <div class="space-y-4">
        <div>
          <Label for="name" class="mb-2">Display Name</Label>
          <Input id="name" placeholder="John Doe" />
        </div>
        <div>
          <Label for="email" class="mb-2">Email</Label>
          <Input id="email" type="email" placeholder="john@example.com" />
        </div>
      </div>

      <FormSectionTitle
        title="Preferences"
        description="Customize your experience"
        class="mt-8"
      />
      <div class="space-y-4">
        <div>
          <Label for="timezone" class="mb-2">Timezone</Label>
          <Input id="timezone" placeholder="UTC" />
        </div>
        <div>
          <Label for="language" class="mb-2">Language</Label>
          <Input id="language" placeholder="English" />
        </div>
      </div>
    </div>
    <pre class="mt-4 p-4 rounded text-sm overflow-x-auto" style="background-color: var(--ds-surface-raised); color: var(--ds-text);">
{`<FormSectionTitle
  title="Account Information"
  description="Basic details about your account"
/>
<div class="space-y-4">
  <div>
    <Label for="name" class="mb-2">Display Name</Label>
    <Input id="name" placeholder="John Doe" />
  </div>
  ...
</div>

<FormSectionTitle
  title="Preferences"
  description="Customize your experience"
  class="mt-8"
/>
...`}
    </pre>
  </section>

  <!-- Props -->
  <section>
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Props</h2>
    <div class="overflow-x-auto">
      <table class="w-full text-sm" style="color: var(--ds-text);">
        <thead>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <th class="text-left py-2 pr-4">Prop</th>
            <th class="text-left py-2 pr-4">Type</th>
            <th class="text-left py-2 pr-4">Default</th>
            <th class="text-left py-2">Description</th>
          </tr>
        </thead>
        <tbody>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">title</td>
            <td class="py-2 pr-4 font-mono text-xs">string</td>
            <td class="py-2 pr-4 font-mono text-xs">required</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Section title text</td>
          </tr>
          <tr class="border-b" style="border-color: var(--ds-border);">
            <td class="py-2 pr-4 font-mono text-xs">description</td>
            <td class="py-2 pr-4 font-mono text-xs">string</td>
            <td class="py-2 pr-4 font-mono text-xs">null</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Optional description text</td>
          </tr>
          <tr>
            <td class="py-2 pr-4 font-mono text-xs">class</td>
            <td class="py-2 pr-4 font-mono text-xs">string</td>
            <td class="py-2 pr-4 font-mono text-xs">''</td>
            <td class="py-2" style="color: var(--ds-text-subtle);">Additional CSS classes</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
