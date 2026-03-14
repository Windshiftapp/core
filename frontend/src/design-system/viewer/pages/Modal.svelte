<script>
  import Modal from '../../../lib/dialogs/Modal.svelte'
  import Button from '../../../lib/components/Button.svelte'
  import Input from '../../../lib/components/Input.svelte'
  import FormField from '../../../lib/components/FormField.svelte'

  let basicOpen = $state(false)
  let customWidthOpen = $state(false)
  let preventCloseOpen = $state(false)
  let formOpen = $state(false)
</script>

<div class="p-8 max-w-6xl">
  <h1 class="text-2xl font-bold mb-2" style="color: var(--ds-text);">Modal</h1>
  <p class="mb-8" style="color: var(--ds-text-subtle);">
    Base modal component with backdrop, close button, and flexible content slot.
  </p>

  <!-- Basic Usage -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Usage</h2>
    <Button variant="primary" on:click={() => basicOpen = true}>
      Open Basic Modal
    </Button>

    <Modal bind:isOpen={basicOpen} onclose={() => basicOpen = false}>
      <div class="p-6">
        <h3 class="text-lg font-semibold mb-2">Basic Modal</h3>
        <p class="text-gray-600 mb-4">
          This is a basic modal with default settings. Click the X button or outside the modal to close.
        </p>
        <div class="flex justify-end">
          <Button variant="primary" on:click={() => basicOpen = false}>
            Close
          </Button>
        </div>
      </div>
    </Modal>
  </section>

  <!-- Custom Width -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Custom Width</h2>
    <div class="flex gap-2">
      <Button variant="secondary" on:click={() => customWidthOpen = true}>
        Open Wide Modal (max-w-2xl)
      </Button>
    </div>

    <Modal bind:isOpen={customWidthOpen} maxWidth="max-w-2xl" onclose={() => customWidthOpen = false}>
      <div class="p-6">
        <h3 class="text-lg font-semibold mb-2">Wide Modal</h3>
        <p class="text-gray-600 mb-4">
          This modal uses <code>maxWidth="max-w-2xl"</code> for a wider content area.
          Available options include: <code>max-w-sm</code>, <code>max-w-md</code>, <code>max-w-lg</code>,
          <code>max-w-xl</code>, <code>max-w-2xl</code>, <code>max-w-3xl</code>, etc.
        </p>
        <div class="flex justify-end">
          <Button variant="primary" on:click={() => customWidthOpen = false}>
            Close
          </Button>
        </div>
      </div>
    </Modal>
  </section>

  <!-- Prevent Close -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Prevent Close</h2>
    <Button variant="secondary" on:click={() => preventCloseOpen = true}>
      Open Persistent Modal
    </Button>

    <Modal
      bind:isOpen={preventCloseOpen}
      preventClose
      showCloseButton={false}
    >
      <div class="p-6">
        <h3 class="text-lg font-semibold mb-2">Persistent Modal</h3>
        <p class="text-gray-600 mb-4">
          This modal cannot be closed by clicking outside or pressing Escape.
          The user must explicitly click the button to close it.
        </p>
        <div class="flex justify-end">
          <Button variant="primary" on:click={() => preventCloseOpen = false}>
            I understand, close modal
          </Button>
        </div>
      </div>
    </Modal>
  </section>

  <!-- With Form -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">With Form Content</h2>
    <Button variant="primary" on:click={() => formOpen = true}>
      Open Form Modal
    </Button>

    <Modal
      bind:isOpen={formOpen}
      onclose={() => formOpen = false}
      onSubmit={() => {
        alert('Form submitted!')
        formOpen = false
      }}
    >
      {#snippet children(submitHint)}
      <div class="p-6">
        <h3 class="text-lg font-semibold mb-4">Create New Item</h3>

        <FormField label="Name" required>
          <Input placeholder="Enter item name" />
        </FormField>

        <FormField label="Description">
          <Input placeholder="Enter description" />
        </FormField>

        <div class="flex justify-end gap-2 mt-6 pt-4 border-t">
          <Button variant="ghost" on:click={() => formOpen = false}>
            Cancel
          </Button>
          <Button variant="primary" keyboardHint={submitHint}>
            Create
          </Button>
        </div>
      </div>
      {/snippet}
    </Modal>
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
            <td class="p-2"><code>isOpen</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>showCloseButton</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">true</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>preventClose</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>maxWidth</code></td>
            <td class="p-2">string (Tailwind class)</td>
            <td class="p-2">'max-w-lg'</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>autoFocus</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">true</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>onSubmit</code></td>
            <td class="p-2">function</td>
            <td class="p-2">null</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>submitDisabled</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <td class="p-2"><code>zIndexClass</code></td>
            <td class="p-2">string</td>
            <td class="p-2">'z-50'</td>
          </tr>
          <tr>
            <td class="p-2"><code>noBackdrop</code></td>
            <td class="p-2">boolean</td>
            <td class="p-2">false</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Events Reference -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Events</h2>
    <div
      class="p-4 rounded-lg overflow-x-auto"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--ds-border);">
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Event</th>
            <th class="text-left p-2 font-medium" style="color: var(--ds-text);">Description</th>
          </tr>
        </thead>
        <tbody style="color: var(--ds-text-subtle);">
          <tr>
            <td class="p-2"><code>onclose</code></td>
            <td class="p-2">Fired when modal is closed via backdrop click, X button, or Escape key</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <!-- Slot Props -->
  <section class="mb-10">
    <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Slot Props</h2>
    <div
      class="p-4 rounded-lg"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        When using <code>onSubmit</code>, the slot receives a <code>submitHint</code> prop that provides
        the appropriate keyboard hint (↵ for simple forms, ⌘↵ for forms with textareas).
      </p>
    </div>
  </section>
</div>
