<script>
  import { onMount } from 'svelte';
  import { CheckSquare, Save, AlertCircle, Puzzle, Upload, RefreshCw, Trash2, ToggleLeft, ToggleRight, Package } from 'lucide-svelte';
  import { moduleSettings } from '../stores/moduleSettings.js';
  import Toggle from '../components/Toggle.svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import { api } from '../api.js';
  
  let saving = false;
  let error = '';
  let successMessage = '';
  
  // Plugin management
  let plugins = [];
  let loadingPlugins = false;
  let uploadingPlugin = false;
  let selectedFile = null;
  let selectedManifest = null;
  let dragActive = false;
  let fileInput;
  let manifestInput;

  // Local toggle state
  let testManagementEnabled = false;
  let initialLoad = true;

  // Sync toggle state with store when loaded
  $: if ($moduleSettings.loaded && initialLoad) {
    testManagementEnabled = $moduleSettings.test_management_enabled;
    initialLoad = false;
  }

  onMount(() => {
    moduleSettings.load().then(() => {
      initialLoad = false; // Enable auto-save after initial load
    });
    loadPlugins();
  });
  
  // Plugin management functions
  async function loadPlugins() {
    loadingPlugins = true;
    try {
      const response = await fetch('/api/plugins');
      if (response.ok) {
        const data = await response.json();
        plugins = Array.isArray(data) ? data : [];
      } else {
        plugins = [];
      }
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
      error = 'Please select a plugin file';
      return;
    }
    
    if (selectedFile.name.endsWith('.wasm') && !selectedManifest) {
      error = 'WASM files require a manifest.json file. Please select a manifest.json or upload a .zip file containing both files.';
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
        body: formData
      });
      
      if (response.ok) {
        successMessage = 'Plugin uploaded successfully!';
        selectedFile = null;
        selectedManifest = null;
        await loadPlugins();
        
        setTimeout(() => {
          successMessage = '';
        }, 3000);
      } else {
        const errorData = await response.text();
        error = `Upload failed: ${errorData}`;
      }
    } catch (err) {
      error = `Upload failed: ${err.message}`;
    } finally {
      uploadingPlugin = false;
    }
  }
  
  async function togglePlugin(plugin) {
    try {
      const response = await fetch(`/api/plugins/${plugin.name}/toggle`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: !plugin.enabled })
      });
      
      if (response.ok) {
        await loadPlugins();
      }
    } catch (err) {
      console.error(`Failed to toggle plugin ${plugin.name}:`, err);
    }
  }
  
  async function reloadPlugin(plugin) {
    try {
      const response = await fetch(`/api/plugins/${plugin.name}/reload`, {
        method: 'POST'
      });
      
      if (response.ok) {
        successMessage = `Plugin ${plugin.name} reloaded successfully`;
        await loadPlugins();
        setTimeout(() => successMessage = '', 3000);
      }
    } catch (err) {
      error = `Failed to reload plugin: ${err.message}`;
    }
  }
  
  async function deletePlugin(plugin) {
    if (!confirm(`Are you sure you want to delete the plugin "${plugin.name}"?`)) {
      return;
    }
    
    try {
      const response = await fetch(`/api/plugins/${plugin.name}`, {
        method: 'DELETE'
      });
      
      if (response.ok) {
        successMessage = `Plugin ${plugin.name} deleted successfully`;
        await loadPlugins();
        setTimeout(() => successMessage = '', 3000);
      }
    } catch (err) {
      error = `Failed to delete plugin: ${err.message}`;
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
      successMessage = 'Module settings saved successfully!';

      // Clear success message after 3 seconds
      setTimeout(() => {
        successMessage = '';
      }, 3000);
    } catch (err) {
      console.error('Failed to save module settings:', err);
      error = 'Failed to save module settings. Please try again.';
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
      error = 'Failed to save module settings. Please try again.';
    } finally {
      saving = false;
    }
  }

</script>

<PageHeader 
  icon={Puzzle} 
  title="Module Settings" 
  subtitle="Configure which modules are enabled in your Windshift installation"
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
              <h3 class="text-lg font-medium" style="color: var(--ds-text);">Test Management</h3>
              <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
                Manage test cases, test runs, test sets, and quality assurance workflows
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
        Plugin Management
      </h2>
      
      <!-- Plugin Upload -->
      <div class="border rounded p-6 mb-4" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <h3 class="text-lg font-medium mb-4" style="color: var(--ds-text);">Upload Plugin</h3>
        
        <!-- Drag and Drop Area -->
        <div
          class="border-2 border-dashed rounded p-8 text-center transition-colors drop-zone"
          class:drop-zone-active={dragActive}
          ondrop={handleFileDrop}
          ondragover={(e) => { e.preventDefault(); dragActive = true; }}
          ondragleave={(e) => { e.preventDefault(); dragActive = false; }}
        >
          <Upload class="w-12 h-12 mx-auto mb-4" style="color: var(--ds-text-subtle);" />
          <p class="text-sm mb-2" style="color: var(--ds-text);">
            Drag and drop a plugin file here, or click to browse
          </p>
          <p class="text-xs mb-4" style="color: var(--ds-text-subtle);">
            <strong>Recommended:</strong> Upload a .zip file containing both plugin.wasm and manifest.json<br>
          </p>
          <input
            type="file"
            accept=".wasm,.zip"
            onchange={handleFileSelect}
            class="hidden"
            bind:this={fileInput}
          />
          <Button variant="primary" onclick={() => fileInput?.click()}>
            Choose Plugin File
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
                  Manifest Required for WASM Files
                </p>
                <p class="text-xs mb-3">
                  WASM files must be accompanied by a manifest.json file that describes the plugin.
                </p>
                <input
                  type="file"
                  accept=".json"
                  onchange={handleManifestSelect}
                  class="hidden"
                  bind:this={manifestInput}
                />
                <Button variant="primary" size="sm" onclick={() => manifestInput?.click()}>
                  {selectedManifest ? 'Change' : 'Choose'} manifest.json
                </Button>
                {#if selectedManifest}
                  <p class="mt-2 text-xs" style="color: var(--ds-text-success);">
                    ✓ Manifest selected: {selectedManifest.name}
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
              {uploadingPlugin ? 'Uploading...' : 'Upload Plugin'}
            </Button>
          </div>
        {/if}
      </div>
      
      <!-- Installed Plugins -->
      <div class="border rounded p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <h3 class="text-lg font-medium mb-4" style="color: var(--ds-text);">Installed Plugins</h3>
        
        {#if loadingPlugins}
          <div class="flex items-center justify-center py-8">
            <Spinner />
          </div>
        {:else if plugins.length === 0}
          <p class="text-center py-8" style="color: var(--ds-text-subtle);">
            No plugins installed yet
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
                      <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">By {plugin.author}</p>
                    {/if}
                    
                    {#if plugin.routes && plugin.routes.length > 0}
                      <div class="mt-3">
                        <p class="text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">Registered Routes:</p>
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
                  
                  <div class="flex items-center gap-2 ml-4">
                    <button
                      onclick={() => togglePlugin(plugin)}
                      class="p-2 rounded plugin-action-btn"
                      title={plugin.enabled ? 'Disable plugin' : 'Enable plugin'}
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
                      title="Reload plugin"
                    >
                      <RefreshCw class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                    </button>

                    <button
                      onclick={() => deletePlugin(plugin)}
                      class="p-2 rounded plugin-delete-btn"
                      title="Delete plugin"
                    >
                      <Trash2 class="w-4 h-4" style="color: var(--ds-text-danger);" />
                    </button>
                  </div>
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
        <strong>Note:</strong> Module changes will affect navigation and available features.
        Some UI elements may require a page refresh to reflect the new settings.
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
