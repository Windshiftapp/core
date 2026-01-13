<script>
  import { createEventDispatcher } from 'svelte';
  import Modal from './Modal.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';

  const dispatch = createEventDispatcher();

  // Props
  export let isOpen = false;
  export let formData = {
    name: '',
    description: ''
  };
  export let isEditing = false;

  function handleSubmit() {
    if (formData.name.trim()) {
      dispatch('save');
    }
  }

  function handleCancel() {
    dispatch('cancel');
  }
</script>

{#if isOpen}
  <Modal
    {isOpen}
    onSubmit={handleSubmit}
    submitDisabled={!formData.name.trim()}
    maxWidth="max-w-lg"
    onclose={handleCancel}
    let:submitHint
  >
    <div class="p-6">
      <h3 class="text-xl font-semibold mb-6" style="color: var(--ds-text);">
        {isEditing ? 'Edit Category' : 'New Category'}
      </h3>

      <div class="space-y-4">
        <div>
          <Label required class="mb-2">Category Name</Label>
          <Input
            bind:value={formData.name}
            placeholder="Development, Marketing, Operations..."
            required
          />
        </div>

        <div>
          <Label class="mb-2">Description</Label>
          <Textarea
            bind:value={formData.description}
            rows={3}
            placeholder="Optional description..."
          />
        </div>
      </div>

      <div class="mt-6 flex gap-3">
        <Button
          variant="primary"
          onclick={handleSubmit}
          disabled={!formData.name.trim()}
          size="medium"
          keyboardHint={submitHint}
        >
          {isEditing ? 'Update' : 'Create'} Category
        </Button>
        <Button
          variant="default"
          onclick={handleCancel}
          size="medium"
          keyboardHint="Esc"
        >
          Cancel
        </Button>
      </div>
    </div>
  </Modal>
{/if}
