<script>
  import { onMount } from 'svelte';
  import { CheckSquare, Save, AlertCircle, Puzzle, Upload, RefreshCw, Trash2, ToggleLeft, ToggleRight, Package } from 'lucide-svelte';
  import { moduleSettings } from '../stores/moduleSettings.js';
  import Toggle from '../components/Toggle.svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import { api, getSecuritySettings, fetchAPI } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';

  let saving = $state(false);
  let error = $state('');
  let successMessage = $state('');

  // Plugin management
  let plugins = $state([]);
  let loadingPlugins = $state(false);
  let uploadingPlugin = $state(false);
  let selectedFile = $state(null);
  let selectedManifest = $state(null);
  let dragActive = $state(false);
  let fileInput;
  let manifestInput;

  // Local toggle state
  let testManagementEnabled = $state(false);
  let initialLoad = $state(true);

  // Plugin system state (from server startup flag)
  let pluginsDisabled = $state(false);

  // Sync toggle state with store when loaded
  $effect(() => {
    if ($moduleSettings.loaded && initialLoad) {
      testManagementEnabled = $moduleSettings.test_management_enabled;
      initialLoad = false;
    }
  });

  onMount(() => {
    moduleSettings.load().then(() => {
      initialLoad = false; // Enable auto-save after initial load
    });
    loadPlugins();
    loadSecuritySettings();
  });

  async function loadSecuritySettings() {
    try {
      const settings = await getSecuritySettings();
      pluginsDisabled = settings.plugins_disabled ?? false;
    } catch (err) {
      console.error('Failed to load security settings:', err);
    }
  }
  
  // Plugin management functions
  async function loadPlugins() {
    loadingPlugins = true;
    try {
      const data = await fetchAPI('/plugins');
      plugins = Array.isArray(data) ? data : [];
    } catch (err) {
      console.error('Failed to load plugins:', err);
      plugins = [];
    } finally {
      loadingPlugins = false;
    }
  }
  
  function handleFileDrop(event) {
    event.preventDefault();
    dragActive = false;
    
    const files = event.dataTransfer.files;
    if (files.length > 0) {
      selectedFile = files[0];
      // Check if there's a manifest.json as well
      for (let i = 1; i < files.length; i++) {
        if (files[i].name === 'manifest.json') {
          selectedManifest = files[i];
          break;
        }
      }
    }
  }
  
  function handleFileSelect(event) {
    const files = event.target.files;
    if (files.length > 0) {
      selectedFile = files[0];
    }
  }
  
  function handleManifestSelect(event) {
    const files = event.target.files;
    if (files.length > 0) {
      selectedManifest = files[0];
    }
  }
  
  async function uploadPlugin() {
    if (!selectedFile) {
      error = t('settings.modules.pleaseSelectPlugin');
      return;
    }

    if (selectedFile.name.endsWith('.wasm') && !selectedManifest) {
      error = t('settings.modules.wasmNeedsManifest');
      return;
    }

    uploadingPlugin = true;
    error = '';

    const formData = new FormData();
    formData.append('plugin', selectedFile);
    if (selectedManifest) {
      formData.append('manifest', selectedManifest);
    }

    try {
      const response = await fetch('/api/plugins/upload', {
        method: 'POST',
        credentials: 'same-origin',
        body: formData
      });

      if (!response.ok) {
        const errorData = await response.text();
        throw new Error(errorData || 'Upload failed');
      }

      successMessage = t('settings.modules.pluginUploadedSuccess');
      selectedFile = null;
      selectedManifest = null;
      await loadPlugins();

      setTimeout(() => {
        successMessage = '';
      }, 3000);
    } catch (err) {
      error = t('settings.modules.failedToUpload', { error: err.message });
    } finally {
      uploadingPlugin = false;
    }
  }
  
  async function togglePlugin(plugin) {
    try {
      await fetchAPI(`/plugins/${plugin.name}/toggle`, {
        method: 'PUT',
        body: JSON.stringify({ enabled: !plugin.enabled })
      });
      await loadPlugins();
    } catch (err) {
      console.error(`Failed to toggle plugin ${plugin.name}:`, err);
    }
  }
  
  async function reloadPlugin(plugin) {
    try {
      await fetchAPI(`/plugins/${plugin.name}/reload`, {
        method: 'POST'
      });

      successMessage = t('settings.modules.pluginReloadedSuccess', { name: plugin.name });
      await loadPlugins();
      setTimeout(() => successMessage = '', 3000);
    } catch (err) {
      error = t('settings.modules.failedToReload', { error: err.message });
    }
  }

  async function deletePlugin(plugin) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('settings.modules.confirmDeletePlugin', { name: plugin.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    try {
      await fetchAPI(`/plugins/${plugin.name}`, {
        method: 'DELETE'
      });

      successMessage = t('settings.modules.pluginDeletedSuccess', { name: plugin.name });
      await loadPlugins();
      setTimeout(() => successMessage = '', 3000);
    } catch (err) {
      error = t('settings.modules.failedToDelete', { error: err.message });
    }
  }

  async function saveSettings() {
    saving = true;
    error = '';
    successMessage = '';

    try {
      const newSettings = {
        time_tracking_enabled: true, // Always enabled
        test_management_enabled: testManagementEnabled
      };

      await moduleSettings.update(newSettings);
      successMessage = t('settings.modules.settingsSavedSuccess');

      // Clear success message after 3 seconds
      setTimeout(() => {
        successMessage = '';
      }, 3000);
    } catch (err) {
      console.error('Failed to save module settings:', err);
      error = t('settings.modules.failedToSave');
    } finally {
      saving = false;
    }
  }

  async function autoSave() {
    if (saving) return; // Prevent concurrent saves

    try {
      saving = true;
      error = '';

      const newSettings = {
        time_tracking_enabled: true, // Always enabled
        test_management_enabled: testManagementEnabled
      };

      await moduleSettings.update(newSettings);
    } catch (err) {
      console.error('Failed to auto-save module settings:', err);
      error = t('settings.modules.failedToSave');
    } finally {
      saving = false;
    }
  }

</script>

<PageHeader
  icon={Puzzle}
  title={t('settings.modules.title')}
  subtitle={t('settings.modules.subtitle')}
/>

  {#if $moduleSettings.loading}
    <div class="flex items-center justify-center py-12">
      <Spinner />
    </div>
  {:else}
    <!-- Success Message -->
    {#if successMessage}
      <div class="mb-6">
        <AlertBox type="success">{successMessage}</AlertBox>
      </div>
    {/if}

    <!-- Error Message -->
    {#if error}
      <div class="mb-6">
        <AlertBox type="error">{error}</AlertBox>
      </div>
    {/if}

    <div class="space-y-6">
      <!-- Test Management Module -->
      <div class="border rounded p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3">
            <CheckSquare class="w-5 h-5" style="color: var(--ds-text-subtle);" />
            <div>
              <h3 class="text-lg font-medium" style="color: var(--ds-text);">{t('testing.title')}</h3>
              <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                {t('testing.subtitle')}
              </p>
            </div>
          </div>
          <Toggle
            bind:checked={testManagementEnabled}
            onchange={autoSave}
          />
        </div>

      </div>
    </div>
    
    <!-- Plugin Management Section -->
    <div class="mt-8">
      <h2 class="text-xl font-semibold mb-4 flex items-center gap-2" style="color: var(--ds-text);">
        <Package class="w-5 h-5" />
        {t('settings.modules.plugins')}
      </h2>

      {#if pluginsDisabled}
        <!-- Plugins Disabled Notice -->
        <div class="mb-4">
          <AlertBox variant="info" message={t('settings.modules.pluginsDisabledMessage')} />
        </div>
      {:else}
        <!-- Plugin Upload -->
        <div class="border rounded p-6 mb-4" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <h3 class="text-lg font-medium mb-4" style="color: var(--ds-text);">{t('settings.modules.uploadPlugin')}</h3>

          <!-- Drag and Drop Area -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="border-2 border-dashed rounded p-8 text-center transition-colors drop-zone"
            class:drop-zone-active={dragActive}
            ondrop={handleFileDrop}
            ondragover={(e) => { e.preventDefault(); dragActive = true; }}
            ondragleave={(e) => { e.preventDefault(); dragActive = false; }}
          >
            <Upload class="w-12 h-12 mx-auto mb-4" style="color: var(--ds-text-subtle);" />
            <p class="text-sm mb-2" style="color: var(--ds-text);">
              {t('settings.modules.dropOrSelect')}
            </p>
            <p class="text-xs mb-4" style="color: var(--ds-text-subtle);">
              {t('settings.modules.supportedFormats')}
            </p>
            <input
              type="file"
              accept=".wasm,.zip"
              onchange={handleFileSelect}
              class="hidden"
              bind:this={fileInput}
            />
            <Button variant="primary" onclick={() => fileInput?.click()}>
              {t('common.select')}
            </Button>

            {#if selectedFile}
              <p class="mt-4 text-sm" style="color: var(--ds-text-subtle);">
                Selected: {selectedFile.name}
              </p>
            {/if}

            {#if selectedFile && selectedFile.name.endsWith('.wasm')}
              <div class="mt-4">
                <AlertBox type="warning">
                  <p class="text-sm mb-2 font-medium">
                    {t('settings.modules.wasmManifestRequired')}
                  </p>
                  <p class="text-xs mb-3">
                    {t('settings.modules.wasmManifestRequiredDesc')}
                  </p>
                  <input
                    type="file"
                    accept=".json"
                    onchange={handleManifestSelect}
                    class="hidden"
                    bind:this={manifestInput}
                  />
                  <Button variant="primary" size="sm" onclick={() => manifestInput?.click()}>
                    {selectedManifest ? t('settings.modules.changeManifest') : t('settings.modules.chooseManifest')}
                  </Button>
                  {#if selectedManifest}
                    <p class="mt-2 text-xs" style="color: var(--ds-text-success);">
                      ✓ {t('settings.modules.manifestSelected', { name: selectedManifest.name })}
                    </p>
                  {/if}
                </AlertBox>
              </div>
            {/if}
          </div>

          {#if selectedFile}
            <div class="mt-4">
              <Button
                variant="primary"
                onclick={uploadPlugin}
                disabled={uploadingPlugin}
              >
                {uploadingPlugin ? t('common.uploading') : t('common.upload')}
              </Button>
            </div>
          {/if}
        </div>
      {/if}
      
      <!-- Installed Plugins -->
      <div class="border rounded p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <h3 class="text-lg font-medium mb-4" style="color: var(--ds-text);">{t('settings.modules.installedPlugins')}</h3>

        {#if loadingPlugins}
          <div class="flex items-center justify-center py-8">
            <Spinner />
          </div>
        {:else if plugins.length === 0}
          <p class="text-center py-8" style="color: var(--ds-text-subtle);">
            {t('settings.modules.noPluginsInstalled')}
          </p>
        {:else}
          <div class="space-y-4">
            {#each plugins as plugin}
              <div class="border rounded p-4" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
                <div class="flex items-start justify-between">
                  <div class="flex-1">
                    <h4 class="font-medium" style="color: var(--ds-text);">
                      {plugin.name} <span class="text-sm" style="color: var(--ds-text-subtle);">v{plugin.version}</span>
                    </h4>
                    {#if plugin.description}
                      <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">{plugin.description}</p>
                    {/if}
                    {#if plugin.author}
                      <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('settings.modules.by')} {plugin.author}</p>
                    {/if}

                    {#if plugin.routes && plugin.routes.length > 0}
                      <div class="mt-3">
                        <p class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('settings.modules.registeredRoutes')}:</p>
                        <div class="space-y-1">
                          {#each plugin.routes as route}
                            <div class="text-xs font-mono rounded px-2 py-1" style="background-color: var(--ds-interactive-subtle); color: var(--ds-text);">
                              <span class="font-semibold">{route.method || 'ANY'}</span> /api/plugins/{plugin.name}{route.path}
                              {#if route.description}
                                <span class="ml-2" style="color: var(--ds-text-subtle);">- {route.description}</span>
                              {/if}
                            </div>
                          {/each}
                        </div>
                      </div>
                    {/if}
                  </div>
                  
                  {#if !pluginsDisabled}
                    <div class="flex items-center gap-2 ml-4">
                      <button
                        onclick={() => togglePlugin(plugin)}
                        class="p-2 rounded plugin-action-btn"
                        title={plugin.enabled ? t('common.disable') : t('common.enable')}
                      >
                        {#if plugin.enabled}
                          <ToggleRight class="w-5 h-5" style="color: var(--ds-text-success);" />
                        {:else}
                          <ToggleLeft class="w-5 h-5" style="color: var(--ds-icon-subtle);" />
                        {/if}
                      </button>

                      <button
                        onclick={() => reloadPlugin(plugin)}
                        class="p-2 rounded plugin-action-btn"
                        title={t('common.reload')}
                      >
                        <RefreshCw class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                      </button>

                      <button
                        onclick={() => deletePlugin(plugin)}
                        class="p-2 rounded plugin-delete-btn"
                        title={t('common.delete')}
                      >
                        <Trash2 class="w-4 h-4" style="color: var(--ds-text-danger);" />
                      </button>
                    </div>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>


    <!-- Information Notice -->
    <div class="mt-6">
      <AlertBox type="info">
        {t('settings.modules.moduleSettingsNote')}
      </AlertBox>
    </div>
  {/if}

<style>
  .drop-zone {
    border-color: var(--ds-border);
  }

  .drop-zone-active {
    border-color: var(--ds-border-focused);
    background-color: var(--ds-background-selected);
  }

  .plugin-action-btn:hover {
    background-color: var(--ds-surface-hovered);
  }

  .plugin-delete-btn:hover {
    background-color: var(--ds-background-danger-subtle);
  }
</style>
