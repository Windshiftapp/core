<script>
  import { onMount } from 'svelte';
  import { writable } from 'svelte/store';
  import { api } from '../api.js';
  import { Plus, Edit, Trash2, Palette, Check, X } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Spinner from '../components/Spinner.svelte';
  import ColorPicker from '../editors/ColorPicker.svelte';
  import Label from '../components/Label.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';

  // State management
  let themes = $state([]);
  let activeTheme = $state(null);
  let loading = $state(true);
  let error = $state(null);
  let showCreateForm = $state(false);
  let editingTheme = $state(null);

  // Form data
  let newTheme = $state({
    name: '',
    description: '',
    nav_background_color_light: '#ffffff',
    nav_text_color_light: '#374151',
    nav_background_color_dark: '#1f2937',
    nav_text_color_dark: '#f3f4f6'
  });

  // Load themes and active theme
  onMount(async () => {
    await loadThemes();
    await loadActiveTheme();
  });

  async function loadThemes() {
    try {
      loading = true;
      error = null;
      themes = await api.themes.getAll();
    } catch (err) {
      error = 'Failed to load themes: ' + err.message;
      console.error('Error loading themes:', err);
    } finally {
      loading = false;
    }
  }

  async function loadActiveTheme() {
    try {
      activeTheme = await api.themes.getActive();
    } catch (err) {
      console.error('Error loading active theme:', err);
    }
  }

  async function createTheme() {
    try {
      error = null;
      const created = await api.themes.create(newTheme);
      themes = [...themes, created];
      
      // Reset form
      newTheme = {
        name: '',
        description: '',
        nav_background_color_light: '#ffffff',
        nav_text_color_light: '#374151',
        nav_background_color_dark: '#1f2937',
        nav_text_color_dark: '#f3f4f6'
      };
      showCreateForm = false;
    } catch (err) {
      error = 'Failed to create theme: ' + err.message;
      console.error('Error creating theme:', err);
    }
  }

  async function updateTheme(id, data) {
    try {
      error = null;
      const updated = await api.themes.update(id, data);
      themes = themes.map(t => t.id === id ? updated : t);
      editingTheme = null;
      
      // If this theme is active, update active theme
      if (updated.is_active) {
        activeTheme = updated;
      }
    } catch (err) {
      error = 'Failed to update theme: ' + err.message;
      console.error('Error updating theme:', err);
    }
  }

  async function deleteTheme(id) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('dialogs.confirmations.deleteTheme'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    try {
      error = null;
      await api.themes.delete(id);
      themes = themes.filter(t => t.id !== id);
    } catch (err) {
      error = 'Failed to delete theme: ' + err.message;
      console.error('Error deleting theme:', err);
    }
  }

  async function activateTheme(id) {
    try {
      error = null;
      await api.themes.activate(id);
      
      // Update local state
      themes = themes.map(t => ({ ...t, is_active: t.id === id }));
      activeTheme = themes.find(t => t.id === id);
      
      // Apply theme immediately
      applyTheme(activeTheme);
    } catch (err) {
      error = 'Failed to activate theme: ' + err.message;
      console.error('Error activating theme:', err);
    }
  }

  function applyTheme(theme) {
    if (!theme) return;

    const root = document.documentElement;
    const isDark = root.dataset.colorMode === 'dark';

    root.style.setProperty(
      '--nav-bg-color',
      isDark ? theme.nav_background_color_dark : theme.nav_background_color_light
    );
    root.style.setProperty(
      '--nav-text-color',
      isDark ? theme.nav_text_color_dark : theme.nav_text_color_light
    );
  }

  function startEdit(theme) {
    editingTheme = { ...theme };
  }

  function cancelEdit() {
    editingTheme = null;
  }

  function handleCreateSubmit(event) {
    event.preventDefault();
    createTheme();
  }

  function handleEditSubmit(e) {
    e.preventDefault();
    updateTheme(editingTheme.id, {
      name: editingTheme.name,
      description: editingTheme.description,
      nav_background_color_light: editingTheme.nav_background_color_light,
      nav_text_color_light: editingTheme.nav_text_color_light,
      nav_background_color_dark: editingTheme.nav_background_color_dark,
      nav_text_color_dark: editingTheme.nav_text_color_dark,
      is_active: editingTheme.is_active
    });
  }
</script>

<div class="theme-manager">
  <PageHeader
    icon={Palette}
    title={t('settings.theme')}
    description={t('settings.appearance')}
  >
    {#snippet actions()}
      <Button
        variant="primary"
        icon={Plus}
        onclick={() => showCreateForm = !showCreateForm}
        keyboardHint="A"
        hotkeyConfig={{ key: toHotkeyString('themes', 'add'), guard: () => !showCreateForm }}
      >
        {t('common.create')}
      </Button>
    {/snippet}
  </PageHeader>
  
  {#if activeTheme}
    <div class="mb-6 flex items-center space-x-2 text-sm" style="color: var(--ds-text-subtle);">
      <Palette class="w-4 h-4" />
      <span>{t('common.active')}: <strong style="color: var(--ds-text);">{activeTheme.name}</strong></span>
    </div>
  {/if}

  {#if error}
    <div class="px-4 py-3 rounded-md mb-6" style="background-color: var(--ds-danger-subtle); border: 1px solid var(--ds-border-danger); color: var(--ds-text-danger);">
      {error}
    </div>
  {/if}

<Modal isOpen={showCreateForm} onclose={() => showCreateForm = false} maxWidth="max-w-2xl">
  <ModalHeader
    title={editingTheme ? t('common.edit') : t('common.create')}
    onClose={() => showCreateForm = false}
  />

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); createTheme(); }}>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
        <div>
          <Label for="name" color="default" class="mb-2">{t('common.name')}</Label>
          <input
            type="text"
            id="name"
            bind:value={newTheme.name}
            placeholder={t('common.name')}
            required
            class="w-full px-3 py-2 rounded-md focus:outline-none focus:ring-2" style="border: 1px solid var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
          />
        </div>

        <div>
          <Label for="description" color="default" class="mb-2">{t('common.description')}</Label>
          <input
            type="text"
            id="description"
            bind:value={newTheme.description}
            placeholder={t('placeholders.optionalDescription')}
            class="w-full px-3 py-2 rounded-md focus:outline-none focus:ring-2" style="border: 1px solid var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
          />
        </div>
      </div>

      <!-- Light Mode Colors -->
      <div class="mb-4">
        <h4 class="text-sm font-semibold mb-3 flex items-center gap-2" style="color: var(--ds-text);">
          <span class="w-3 h-3 rounded-full bg-yellow-400"></span>
          {t('settings.lightMode')}
        </h4>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <Label color="default" class="mb-2">{t('common.color')}</Label>
            <ColorPicker bind:value={newTheme.nav_background_color_light} compact={true} />
          </div>

          <div>
            <Label color="default" class="mb-2">{t('common.color')}</Label>
            <ColorPicker bind:value={newTheme.nav_text_color_light} compact={true} />
          </div>
        </div>
      </div>

      <!-- Dark Mode Colors -->
      <div class="mb-4">
        <h4 class="text-sm font-semibold mb-3 flex items-center gap-2" style="color: var(--ds-text);">
          <span class="w-3 h-3 rounded-full bg-gray-700"></span>
          {t('settings.darkMode')}
        </h4>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <Label color="default" class="mb-2">{t('common.color')}</Label>
            <ColorPicker bind:value={newTheme.nav_background_color_dark} compact={true} />
          </div>

          <div>
            <Label color="default" class="mb-2">{t('common.color')}</Label>
            <ColorPicker bind:value={newTheme.nav_text_color_dark} compact={true} />
          </div>
        </div>
      </div>
    </form>
  </div>

  <DialogFooter
    confirmLabel={editingTheme ? t('common.update') : t('common.create')}
    disabled={!newTheme.name}
    onCancel={() => showCreateForm = false}
    onConfirm={createTheme}
  />
</Modal>

  <!-- Themes List -->
  {#if loading}
    <div class="flex justify-center py-8">
      <Spinner />
    </div>
  {:else}
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      {#each themes as theme (theme.id)}
        <div class="rounded overflow-hidden" style="background-color: var(--ds-surface); border: 1px solid var(--ds-border);">
          <!-- Theme Previews (Light and Dark side by side) -->
          <div class="flex">
            <div
              class="h-14 flex-1 flex items-center px-4"
              style="background-color: {theme.nav_background_color_light}; color: {theme.nav_text_color_light};"
            >
              <div class="flex items-center space-x-2">
                <Palette class="w-4 h-4" />
                <span class="font-medium text-sm">Light</span>
              </div>
            </div>
            <div
              class="h-14 flex-1 flex items-center px-4"
              style="background-color: {theme.nav_background_color_dark}; color: {theme.nav_text_color_dark};"
            >
              <div class="flex items-center space-x-2">
                <Palette class="w-4 h-4" />
                <span class="font-medium text-sm">Dark</span>
              </div>
            </div>
          </div>

          <!-- Theme Info -->
          <div class="p-4">
            {#if editingTheme && editingTheme.id === theme.id}
              <!-- Edit Form -->
              <form onsubmit={handleEditSubmit} class="space-y-4">
                <div>
                  <Label color="default" class="mb-1">Name</Label>
                  <input
                    type="text"
                    bind:value={editingTheme.name}
                    required
                    class="w-full px-3 py-2 rounded-md focus:outline-none focus:ring-2" style="border: 1px solid var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
                  />
                </div>

                <div>
                  <Label color="default" class="mb-1">Description</Label>
                  <input
                    type="text"
                    bind:value={editingTheme.description}
                    class="w-full px-3 py-2 rounded-md focus:outline-none focus:ring-2" style="border: 1px solid var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
                  />
                </div>

                <!-- Light Mode Colors -->
                <div class="mb-3">
                  <h5 class="text-xs font-semibold mb-2 flex items-center gap-1" style="color: var(--ds-text-subtle);">
                    <span class="w-2 h-2 rounded-full bg-yellow-400"></span>
                    Light Mode
                  </h5>
                  <div class="grid grid-cols-2 gap-3">
                    <div>
                      <label class="block text-xs mb-1" style="color: var(--ds-text-subtle);">Background</label>
                      <ColorPicker bind:value={editingTheme.nav_background_color_light} compact={true} />
                    </div>
                    <div>
                      <label class="block text-xs mb-1" style="color: var(--ds-text-subtle);">Text</label>
                      <ColorPicker bind:value={editingTheme.nav_text_color_light} compact={true} />
                    </div>
                  </div>
                </div>

                <!-- Dark Mode Colors -->
                <div>
                  <h5 class="text-xs font-semibold mb-2 flex items-center gap-1" style="color: var(--ds-text-subtle);">
                    <span class="w-2 h-2 rounded-full bg-gray-700"></span>
                    Dark Mode
                  </h5>
                  <div class="grid grid-cols-2 gap-3">
                    <div>
                      <label class="block text-xs mb-1" style="color: var(--ds-text-subtle);">Background</label>
                      <ColorPicker bind:value={editingTheme.nav_background_color_dark} compact={true} />
                    </div>
                    <div>
                      <label class="block text-xs mb-1" style="color: var(--ds-text-subtle);">Text</label>
                      <ColorPicker bind:value={editingTheme.nav_text_color_dark} compact={true} />
                    </div>
                  </div>
                </div>

                <div class="flex justify-end space-x-2 mt-4">
                  <button
                    type="button"
                    onclick={cancelEdit}
                    class="flex items-center space-x-1 px-3 py-1 text-sm rounded transition-colors"
                    style="color: var(--ds-text-subtle); background-color: var(--ds-surface-secondary);"
                  >
                    <X class="w-3 h-3" />
                    <span>{t('common.cancel')}</span>
                  </button>
                  <button
                    type="submit"
                    class="flex items-center space-x-1 px-3 py-1 text-sm text-white rounded transition-colors"
                    style="background-color: var(--ds-background-brand); color: white;"
                  >
                    <Check class="w-3 h-3" />
                    <span>{t('common.save')}</span>
                  </button>
                </div>
              </form>
            {:else}
              <!-- Display Mode -->
              <div class="flex justify-between items-start mb-4">
                <div>
                  <h3 class="text-lg font-semibold flex items-center space-x-2" style="color: var(--ds-text);">
                    <span>{theme.name}</span>
                    {#if theme.is_default}
                      <span class="px-2 py-1 text-xs rounded" style="background-color: var(--ds-surface-secondary); color: var(--ds-text-subtle);">{t('common.default')}</span>
                    {/if}
                    {#if theme.is_active}
                      <span class="px-2 py-1 text-xs rounded" style="background-color: var(--ds-surface-success); color: var(--ds-text-success);">{t('common.active')}</span>
                    {/if}
                  </h3>
                  {#if theme.description}
                    <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">{theme.description}</p>
                  {/if}
                </div>
              </div>

              <div class="grid grid-cols-2 gap-4 mb-4">
                <!-- Light Mode Info -->
                <div class="text-sm">
                  <h5 class="text-xs font-semibold mb-1 flex items-center gap-1" style="color: var(--ds-text-subtle);">
                    <span class="w-2 h-2 rounded-full bg-yellow-400"></span>
                    Light
                  </h5>
                  <div class="space-y-1">
                    <div>
                      <span style="color: var(--ds-text-subtle);">Bg:</span>
                      <span class="font-mono" style="color: var(--ds-text);">{theme.nav_background_color_light}</span>
                    </div>
                    <div>
                      <span style="color: var(--ds-text-subtle);">Text:</span>
                      <span class="font-mono" style="color: var(--ds-text);">{theme.nav_text_color_light}</span>
                    </div>
                  </div>
                </div>
                <!-- Dark Mode Info -->
                <div class="text-sm">
                  <h5 class="text-xs font-semibold mb-1 flex items-center gap-1" style="color: var(--ds-text-subtle);">
                    <span class="w-2 h-2 rounded-full bg-gray-700"></span>
                    Dark
                  </h5>
                  <div class="space-y-1">
                    <div>
                      <span style="color: var(--ds-text-subtle);">Bg:</span>
                      <span class="font-mono" style="color: var(--ds-text);">{theme.nav_background_color_dark}</span>
                    </div>
                    <div>
                      <span style="color: var(--ds-text-subtle);">Text:</span>
                      <span class="font-mono" style="color: var(--ds-text);">{theme.nav_text_color_dark}</span>
                    </div>
                  </div>
                </div>
              </div>

              <div class="flex justify-between items-center">
                <div class="flex space-x-2">
                  {#if !theme.is_active}
                    <Button
                      variant="primary"
                      size="sm"
                      icon={Check}
                      onclick={() => activateTheme(theme.id)}
                    >
                      {t('common.enable')}
                    </Button>
                  {/if}

                  {#if !theme.is_default}
                    <button
                      onclick={() => startEdit(theme)}
                      class="flex items-center space-x-1 px-3 py-1 text-sm rounded transition-colors hover-edit-btn"
                      style="color: var(--ds-text-subtle); background-color: var(--ds-surface-secondary);"
                    >
                      <Edit class="w-3 h-3" />
                      <span>{t('common.edit')}</span>
                    </button>
                  {/if}
                </div>

                {#if !theme.is_default}
                  <button
                    onclick={() => deleteTheme(theme.id)}
                    class="flex items-center space-x-1 px-3 py-1 text-sm rounded transition-colors"
                    style="color: var(--ds-text-danger); background-color: var(--ds-danger-subtle);"
                  >
                    <Trash2 class="w-3 h-3" />
                    <span>{t('common.delete')}</span>
                  </button>
                {/if}
              </div>
            {/if}
          </div>
        </div>
      {/each}
    </div>

    {#if themes.length === 0}
      <div class="text-center py-12">
        <Palette class="w-12 h-12 mx-auto mb-4" style="color: var(--ds-text-subtle);" />
        <h3 class="text-lg font-medium mb-2" style="color: var(--ds-text);">{t('common.noData')}</h3>
        <p class="mb-4" style="color: var(--ds-text-subtle);">{t('settings.appearance')}</p>
        <Button
          variant="primary"
          icon={Plus}
          onclick={() => showCreateForm = true}
        >
          {t('common.create')}
        </Button>
      </div>
    {/if}
  {/if}
</div>

<style>
  .theme-manager {
    max-width: 100%;
    padding: 0;
  }

  .hover-edit-btn:hover {
    background-color: var(--ds-surface-tertiary) !important;
  }
</style>