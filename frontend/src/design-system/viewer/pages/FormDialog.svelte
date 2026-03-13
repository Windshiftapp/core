<script>
  import FormDialog from '../../../lib/dialogs/FormDialog.svelte'
  import DialogFooter from '../../../lib/dialogs/DialogFooter.svelte'
  import Button from '../../../lib/components/Button.svelte'
  import Input from '../../../lib/components/Input.svelte'
  import FormField from '../../../lib/components/FormField.svelte'
  import { User, Trash2, Settings } from 'lucide-svelte'

  let basicOpen = $state(false)
  let dangerOpen = $state(false)
  let loadingOpen = $state(false)
  let loading = $state(false)

  function handleSubmit() {
    loading = true
    setTimeout(() => {
      loading = false
      loadingOpen = false
    }, 2000)
  }
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">FormDialog</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Pre-composed form dialog with header, content area, and footer with action buttons.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <Button variant="primary" on:click={() => basicOpen = true}>
      Open Form Dialog
    </Button>

    <FormDialog
      bind:isOpen={basicOpen}
      title="Create User"
      subtitle="Add a new user to your team"
      icon={User}
      confirmLabel="Create User"
      onClose={() => basicOpen = false}
      onSubmit={() => {
        alert('User created!')
        basicOpen = false
      }}
    >
      <FormField label="Name" required>
        <Input placeholder="Enter full name" />
      </FormField>
      <FormField label="Email" required>
        <Input type="email" placeholder="user@example.com" />
      </FormField>
      <FormField label="Role">
        <Input placeholder="Enter role" />
      </FormField>
    </FormDialog>
  </section>

  <!-- Danger Variant -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Danger Variant</h2>
    <Button variant="danger" on:click={() => dangerOpen = true}>
      Delete Item
    </Button>

    <FormDialog
      bind:isOpen={dangerOpen}
      title="Delete Project"
      subtitle="This action cannot be undone"
      icon={Trash2}
      variant="danger"
      confirmLabel="Delete Project"
      onClose={() => dangerOpen = false}
      onSubmit={() => {
        alert('Deleted!')
        dangerOpen = false
      }}
    >
      <p class="text-gray-600">
        Are you sure you want to delete this project? All data will be permanently removed.
        This action cannot be undone.
      </p>
    </FormDialog>
  </section>

  <!-- Loading State -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Loading State</h2>
    <Button variant="primary" on:click={() => loadingOpen = true}>
      Open With Loading
    </Button>

    <FormDialog
      bind:isOpen={loadingOpen}
      title="Save Settings"
      subtitle="Configure your preferences"
      icon={Settings}
      confirmLabel="Save"
      loading={loading}
      onClose={() => loadingOpen = false}
      onSubmit={handleSubmit}
    >
      <FormField label="Setting 1">
        <Input placeholder="Enter value" disabled={loading} />
      </FormField>
      <FormField label="Setting 2">
        <Input placeholder="Enter value" disabled={loading} />
      </FormField>
    </FormDialog>
  </section>

  <!-- DialogFooter Component -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">DialogFooter Component</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      The footer with Cancel/Confirm buttons can be used standalone.
    </p>
    <div
      class="rounded-lg overflow-hidden"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <div class="p-4">
        <p style="color: var(--ds-text);">Some content above the footer...</p>
      </div>
      <DialogFooter
        cancelLabel="Cancel"
        confirmLabel="Save Changes"
        onCancel={() => alert('Cancelled')}
        onConfirm={() => alert('Confirmed')}
      />
    </div>
  </section>

  <!-- DialogFooter as Form Footer -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Form Footer Variant</h2>
    <p class="mb-4 text-sm" style="color: var(--ds-text-subtle);">
      DialogFooter configured for form submission with keyboard hint and loading text.
    </p>
    <div
      class="rounded-lg overflow-hidden"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <div class="p-4">
        <p style="color: var(--ds-text);">Some content above the footer...</p>
      </div>
      <DialogFooter
        cancelLabel="Cancel"
        confirmLabel="Save"
        confirmType="submit"
        showKeyboardHint={true}
        onCancel={() => alert('Cancelled')}
        onConfirm={() => alert('Saved')}
      />
    </div>
  </section>

  <!-- Props Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">FormDialog Props</h2>
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
            <td class="p-2"><code>isOpen</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>title</code></td>
            <td class="p-2">string</td>
            <td class="p-2">-</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>subtitle</code></td>
            <td class="p-2">string</td>
            <td class="p-2">''</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>icon</code></td>
            <td class="p-2">Lucide icon component</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>confirmLabel</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Save'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>cancelLabel</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Cancel'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>variant</code></td>
            <td class="p-2">'primary' | 'danger'</td>
            <td class="p-2">'primary'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>loading</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>maxWidth</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'max-w-lg'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onClose</code></td>
            <td class="p-2">function</td>
            <td class="p-2">null</td>
          </tr>
          <tr>
            <td class="p-2"><code>onSubmit</code></td>
            <td class="p-2">function</td>
            <td class="p-2">null</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- DialogFooter Props -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">DialogFooter Props</h2>
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
            <td class="p-2"><code>cancelLabel</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Cancel'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>confirmLabel</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'Confirm'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>variant</code></td>
            <td class="p-2">'primary' | 'danger'</td>
            <td class="p-2">'primary'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>loading</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>disabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>showCancel</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">true</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onCancel</code></td>
            <td class="p-2">function</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onConfirm</code></td>
            <td class="p-2">function</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>extra</code></td>
            <td class="p-2">snippet</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>confirmType</code></td>
            <td class="p-2">'button' | 'submit'</td>
            <td class="p-2">'button'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>showKeyboardHint</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>loadingLabel</code></td>
            <td class="p-2">string</td>
            <td class="p-2">null</td>
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
</div>
