<script>
  import FormField from '../../../lib/FormField.svelte'
  import Input from '../../../lib/Input.svelte'
  import Select from '../../../lib/Select.svelte'
  import Textarea from '../../../lib/Textarea.svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">FormField</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Form field wrapper component that provides consistent labeling, helper text, and error messaging.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <div class="max-w-md space-y-4">
      <FormField label="Email Address" id="email">
        <Input type="email" id="email" placeholder="user@example.com" />
      </FormField>

      <FormField label="Full Name" id="name">
        <Input id="name" placeholder="John Doe" />
      </FormField>
    </div>
  </section>

  <!-- With Helper Text -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Helper Text</h2>
    <div class="max-w-md space-y-4">
      <FormField
        label="Username"
        id="username"
        helper="Choose a unique username. It will be visible to other users."
      >
        <Input id="username" placeholder="johndoe" />
      </FormField>

      <FormField
        label="Password"
        id="password"
        helper="Must be at least 8 characters with one uppercase and one number."
      >
        <Input type="password" id="password" placeholder="Enter password" />
      </FormField>
    </div>
  </section>

  <!-- Required Fields -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Required Fields</h2>
    <div class="max-w-md space-y-4">
      <FormField label="Project Name" id="project" required>
        <Input id="project" placeholder="Enter project name" required />
      </FormField>

      <FormField label="Category" id="category" required>
        <Select id="category" required>
          <option value="">Select a category...</option>
          <option value="dev">Development</option>
          <option value="design">Design</option>
          <option value="marketing">Marketing</option>
        </Select>
      </FormField>
    </div>
  </section>

  <!-- Error States -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Error States</h2>
    <div class="max-w-md space-y-4">
      <FormField
        label="Email"
        id="email-error"
        error="Please enter a valid email address."
      >
        <Input id="email-error" value="invalid-email" />
      </FormField>

      <FormField
        label="Description"
        id="desc-error"
        error="Description must be at least 10 characters."
        helper="This helper is hidden when there's an error."
      >
        <Textarea id="desc-error" value="Too short" rows={2} />
      </FormField>
    </div>
  </section>

  <!-- With Different Input Types -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Different Input Types</h2>
    <div class="max-w-md space-y-4">
      <FormField label="Select Option" id="select-example">
        <Select id="select-example">
          <option value="">Choose...</option>
          <option value="1">Option 1</option>
          <option value="2">Option 2</option>
        </Select>
      </FormField>

      <FormField label="Description" id="textarea-example" helper="Provide a detailed description.">
        <Textarea id="textarea-example" placeholder="Enter description..." rows={4} />
      </FormField>
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
            <td class="p-2"><code>label</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>required</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>helper</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>error</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>id</code></td>
            <td class="p-2">string</td>
            <td class="p-2">null</td>
          </tr>
          <tr>
            <td class="p-2"><code>children</code></td>
            <td class="p-2">snippet</td>
            <td class="p-2">-</td>
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
        The <code>error</code> prop takes precedence over <code>helper</code> - when an error is present,
        the helper text will be hidden and replaced by the error message.
      </p>
    </div>
  </section>
</div>
