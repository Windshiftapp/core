<script>
  import { Trash2, Upload, User } from '@lucide/svelte';
  import { api } from '../api.js';
  import AlertBox from '../components/AlertBox.svelte';
  import Button from '../components/Button.svelte';
  import DescriptionText from '../components/DescriptionText.svelte';
  import FileInput from '../components/FileInput.svelte';
  import Spinner from '../components/Spinner.svelte';
  import { confirm } from '../composables/useConfirm.js';
  import { authStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';

  let { user = $bindable(null), userId = null } = $props();

  let error = $state('');
  let showUpload = $state(false);
  let uploading = $state(false);

  async function upload(files) {
    if (!userId || !files?.length) return;

    const file = files[0];
    if (!file.type.startsWith('image/')) {
      error = t('dialogs.alerts.pleaseSelectImage');
      return;
    }

    uploading = true;
    error = '';
    try {
      const formData = new FormData();
      formData.append('file', file);
      formData.append('item_id', '0');
      formData.append('category', 'avatar');

      const uploadResult = await api.attachments.upload(formData);
      if (uploadResult?.success && uploadResult.avatar_url) {
        user = await api.updateUserAvatar(userId, uploadResult.avatar_url);
        authStore.patchCurrentUser({ avatar_url: user.avatar_url || uploadResult.avatar_url });
        showUpload = false;
      }
    } catch (err) {
      error = err.message || t('dialogs.alerts.failedToUpload', { error: 'avatar' });
    } finally {
      uploading = false;
    }
  }

  async function remove() {
    if (!userId) return;
    const confirmed = await confirm({
      title: t('common.remove'),
      message: t('dialogs.confirmations.removeAvatar'),
      confirmText: t('common.remove'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;

    try {
      user = await api.updateUserAvatar(userId, null);
      authStore.patchCurrentUser({ avatar_url: '' });
    } catch (err) {
      error = err.message || t('dialogs.alerts.failedToDelete', { error: 'avatar' });
    }
  }
</script>

{#if error}
  <AlertBox message={error} />
{/if}

<div class="flex items-center justify-between mb-6">
  <div>
    <h2 class="text-lg font-medium flex items-center gap-2" style="color: var(--ds-text);">
      <User class="h-5 w-5" style="color: var(--ds-text-subtle);" />
      {t('users.profilePicture')}
    </h2>
    <p class="text-sm" style="color: var(--ds-text-subtle);">{t('users.uploadAndManageAvatar')}</p>
  </div>
  <div class="flex items-center gap-2">
    {#if user?.avatar_url}
      <Button variant="default" onclick={remove} icon={Trash2} size="medium">
        {t('common.remove')}
      </Button>
    {/if}
    <Button
      variant="primary"
      onclick={() => (showUpload = !showUpload)}
      icon={Upload}
      size="medium"
    >
      {user?.avatar_url ? t('users.changeAvatar') : t('users.uploadAvatar')}
    </Button>
  </div>
</div>

<div class="flex items-center gap-6 mb-6">
  <div class="relative">
    {#if user?.avatar_url}
      <img
        class="h-20 w-20 rounded-full border-2"
        style="border-color: var(--ds-border);"
        src={user.avatar_url}
        alt={t('users.currentProfilePicture')}
      />
    {:else}
      <div
        class="h-20 w-20 rounded-full flex items-center justify-center border-2"
        style="background-color: var(--ds-background-neutral); border-color: var(--ds-border);"
      >
        <User class="h-10 w-10" style="color: var(--ds-icon);" />
      </div>
    {/if}
  </div>
  <div>
    <h3 class="font-medium" style="color: var(--ds-text);">{t('users.currentProfilePicture')}</h3>
    <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
      {user?.avatar_url ? t('users.customAvatarActive') : t('users.usingDefaultAvatar')}
    </p>
    <DescriptionText variant="subtlest">{t('users.avatarRecommendation')}</DescriptionText>
  </div>
</div>

{#if showUpload}
  <div
    class="border rounded p-4"
    style="background-color: var(--ds-surface-sunken); border-color: var(--ds-border);"
  >
    <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">
      {t('users.uploadNewAvatar')}
    </h3>
    <div class="mb-4">
      <FileInput
        accept="image/*"
        onchange={(event) => upload(/** @type {HTMLInputElement} */ (event.target).files)}
        disabled={uploading}
        class="block w-full text-sm file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-blue-600 file:text-white hover:file:bg-blue-700 disabled:opacity-50"
        style="color: var(--ds-text-subtle);"
      />
      <p class="text-xs mt-2" style="color: var(--ds-text-subtlest);">{t('users.avatarFileHint')}</p>
    </div>
    {#if uploading}
      <div class="mb-4 flex items-center gap-2 text-sm" style="color: var(--ds-text-subtle);">
        <Spinner size="sm" />
        {t('users.uploadingAvatar')}
      </div>
    {/if}
    <div class="flex justify-end">
      <Button variant="default" onclick={() => (showUpload = false)} size="small" disabled={uploading}>
        {t('common.cancel')}
      </Button>
    </div>
  </div>
{/if}
