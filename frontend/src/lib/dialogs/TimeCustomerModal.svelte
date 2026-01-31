<script>
  import { tick } from 'svelte';
  import Modal from './Modal.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import Label from '../components/Label.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import { Camera, Trash2 } from 'lucide-svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';

  // Props
  let {
    isOpen = false,
    formData = $bindable({
      name: '',
      email: '',
      description: '',
      active: true,
      avatar_url: null,
      custom_field_values: {}
    }),
    customerOrgFields = [],
    attachmentsEnabled = false,
    isEditing = false,
    onsave = () => {},
    oncancel = () => {}
  } = $props();

  let uploadingAvatar = $state(false);
  let showAvatarUpload = $state(false);
  const nameInputId = 'customer-name-input';
  let nameInputRef = $state(null);

  // Avatar upload functionality
  async function handleAvatarUpload(files) {
    if (!files || files.length === 0) return;

    if (!attachmentsEnabled) {
      alert(t('organization.attachmentsRequired'));
      return;
    }

    const file = files[0];
    if (!file.type.startsWith('image/')) {
      alert(t('organization.pleaseSelectImage'));
      return;
    }

    uploadingAvatar = true;
    try {
      const uploadFormData = new FormData();
      uploadFormData.append('file', file);
      uploadFormData.append('item_id', '0');
      uploadFormData.append('category', 'customer_avatar');

      const uploadResult = await api.attachments.upload(uploadFormData);

      if (uploadResult && uploadResult.success && uploadResult.avatar_url) {
        formData = { ...formData, avatar_url: uploadResult.avatar_url };
        showAvatarUpload = false;
      }
    } catch (err) {
      alert(t('organization.failedToUploadAvatar') + ': ' + (err.message || err));
    } finally {
      uploadingAvatar = false;
    }
  }

  function removeAvatar() {
    formData = { ...formData, avatar_url: null };
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
      onsave();
    }
  }

  function handleCancel() {
    oncancel();
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

  $effect(() => {
    if (isOpen) {
      focusNameInput();
    }
  });
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
        {isEditing ? t('organization.editOrganization') : t('organization.newOrganization')}
      </h3>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <Label required class="mb-2">{t('organization.organizationName')}</Label>
          <Input id={nameInputId} bind:inputRef={nameInputRef} bind:value={formData.name} required />
        </div>

        <div>
          <Label class="mb-2">{t('organization.email')}</Label>
          <Input type="email" bind:value={formData.email} />
        </div>
      </div>

      <!-- Avatar Upload Section -->
      <div class="mt-6">
        <Label class="mb-2">{t('organization.organizationAvatar')}</Label>

        <!-- Avatar Preview -->
        {#if formData.avatar_url}
          <div class="flex items-center gap-4 mb-3">
            <img
              src={formData.avatar_url}
              alt="Organization avatar"
              class="w-16 h-16 rounded object-cover"
            />
            <div class="flex-1">
              <div class="text-sm font-medium" style="color: var(--ds-text);">{t('organization.customAvatar')}</div>
              <div class="text-xs" style="color: var(--ds-text-subtle);">{t('organization.uploadedImage')}</div>
            </div>
            <Button
              variant="default"
              size="sm"
              onclick={removeAvatar}
              icon={Trash2}
            >
              {t('common.remove')}
            </Button>
          </div>
        {:else}
          <div class="flex items-center gap-4 p-4 rounded border mb-3" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div class="w-16 h-16 rounded flex items-center justify-center text-white font-semibold text-xl" style="background-color: #3b82f6;">
              {getInitials(formData.name)}
            </div>
            <div class="flex-1">
              <div class="text-sm font-medium" style="color: var(--ds-text);">{t('organization.defaultAvatar')}</div>
              <div class="text-xs" style="color: var(--ds-text-subtle);">{t('organization.usingInitials')}</div>
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
            {formData.avatar_url ? t('organization.changeAvatar') : t('organization.uploadAvatar')}
          </Button>
          {#if !attachmentsEnabled}
            <p class="text-xs mt-1" style="color: var(--ds-text-warning);">
              {t('organization.attachmentsRequired')}
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
              <div class="mt-2 text-sm text-blue-600">{t('common.uploading')}</div>
            {/if}
            <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
              {t('organization.uploadRecommendation')}
            </p>
          </div>
        {/if}
      </div>

      <div class="mt-6">
        <Label class="mb-2">{t('common.description')}</Label>
        <Textarea bind:value={formData.description} rows={3} />
      </div>

      <div class="mt-6">
        <Checkbox
          bind:checked={formData.active}
          label={t('organization.activeOrganization')}
          size="small"
        />
      </div>

      <!-- Custom Fields -->
      {#if customerOrgFields.length > 0}
        <div class="mt-6 pt-6 border-t" style="border-color: var(--ds-border);">
          <h3 class="text-sm font-medium mb-4" style="color: var(--ds-text);">{t('organization.customFields')}</h3>
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
          {isEditing ? t('organization.updateOrganization') : t('organization.createOrganization')}
        </Button>
        <Button
          variant="default"
          onclick={handleCancel}
          size="medium"
          keyboardHint="Esc"
        >
          {t('common.cancel')}
        </Button>
      </div>
    </div>
  </Modal>
{/if}
