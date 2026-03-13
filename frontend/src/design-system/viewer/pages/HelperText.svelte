<script>
  import HelperText from '../../../lib/components/HelperText.svelte'
  import Label from '../../../lib/components/Label.svelte'
  import Input from '../../../lib/components/Input.svelte'
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">HelperText</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Helper text component for form fields, supporting hints, errors, success, and warning messages.
  </p>

  <!-- Variants -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Variants</h2>
    <div class="space-y-4">
      <div>
        <HelperText>This is default helper text with hints for the user.</HelperText>
      </div>
      <div>
        <HelperText variant="error">This field is required.</HelperText>
      </div>
      <div>
        <HelperText variant="success">Email address is available!</HelperText>
      </div>
      <div>
        <HelperText variant="warning">Password is weak. Consider using a stronger one.</HelperText>
      </div>
    </div>
  </section>

  <!-- In Context -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Form Examples</h2>
    <div class="space-y-6 max-w-md">
      <!-- With hint -->
      <div>
        <Label for="username" class="mb-2">Username</Label>
        <Input id="username" placeholder="johndoe" />
        <HelperText>Choose a unique username (3-20 characters)</HelperText>
      </div>

      <!-- With error -->
      <div>
        <Label for="email" required class="mb-2">Email</Label>
        <Input id="email" value="invalid-email" class="border-red-500" />
        <HelperText variant="error">Please enter a valid email address.</HelperText>
      </div>

      <!-- With success -->
      <div>
        <Label for="available" class="mb-2">Domain Name</Label>
        <Input id="available" value="my-awesome-site" />
        <HelperText variant="success">This domain is available!</HelperText>
      </div>

      <!-- With warning -->
      <div>
        <Label for="password" class="mb-2">Password</Label>
        <Input id="password" type="password" value="password123" />
        <HelperText variant="warning">This password has been found in data breaches.</HelperText>
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
            <td class="p-2"><code>variant</code></td>
            <td class="p-2">'default' | 'error' | 'success' | 'warning'</td>
            <td class="p-2">'default'</td>
            <td class="p-2">Visual style of the helper text</td>
          </tr>
          <tr>
            <td class="p-2"><code>class</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
            <td class="p-2">Additional CSS classes</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Token Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Token Reference</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Variant</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Token</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2">default</td>
            <td class="p-2"><code>--ds-text-subtle</code></td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2">error</td>
            <td class="p-2"><code>--ds-text-danger</code></td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2">success</td>
            <td class="p-2"><code>--ds-text-success</code></td>
          </tr>
          <tr>
            <td class="p-2">warning</td>
            <td class="p-2"><code>--ds-text-warning</code></td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</div>
