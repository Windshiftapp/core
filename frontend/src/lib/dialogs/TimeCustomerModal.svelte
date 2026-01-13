<script>
  import { createEventDispatcher, tick } from 'svelte';
  import Modal from './Modal.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import Label from '../components/Label.svelte';
  import { Camera, Trash2 } from 'lucide-svelte';

  const dispatch = createEventDispatcher();

  // Props
  export let isOpen = false;
  export let formData = {
    name: '',
    email: '',
    description: '',
    active: true,
    avatar_url: null,
    custom_field_values: {}
  };
  export let customerOrgFields = [];
  export let attachmentsEnabled = false;
  export let isEditing = false;

  let uploadingAvatar = false;
  let showAvatarUpload = false;
  const nameInputId = 'customer-name-input';
  let nameInputRef = null;

  // Avatar upload functionality
  async function handleAvatarUpload(files) {
    if (!files || files.length === 0) return;

    if (!attachmentsEnabled) {
      alert('Attachments must be enabled to upload customer avatars');
      return;
    }

    const file = files[0];
    if (!file.type.startsWith('image/')) {
      alert('Please select an image file');
      return;
    }

    uploadingAvatar = true;
    try {
      const uploadFormData = new FormData();
      uploadFormData.append('file', file);
      uploadFormData.append('item_id', '0');
      uploadFormData.append('category', 'customer_avatar');

      const response = await fetch('/api/attachments/upload', {
        method: 'POST',
        body: uploadFormData,
      });

      if (!response.ok) {
        throw new Error(`Upload failed: ${response.statusText}`);
      }

      const uploadResult = await response.json();

      if (uploadResult && uploadResult.success && uploadResult.avatar_url) {
        formData.avatar_url = uploadResult.avatar_url;
        showAvatarUpload = false;
        alert('Avatar uploaded successfully');
      }
    } catch (err) {
      alert('Failed to upload avatar: ' + (err.message || err));
    } finally {
      uploadingAvatar = false;
    }
  }

  function removeAvatar() {
    formData.avatar_url = null;
  }

  function getInitials(name) {
    if (!name) return '?';
    const parts = name.trim().split(' ');
    if (parts.length >= 2) {
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
    }
    return name.substring(0, 2).toUpperCase();
  }

  function handleSubmit() {
    if (formData.name.trim()) {
      dispatch('save');
    }
  }

  function handleCancel() {
    dispatch('cancel');
  }

  async function focusNameInput() {
    await tick();
    // Wait for Modal's own focus logic (100ms) to complete
    setTimeout(() => {
      const el = nameInputRef || document.getElementById(nameInputId);
      if (el) {
        el.focus();
        el.select();
      }
    }, 120);
  }

  $: if (isOpen) {
    focusNameInput();
  }
</script>

{#if isOpen}
  <Modal
    {isOpen}
    onSubmit={handleSubmit}
    submitDisabled={!formData.name.trim()}
    maxWidth="max-w-2xl"
    onclose={handleCancel}
    let:submitHint
  >
    <div class="p-6">
      <h3 class="text-xl font-semibold mb-6" style="color: var(--ds-text);">
        {isEditing ? 'Edit Customer' : 'New Customer'}
      </h3>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <Label required class="mb-2">Customer Name</Label>
          <Input id={nameInputId} bind:inputRef={nameInputRef} bind:value={formData.name} required />
        </div>

        <div>
          <Label class="mb-2">Email</Label>
          <Input type="email" bind:value={formData.email} />
        </div>
      </div>

      <!-- Avatar Upload Section -->
      <div class="mt-6">
        <Label class="mb-2">Customer Avatar</Label>

        <!-- Avatar Preview -->
        {#if formData.avatar_url}
          <div class="flex items-center gap-4 mb-3">
            <img
              src={formData.avatar_url}
              alt="Customer avatar"
              class="w-16 h-16 rounded object-cover"
            />
            <div class="flex-1">
              <div class="text-sm font-medium" style="color: var(--ds-text);">Custom Avatar</div>
              <div class="text-xs" style="color: var(--ds-text-subtle);">Uploaded image</div>
            </div>
            <Button
              variant="default"
              size="sm"
              onclick={removeAvatar}
              icon={Trash2}
            >
              Remove
            </Button>
          </div>
        {:else}
          <div class="flex items-center gap-4 p-4 rounded border mb-3" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="w-16 h-16 rounded flex items-center justify-center text-white font-semibold text-xl" style="background-color: #3b82f6;">
              {getInitials(formData.name)}
            </div>
            <div class="flex-1">
              <div class="text-sm font-medium" style="color: var(--ds-text);">Default Avatar</div>
              <div class="text-xs" style="color: var(--ds-text-subtle);">Using initials</div>
            </div>
          </div>
        {/if}

        <!-- Upload Controls -->
        <div>
          <Button
            variant="default"
            size="sm"
            onclick={() => showAvatarUpload = !showAvatarUpload}
            icon={Camera}
            disabled={!attachmentsEnabled}
          >
            {formData.avatar_url ? 'Change Avatar' : 'Upload Avatar'}
          </Button>
          {#if !attachmentsEnabled}
            <p class="text-xs mt-1" style="color: var(--ds-text-warning);">
              Attachments must be enabled to upload customer avatars
            </p>
          {/if}
        </div>

        <!-- Upload Input (shown when toggled) -->
        {#if showAvatarUpload && attachmentsEnabled}
          <div class="mt-3 p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <input
              type="file"
              accept="image/*"
              onchange={(e) => handleAvatarUpload(e.target.files)}
              disabled={uploadingAvatar}
              class="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100 disabled:opacity-50"
            />
            {#if uploadingAvatar}
              <div class="mt-2 text-sm text-blue-600">Uploading...</div>
            {/if}
            <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
              Recommended: Square images, at least 256x256 pixels for best quality
            </p>
          </div>
        {/if}
      </div>

      <div class="mt-6">
        <Label class="mb-2">Description</Label>
        <Textarea bind:value={formData.description} rows={3} />
      </div>

      <div class="mt-6 flex items-center">
        <input
          type="checkbox"
          bind:checked={formData.active}
          id="active"
          class="mr-3 w-4 h-4 text-blue-600 rounded focus:ring-2 focus:ring-blue-500"
        />
        <label for="active" class="text-sm font-medium" style="color: var(--ds-text);">Active Customer</label>
      </div>

      <!-- Custom Fields -->
      {#if customerOrgFields.length > 0}
        <div class="mt-6 pt-6 border-t" style="border-color: var(--ds-border);">
          <h3 class="text-sm font-medium mb-4" style="color: var(--ds-text);">Custom Fields</h3>
          <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
            {#each customerOrgFields as field}
              <CustomFieldRenderer
                {field}
                bind:value={formData.custom_field_values[field.name]}
                readonly={false}
                onChange={(val) => {
                  formData.custom_field_values[field.name] = val;
                }}
              />
            {/each}
          </div>
        </div>
      {/if}

      <div class="mt-8 flex gap-3">
        <Button
          variant="primary"
          onclick={handleSubmit}
          disabled={!formData.name.trim()}
          size="medium"
          keyboardHint={submitHint}
        >
          {isEditing ? 'Update' : 'Create'} Customer
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
