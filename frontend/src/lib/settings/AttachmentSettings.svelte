<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { Save, AlertCircle, CheckCircle, X, Plus, Trash2, Paperclip } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Label from '../components/Label.svelte';
  import { successToast } from '../stores/toasts.svelte.js';
  import Toggle from '../components/Toggle.svelte';
  import { t } from '../stores/i18n.svelte.js';
  
  let settings = $state({
    id: 1,
    max_file_size: 52428800, // 50MB default
    allowed_mime_types: '[]',
    enabled: false
  });
  let status = $state({
    enabled: false,
    attachment_path: '',
    writable: false
  });
  let loading = $state(true);
  let saving = $state(false);
  let error = $state(null);
  
  // Form fields
  let maxFileSizeMB = $state(50);
  let allowedMimeTypes = $state([]);
  let enabled = $state(false);
  let newMimeType = $state('');
  
  // Common MIME type presets
  const mimeTypePresets = $derived([
    { label: t('settings.attachments.images'), types: ['image/jpeg', 'image/png', 'image/gif', 'image/webp'] },
    { label: t('settings.attachments.documents'), types: ['application/pdf', 'application/msword', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'] },
    { label: t('settings.attachments.spreadsheets'), types: ['application/vnd.ms-excel', 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'] },
    { label: t('settings.attachments.archives'), types: ['application/zip', 'application/x-rar-compressed', 'application/x-7z-compressed'] },
    { label: t('settings.attachments.text'), types: ['text/plain', 'text/csv', 'application/json'] }
  ]);

  // Get color for MIME type chip based on type category
  function getMimeTypeColor(mimeType) {
    if (mimeType.startsWith('image/')) return 'green';
    if (mimeType.startsWith('text/')) return 'sky';
    if (mimeType.includes('pdf') || mimeType.includes('word') || mimeType.includes('document')) return 'blue';
    if (mimeType.includes('sheet') || mimeType.includes('excel')) return 'emerald';
    if (mimeType.includes('zip') || mimeType.includes('compressed') || mimeType.includes('archive') || mimeType.includes('rar') || mimeType.includes('7z')) return 'purple';
    if (mimeType.includes('json')) return 'amber';
    return 'zinc';
  }
  
  onMount(async () => {
    await loadSettings();
    await loadStatus();
    loading = false;
    initialLoad = false; // Enable auto-save after initial load
  });
  
  async function loadSettings() {
    try {
      settings = await api.attachmentSettings.get();
      
      // Convert to form values
      maxFileSizeMB = Math.round(settings.max_file_size / 1048576); // bytes to MB
      enabled = settings.enabled;
      
      if (settings.allowed_mime_types) {
        try {
          allowedMimeTypes = JSON.parse(settings.allowed_mime_types);
        } catch (e) {
          allowedMimeTypes = [];
        }
      }
    } catch (err) {
      if (err.message.includes('Not Found')) {
        // Attachment functionality is not configured
        error = t('settings.attachments.notAvailable');
        settings = {
          enabled: false,
          attachment_path: null,
          max_file_size: 52428800,
          allowed_mime_types: '[]'
        };
      } else {
        console.error('Failed to load attachment settings:', err);
        error = t('settings.attachments.failedToLoad');
      }
    }
  }
  
  async function loadStatus() {
    try {
      status = await api.attachmentSettings.getStatus();
    } catch (err) {
      if (err.message.includes('Not Found') || err.message.includes('404')) {
        // Attachment status endpoint doesn't exist when attachments are not configured
        status = null;
      } else {
        console.error('Failed to load attachment status:', err);
      }
    }
  }
  
  async function saveSettings() {
    if (saving) return;

    try {
      saving = true;
      error = null;

      const requestData = {
        max_file_size: maxFileSizeMB * 1048576, // MB to bytes
        allowed_mime_types: allowedMimeTypes,
        enabled: enabled
      };

      await api.attachmentSettings.update(settings.id, requestData);

      successToast(t('settings.attachments.settingsSavedSuccess'));

      // Reload settings to get updated values
      await loadSettings();

    } catch (err) {
      console.error('Failed to save settings:', err);
      error = err.message || t('settings.attachments.failedToSave');
    } finally {
      saving = false;
    }
  }
  
  function addMimeType() {
    if (newMimeType.trim() && !allowedMimeTypes.includes(newMimeType.trim())) {
      allowedMimeTypes = [...allowedMimeTypes, newMimeType.trim()];
      newMimeType = '';
      // Auto-save after adding MIME type
      if (!initialLoad) saveSettings();
    }
  }
  
  function removeMimeType(index) {
    allowedMimeTypes = allowedMimeTypes.filter((_, i) => i !== index);
    // Auto-save after removing MIME type
    if (!initialLoad) saveSettings();
  }
  
  function addPresetMimeTypes(types) {
    const newTypes = types.filter(type => !allowedMimeTypes.includes(type));
    allowedMimeTypes = [...allowedMimeTypes, ...newTypes];
    // Auto-save after adding preset MIME types
    if (!initialLoad && newTypes.length > 0) saveSettings();
  }
  
  function clearAllMimeTypes() {
    allowedMimeTypes = [];
    // Auto-save after clearing all MIME types
    if (!initialLoad) saveSettings();
  }
  
  function clearError() {
    error = null;
  }
  
  // Format file size for display
  function formatFileSize(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }
  
  function handleKeydown(event) {
    if (event.key === 'Enter') {
      event.preventDefault();
      addMimeType();
    }
  }

  // Auto-save when settings change
  let initialLoad = $state(true);
  $effect(() => {
    if (!initialLoad && !saving && enabled !== settings.enabled) {
      saveSettings();
    }
  });

  $effect(() => {
    if (!initialLoad && !saving && maxFileSizeMB !== Math.round(settings.max_file_size / 1048576)) {
      clearTimeout(saveTimeout);
      saveTimeout = setTimeout(() => {
        saveSettings();
      }, 1000); // Debounce for 1 second
    }
  });

  let saveTimeout;
</script>

<PageHeader
  icon={Paperclip}
  title={t('settings.attachments.title')}
  subtitle={t('settings.attachments.subtitle')}
/>
    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Spinner />
      </div>
    {:else}
      <!-- Status Section -->
      {#if status}
        <div class="rounded border p-6 mb-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <h2 class="text-lg font-medium mb-4" style="color: var(--ds-text);">{t('settings.attachments.systemStatus')}</h2>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div class="flex items-center gap-3">
              {#if status.enabled}
                <CheckCircle class="w-5 h-5" style="color: var(--ds-accent-green);" />
                <span class="text-sm font-medium" style="color: var(--ds-accent-green-bolder);">{t('settings.attachments.attachmentsEnabled')}</span>
              {:else}
                <AlertCircle class="w-5 h-5" style="color: var(--ds-accent-red);" />
                <span class="text-sm font-medium" style="color: var(--ds-accent-red-bolder);">{t('settings.attachments.attachmentsDisabled')}</span>
              {/if}
            </div>

            <div class="text-sm">
              <span style="color: var(--ds-text-subtle);">{t('settings.attachments.storagePath')}</span>
              <br>
              <span class="font-mono text-xs" style="color: {status.attachment_path ? 'var(--ds-text)' : 'var(--ds-text-disabled)'};">
                {status.attachment_path || t('settings.attachments.notConfigured')}
              </span>
            </div>

            <div class="flex items-center gap-3">
              {#if status.writable}
                <CheckCircle class="w-5 h-5" style="color: var(--ds-accent-green);" />
                <span class="text-sm font-medium" style="color: var(--ds-accent-green-bolder);">{t('settings.attachments.pathWritable')}</span>
              {:else}
                <AlertCircle class="w-5 h-5" style="color: var(--ds-accent-yellow);" />
                <span class="text-sm font-medium" style="color: var(--ds-accent-yellow-bolder);">{t('settings.attachments.pathStatusUnknown')}</span>
              {/if}
            </div>
          </div>

          {#if !status.enabled}
            <div class="mt-4 p-3 rounded border-l-4" style="border-color: var(--ds-accent-blue); background-color: var(--ds-accent-blue-subtlest);">
              <p class="text-sm" style="color: var(--ds-accent-blue-bolder);">
                <strong>{t('settings.attachments.enableNote')}</strong>
                <br>
                {t('settings.attachments.enableExample')}
              </p>
            </div>
          {/if}
        </div>
      {:else}
        <div class="rounded border p-6 mb-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <h2 class="text-lg font-medium mb-4" style="color: var(--ds-text);">{t('settings.attachments.systemStatus')}</h2>
          <div class="p-3 rounded border-l-4" style="border-color: var(--ds-accent-blue); background-color: var(--ds-accent-blue-subtlest);">
            <p class="text-sm" style="color: var(--ds-accent-blue-bolder);">
              <strong>{t('settings.attachments.enableNote')}</strong>
              <br>
              {t('settings.attachments.enableExample')}
            </p>
          </div>
        </div>
      {/if}

      <!-- Settings Form -->
      <div class="space-y-6">
        <!-- General Settings -->
        <div class="rounded border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <h2 class="text-lg font-medium mb-4" style="color: var(--ds-text);">{t('settings.attachments.generalSettings')}</h2>

          <div class="space-y-4">
            <!-- Enable/Disable Toggle -->
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium" style="color: var(--ds-text);">
                  {t('settings.attachments.enableAttachments')}
                </label>
                <p class="text-xs" style="color: var(--ds-text-subtle);">{t('settings.attachments.enableAttachmentsDesc')}</p>
              </div>
<Toggle bind:checked={enabled} disabled={!status || !status.attachment_path} />
            </div>

            <!-- Max File Size -->
            <div>
              <Label color="default" class="mb-1">{t('settings.attachments.maxFileSize')}</Label>
              <input
                type="number"
                bind:value={maxFileSizeMB}
                min="1"
                max="1024"
                class="w-32 px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
              />
              <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                {t('settings.attachments.current')}: {formatFileSize(maxFileSizeMB * 1048576)}
              </p>
            </div>
          </div>
        </div>

        <!-- MIME Type Restrictions -->
        <div class="rounded border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <h2 class="text-lg font-medium mb-4" style="color: var(--ds-text);">{t('settings.attachments.fileTypeRestrictions')}</h2>
          <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
            {t('settings.attachments.fileTypeRestrictionsDesc')}
          </p>

          <!-- Quick Add Presets -->
          <div class="mb-4">
            <Label color="default" class="mb-2">{t('settings.attachments.quickAddCommonTypes')}</Label>
            <div class="flex flex-wrap gap-2">
              {#each mimeTypePresets as preset}
                <Button
                  variant="default"
                  size="small"
                  onclick={() => addPresetMimeTypes(preset.types)}
                  disabled={saving}
                >
                  + {preset.label}
                </Button>
              {/each}
              {#if allowedMimeTypes.length > 0}
                <Button
                  variant="default"
                  size="small"
                  onclick={clearAllMimeTypes}
                  disabled={saving}
                  class="text-red-600 hover:text-red-700"
                >
                  {t('settings.attachments.clearAll')}
                </Button>
              {/if}
            </div>
          </div>

          <!-- Add Custom MIME Type -->
          <div class="mb-4">
            <Label color="default" class="mb-1">{t('settings.attachments.addMimeType')}</Label>
            <div class="flex gap-2">
              <input
                type="text"
                bind:value={newMimeType}
                onkeydown={handleKeydown}
                placeholder={t('settings.attachments.mimeTypePlaceholder')}
                class="flex-1 px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
              />
              <Button
                variant="default"
                icon={Plus}
                onclick={addMimeType}
                disabled={!newMimeType.trim() || saving}
              >
                {t('common.add')}
              </Button>
            </div>
          </div>

          <!-- Current MIME Types -->
          {#if allowedMimeTypes.length > 0}
            <div>
              <Label color="default" class="mb-2">{t('settings.attachments.allowedMimeTypes')} ({allowedMimeTypes.length}):</Label>
              <div class="flex flex-wrap gap-2 max-h-48 overflow-y-auto p-1">
                {#each allowedMimeTypes as mimeType, index}
                  <Lozenge
                    color={getMimeTypeColor(mimeType)}
                    text={mimeType}
                    size="sm"
                    rounded="rounded-md"
                  >
                    <button
                      onclick={() => removeMimeType(index)}
                      disabled={saving}
                      class="ml-1 hover:opacity-70 transition-opacity disabled:opacity-50"
                      aria-label="Remove {mimeType}"
                    >
                      <X class="w-3 h-3" />
                    </button>
                  </Lozenge>
                {/each}
              </div>
            </div>
          {:else}
            <div class="text-center py-4" style="color: var(--ds-text-subtlest);">
              <p class="text-sm">{t('settings.attachments.allFileTypesAllowed')}</p>
            </div>
          {/if}
        </div>

        <!-- Error Messages -->
        {#if error}
          <div class="flex items-center gap-2 text-red-600">
            <AlertCircle class="w-4 h-4" />
            <span class="text-sm">{error}</span>
            <Button variant="ghost" icon={X} size="small" onclick={clearError} title="Dismiss" />
          </div>
        {/if}
      </div>
    {/if}
