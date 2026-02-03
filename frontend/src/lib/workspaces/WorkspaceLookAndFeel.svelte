<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { workspacePermissions, currentWorkspace, attachmentStatus, workspacesStore } from '../stores';
  import { workspaceGradientIndex, applyToAllViews as applyToAllViewsStore, workspaceBackgroundImageUrl } from '../stores/workspaceGradient.svelte.js';
  import { gradients } from '../utils/gradients.js';
  import { backgroundCategories, backgroundPresets, getPresetsByCategory } from '../utils/backgroundImages.js';
  import { workspaceIconMap } from '../utils/icons.js';
  import { Palette, Camera, Trash2, X, Shield, Package, Upload, Image } from 'lucide-svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Label from '../components/Label.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  let { workspaceId = null } = $props();

  let workspace = $state(null);
  let loading = $state(true);

  // Display mode
  let displayMode = $state('default');

  // Gradient and background image
  let selectedGradient = $state(0);
  let backgroundImageUrl = $state(null);
  let currentLayout = $state(null);

  // Background image upload state
  let uploadingBackground = $state(false);
  let showBackgroundUpload = $state(false);
  let selectedBackgroundCategory = $state('abstract');

  // Identity
  let icon = $state('Package');
  let color = $state('#3b82f6');
  let avatarUrl = $state(null);

  // Avatar upload state
  let uploadingAvatar = $state(false);
  let showAvatarUpload = $state(false);

  // Permission check
  const canAdmin = $derived(workspacePermissions.canAdminWorkspace(workspaceId));

  const iconMap = workspaceIconMap;

  // Background: image takes priority over gradient
  const hasBackgroundImage = $derived(backgroundImageUrl !== null && backgroundImageUrl !== '');
  const hasGradient = $derived(!hasBackgroundImage && selectedGradient > 0 && gradients[selectedGradient]?.value);
  const hasCustomBackground = $derived(hasBackgroundImage || hasGradient);

  // Compute background style
  const backgroundStyle = $derived(() => {
    if (hasBackgroundImage) {
      return `background: linear-gradient(rgba(0,0,0,0.3), rgba(0,0,0,0.3)), url(${backgroundImageUrl}) center/cover no-repeat fixed;`;
    }
    if (hasGradient) {
      return `background: ${gradients[selectedGradient].value};`;
    }
    return 'background-color: var(--ds-surface);';
  });

  // Debounced auto-save
  let saveTimeout;
  function debouncedSave() {
    clearTimeout(saveTimeout);
    saveTimeout = setTimeout(() => save(), 1000);
  }

  onDestroy(() => {
    clearTimeout(saveTimeout);
  });

  onMount(async () => {
    try {
      const [ws, layout] = await Promise.all([
        api.workspaces.get(workspaceId),
        api.workspaces.getHomepageLayout(workspaceId)
      ]);

      workspace = ws;
      if (workspace) {
        displayMode = workspace.display_mode || 'default';
        icon = workspace.icon || 'Package';
        color = workspace.color || '#3b82f6';
        avatarUrl = workspace.avatar_url || null;
      }

      currentLayout = layout;
      if (layout) {
        selectedGradient = layout.gradient ?? 0;
        backgroundImageUrl = layout.backgroundImageUrl ?? null;
      }
    } catch (error) {
      console.error('Failed to load look and feel data:', error);
    } finally {
      loading = false;
    }
  });

  async function save() {
    try {
      const layoutPayload = {
        sections: currentLayout?.sections || [],
        widgets: currentLayout?.widgets || [],
        gradient: selectedGradient,
        applyToAllViews: true,
        backgroundImageUrl: backgroundImageUrl || ''
      };

      await Promise.all([
        api.workspaces.update(workspaceId, {
          name: workspace.name,
          key: workspace.key,
          description: workspace.description || '',
          active: workspace.active,
          time_project_id: workspace.time_project_id || null,
          default_view: workspace.default_view || 'board',
          display_mode: displayMode,
          icon,
          color,
          avatar_url: avatarUrl
        }),
        api.workspaces.updateHomepageLayout(workspaceId, layoutPayload)
      ]);

      // Update currentWorkspace store immediately (no full reload)
      currentWorkspace.patch({
        display_mode: displayMode,
        icon,
        color,
        avatar_url: avatarUrl
      });

      // Also update the workspacesStore so the dropdown shows updated icon/color
      workspacesStore.updateWorkspace(workspaceId, {
        icon,
        color,
        avatar_url: avatarUrl,
        display_mode: displayMode
      });

      // Update gradient and background stores
      workspaceGradientIndex.set(selectedGradient);
      applyToAllViewsStore.set(true);
      workspaceBackgroundImageUrl.set(backgroundImageUrl);
    } catch (error) {
      console.error('Failed to save:', error);
      errorToast(t('lookAndFeel.failedToSave', { error: error.message || error }));
    }
  }

  async function handleAvatarUpload(files) {
    if (!files || files.length === 0) return;

    if (!attachmentStatus.enabled) {
      errorToast(t('workspaceSettings.attachmentsRequired'));
      return;
    }

    const file = files[0];
    if (!file.type.startsWith('image/')) {
      errorToast(t('workspaceSettings.pleaseSelectImage'));
      return;
    }

    uploadingAvatar = true;
    try {
      const uploadFormData = new FormData();
      uploadFormData.append('file', file);
      uploadFormData.append('item_id', workspaceId.toString());
      uploadFormData.append('category', 'workspace_avatar');

      const response = await fetch('/api/attachments/upload', {
        method: 'POST',
        body: uploadFormData,
      });

      if (!response.ok) {
        throw new Error(`Upload failed: ${response.statusText}`);
      }

      const uploadResult = await response.json();

      if (uploadResult && uploadResult.success && uploadResult.avatar_url) {
        avatarUrl = uploadResult.avatar_url;
        showAvatarUpload = false;
        successToast(t('workspaceSettings.avatarUploadedSuccess'));
        save();
      }
    } catch (err) {
      errorToast(t('workspaceSettings.failedToUploadAvatar', { error: err.message || err }));
    } finally {
      uploadingAvatar = false;
    }
  }

  function removeAvatar() {
    avatarUrl = null;
    debouncedSave();
  }

  function handleIconChange(event) {
    icon = event.detail.icon;
    color = event.detail.color;
    debouncedSave();
  }

  function selectGradient(index) {
    selectedGradient = index;
    // Clear background image when selecting a gradient
    if (index > 0) {
      backgroundImageUrl = null;
      workspaceBackgroundImageUrl.set(null);
    }
    workspaceGradientIndex.set(index);
    applyToAllViewsStore.set(true);
    debouncedSave();
  }

  function selectBackgroundImage(url) {
    backgroundImageUrl = url;
    // Clear gradient when selecting a background image
    selectedGradient = 0;
    workspaceGradientIndex.set(0);
    workspaceBackgroundImageUrl.set(url);
    applyToAllViewsStore.set(true);
    debouncedSave();
  }

  function removeBackgroundImage() {
    backgroundImageUrl = null;
    workspaceBackgroundImageUrl.set(null);
    debouncedSave();
  }

  async function handleBackgroundUpload(files) {
    if (!files || files.length === 0) return;

    if (!attachmentStatus.enabled) {
      errorToast(t('workspaceSettings.attachmentsRequired'));
      return;
    }

    const file = files[0];
    if (!file.type.startsWith('image/')) {
      errorToast(t('workspaceSettings.pleaseSelectImage'));
      return;
    }

    uploadingBackground = true;
    try {
      const uploadFormData = new FormData();
      uploadFormData.append('file', file);
      uploadFormData.append('item_id', workspaceId.toString());
      uploadFormData.append('category', 'workspace_background');

      const response = await fetch('/api/attachments/upload', {
        method: 'POST',
        body: uploadFormData,
      });

      if (!response.ok) {
        throw new Error(`Upload failed: ${response.statusText}`);
      }

      const uploadResult = await response.json();

      if (uploadResult && uploadResult.success && uploadResult.background_url) {
        selectBackgroundImage(uploadResult.background_url);
        showBackgroundUpload = false;
        successToast(t('lookAndFeel.backgroundUploadedSuccess'));
      }
    } catch (err) {
      errorToast(t('lookAndFeel.failedToUploadBackground', { error: err.message || err }));
    } finally {
      uploadingBackground = false;
    }
  }
</script>

{#if loading}
  <div class="rounded-xl p-6 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <div class="animate-pulse">
      <div class="h-4 rounded w-1/4 mb-4" style="background-color: var(--ds-surface);"></div>
      <div class="h-4 rounded w-3/4" style="background-color: var(--ds-surface);"></div>
    </div>
  </div>
{:else if !canAdmin}
  <div class="rounded-xl p-8 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <div class="text-center py-8">
      <Shield class="w-12 h-12 mx-auto mb-4 text-amber-500" />
      <h2 class="text-lg font-semibold mb-2" style="color: var(--ds-text);">{t('workspaceSettings.accessDenied')}</h2>
      <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{t('workspaceSettings.accessDeniedDescription')}</p>
      <Button onclick={() => navigate(`/workspaces/${workspaceId}`)} variant="primary">
        {t('workspaceSettings.backToWorkspace')}
      </Button>
    </div>
  </div>
{:else if workspace}
  <div class="look-and-feel-wrapper" style="{backgroundStyle()}">
  <div class="p-6">
  <div class="space-y-6 max-w-4xl">
    <!-- Header -->
    <PageHeader
      icon={Palette}
      title={t('lookAndFeel.title')}
      subtitle={t('lookAndFeel.subtitle')}
      textStyle={hasCustomBackground ? 'color: white;' : ''}
      subtitleStyle={hasCustomBackground ? 'color: rgba(255, 255, 255, 0.8);' : ''}
    />

    <!-- Section 1: Display Mode (hide for personal workspaces) -->
    {#if !workspace.is_personal}
      <div class="rounded-xl p-6 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: {hasCustomBackground ? 'transparent' : 'var(--ds-border)'};">
        <h3 class="text-lg font-medium mb-2" style="color: var(--ds-text);">{t('lookAndFeel.displayModeTitle')}</h3>
        <p class="text-sm mb-6" style="color: var(--ds-text-subtle);">{t('lookAndFeel.displayModeDescription')}</p>

        <div class="flex flex-wrap gap-4">
          <!-- Default Mode Card -->
          <button
            class="mode-card p-4 rounded-lg border-2 text-left transition-all hover:shadow-md"
            class:mode-card-selected={displayMode === 'default'}
            style="background-color: var(--ds-surface-raised); border-color: {displayMode === 'default' ? 'var(--ds-brand)' : 'var(--ds-border)'}; max-width: 300px; width: 100%;"
            onclick={() => { displayMode = 'default'; debouncedSave(); }}
          >
            <div class="mb-3 rounded overflow-hidden" style="background-color: var(--ds-surface); aspect-ratio: 16/10;">
              <svg viewBox="0 0 320 200" fill="none" xmlns="http://www.w3.org/2000/svg" class="w-full h-full">
                <rect width="320" height="24" fill="var(--ds-surface-sunken, #e5e7eb)"/>
                <rect y="24" width="70" height="176" fill="var(--ds-surface-sunken, #e5e7eb)"/>
                <rect x="70" y="24" width="60" height="176" fill="var(--ds-surface-raised, #f3f4f6)"/>
                <rect x="78" y="36" width="44" height="6" rx="2" fill="var(--ds-text-subtle, #9ca3af)" opacity="0.5"/>
                <rect x="78" y="48" width="36" height="6" rx="2" fill="var(--ds-text-subtle, #9ca3af)" opacity="0.3"/>
                <rect x="78" y="60" width="40" height="6" rx="2" fill="var(--ds-text-subtle, #9ca3af)" opacity="0.3"/>
                <rect x="78" y="72" width="32" height="6" rx="2" fill="var(--ds-text-subtle, #9ca3af)" opacity="0.3"/>
                <rect x="140" y="34" width="40" height="156" rx="4" fill="var(--ds-surface-raised, #f3f4f6)"/>
                <rect x="185" y="34" width="40" height="156" rx="4" fill="var(--ds-surface-raised, #f3f4f6)"/>
                <rect x="230" y="34" width="40" height="156" rx="4" fill="var(--ds-surface-raised, #f3f4f6)"/>
                <rect x="275" y="34" width="40" height="156" rx="4" fill="var(--ds-surface-raised, #f3f4f6)"/>
                <rect x="144" y="46" width="32" height="18" rx="2" fill="var(--ds-brand, #3b82f6)" opacity="0.2"/>
                <rect x="144" y="68" width="32" height="14" rx="2" fill="var(--ds-brand, #3b82f6)" opacity="0.15"/>
                <rect x="189" y="46" width="32" height="22" rx="2" fill="var(--ds-brand, #3b82f6)" opacity="0.2"/>
                <rect x="234" y="46" width="32" height="16" rx="2" fill="var(--ds-brand, #3b82f6)" opacity="0.2"/>
              </svg>
            </div>
            <div class="font-medium text-sm" style="color: var(--ds-text);">{t('workspaceSettings.modeDefault')}</div>
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('workspaceSettings.modeDefaultDescription')}</p>
          </button>

          <!-- Board Mode Card -->
          <button
            class="mode-card p-4 rounded-lg border-2 text-left transition-all hover:shadow-md"
            class:mode-card-selected={displayMode === 'board'}
            style="background-color: var(--ds-surface-raised); border-color: {displayMode === 'board' ? 'var(--ds-brand)' : 'var(--ds-border)'}; max-width: 300px; width: 100%;"
            onclick={() => { displayMode = 'board'; debouncedSave(); }}
          >
            <div class="mb-3 rounded overflow-hidden" style="background-color: var(--ds-surface); aspect-ratio: 16/10;">
              <svg viewBox="0 0 320 200" fill="none" xmlns="http://www.w3.org/2000/svg" class="w-full h-full">
                <rect width="320" height="24" fill="var(--ds-surface-sunken, #e5e7eb)"/>
                <rect y="24" width="48" height="176" fill="var(--ds-surface-sunken, #e5e7eb)"/>
                <rect x="48" y="24" width="272" height="20" fill="var(--ds-surface-raised, #f3f4f6)"/>
                <rect x="56" y="30" width="50" height="8" rx="2" fill="var(--ds-text-subtle, #9ca3af)" opacity="0.4"/>
                <rect x="250" y="30" width="24" height="8" rx="2" fill="var(--ds-text-subtle, #9ca3af)" opacity="0.3"/>
                <rect x="56" y="50" width="58" height="140" rx="4" fill="var(--ds-surface-raised, #f3f4f6)"/>
                <rect x="120" y="50" width="58" height="140" rx="4" fill="var(--ds-surface-raised, #f3f4f6)"/>
                <rect x="184" y="50" width="58" height="140" rx="4" fill="var(--ds-surface-raised, #f3f4f6)"/>
                <rect x="248" y="50" width="58" height="140" rx="4" fill="var(--ds-surface-raised, #f3f4f6)"/>
                <rect x="62" y="62" width="46" height="20" rx="2" fill="var(--ds-brand, #3b82f6)" opacity="0.2"/>
                <rect x="62" y="86" width="46" height="16" rx="2" fill="var(--ds-brand, #3b82f6)" opacity="0.15"/>
                <rect x="62" y="106" width="46" height="18" rx="2" fill="var(--ds-brand, #3b82f6)" opacity="0.15"/>
                <rect x="126" y="62" width="46" height="24" rx="2" fill="var(--ds-brand, #3b82f6)" opacity="0.2"/>
                <rect x="126" y="90" width="46" height="16" rx="2" fill="var(--ds-brand, #3b82f6)" opacity="0.15"/>
                <rect x="190" y="62" width="46" height="18" rx="2" fill="var(--ds-brand, #3b82f6)" opacity="0.2"/>
                <rect x="254" y="62" width="46" height="20" rx="2" fill="var(--ds-brand, #3b82f6)" opacity="0.2"/>
              </svg>
            </div>
            <div class="font-medium text-sm" style="color: var(--ds-text);">{t('workspaceSettings.modeBoard')}</div>
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('workspaceSettings.modeBoardDescription')}</p>
          </button>

          <!-- ITSM Mode Card (Coming Soon) -->
          <div
            class="mode-card p-4 rounded-lg border-2 text-left opacity-50 cursor-not-allowed"
            style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); max-width: 300px; width: 100%;"
          >
            <div class="mb-3 rounded overflow-hidden" style="background-color: var(--ds-surface); aspect-ratio: 16/10;">
              <svg viewBox="0 0 320 200" fill="none" xmlns="http://www.w3.org/2000/svg" class="w-full h-full">
                <rect width="320" height="24" fill="var(--ds-surface-sunken, #e5e7eb)"/>
                <rect y="24" width="48" height="176" fill="var(--ds-surface-sunken, #e5e7eb)"/>
                <rect x="48" y="24" width="100" height="176" fill="var(--ds-surface-raised, #f3f4f6)"/>
                <rect x="56" y="36" width="84" height="8" rx="2" fill="var(--ds-text-subtle, #9ca3af)" opacity="0.4"/>
                <rect x="56" y="50" width="84" height="20" rx="3" fill="var(--ds-surface, #f9fafb)"/>
                <rect x="56" y="74" width="84" height="20" rx="3" fill="var(--ds-surface, #f9fafb)"/>
                <rect x="56" y="98" width="84" height="20" rx="3" fill="var(--ds-surface, #f9fafb)"/>
                <rect x="156" y="34" width="150" height="156" rx="4" fill="var(--ds-surface-raised, #f3f4f6)"/>
                <rect x="164" y="46" width="80" height="10" rx="2" fill="var(--ds-text-subtle, #9ca3af)" opacity="0.4"/>
                <rect x="164" y="64" width="130" height="6" rx="2" fill="var(--ds-text-subtle, #9ca3af)" opacity="0.2"/>
                <rect x="164" y="76" width="110" height="6" rx="2" fill="var(--ds-text-subtle, #9ca3af)" opacity="0.2"/>
              </svg>
            </div>
            <div class="font-medium text-sm flex items-center gap-2" style="color: var(--ds-text);">
              {t('workspaceSettings.modeItsm')}
              <span class="text-[10px] px-1.5 py-0.5 rounded-full font-medium" style="background-color: var(--ds-surface); color: var(--ds-text-subtle);">{t('workspaceSettings.modeComingSoon')}</span>
            </div>
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{t('workspaceSettings.modeItsmDescription')}</p>
          </div>
        </div>
      </div>
    {/if}

    <!-- Section 2: Background & Gradient -->
    <div class="rounded-xl p-6 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: {hasCustomBackground ? 'transparent' : 'var(--ds-border)'};">
      <h3 class="text-lg font-medium mb-2" style="color: var(--ds-text);">{t('lookAndFeel.gradientTitle')}</h3>
      <p class="text-sm mb-6" style="color: var(--ds-text-subtle);">{t('lookAndFeel.gradientDescription')}</p>

      <!-- Gradient Grid -->
      <div class="mb-6">
        <Label class="mb-3">{t('lookAndFeel.gradients')}</Label>
        <div class="grid grid-cols-10 gap-3">
          {#each gradients as gradient, index}
            <button
              onclick={() => selectGradient(index)}
              class="group relative w-[30px] h-[30px] rounded overflow-hidden transition-all hover:scale-110"
              style={selectedGradient === index && !hasBackgroundImage ? 'box-shadow: 0 0 0 2px var(--ds-border-focused); outline-offset: 2px;' : ''}
              title={gradient.name}
            >
              {#if index === 0}
                <div class="w-full h-full flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
                  <X class="w-3 h-3" style="color: var(--ds-text-subtle);" />
                </div>
              {:else}
                <div class="w-full h-full" style="background: {gradient.value};"></div>
              {/if}

              {#if selectedGradient === index && !hasBackgroundImage}
                <div class="absolute inset-0 flex items-center justify-center bg-black/20">
                  <div class="w-3 h-3 bg-white rounded-full flex items-center justify-center">
                    <svg class="w-2 h-2" style="color: var(--ds-icon-accent-blue);" fill="currentColor" viewBox="0 0 20 20">
                      <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                    </svg>
                  </div>
                </div>
              {/if}
            </button>
          {/each}
        </div>
      </div>

      <!-- Background Images -->
      <div class="border-t pt-6" style="border-color: var(--ds-border);">
        <Label class="mb-3">{t('lookAndFeel.backgroundImages')}</Label>

        <!-- Current background image preview -->
        {#if hasBackgroundImage}
          <div class="mb-4 p-3 rounded-lg border flex items-center gap-4" style="border-color: var(--ds-border-focused); background-color: var(--ds-surface);">
            <div class="w-20 h-12 rounded overflow-hidden flex-shrink-0">
              <img src={backgroundImageUrl} alt="Current background" class="w-full h-full object-cover" />
            </div>
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium" style="color: var(--ds-text);">{t('lookAndFeel.currentBackground')}</div>
              <div class="text-xs truncate" style="color: var(--ds-text-subtle);">{backgroundImageUrl}</div>
            </div>
            <Button
              variant="default"
              size="sm"
              onclick={removeBackgroundImage}
              icon={Trash2}
            >
              {t('workspaceSettings.remove')}
            </Button>
          </div>
        {/if}

        <!-- Category tabs -->
        <div class="flex gap-2 mb-4">
          {#each backgroundCategories as category}
            <button
              class="category-tab px-3 py-1.5 text-sm font-medium rounded-md transition-colors"
              class:category-tab-selected={selectedBackgroundCategory === category.id}
              onclick={() => selectedBackgroundCategory = category.id}
            >
              {category.name}
            </button>
          {/each}
        </div>

        <!-- Preset images grid -->
        <div class="grid grid-cols-4 gap-3 mb-4">
          {#each getPresetsByCategory(selectedBackgroundCategory) as preset}
            <button
              onclick={() => selectBackgroundImage(preset.url)}
              class="group relative aspect-video rounded-lg overflow-hidden transition-all hover:scale-105"
              style={backgroundImageUrl === preset.url ? 'box-shadow: 0 0 0 2px var(--ds-border-focused);' : ''}
              title={preset.name}
            >
              <img
                src={preset.thumbnail}
                alt={preset.name}
                class="w-full h-full object-cover"
                loading="lazy"
              />
              <div class="absolute inset-0 bg-black/0 group-hover:bg-black/20 transition-colors"></div>
              <div class="absolute bottom-0 left-0 right-0 p-2 bg-gradient-to-t from-black/60 to-transparent">
                <span class="text-xs text-white font-medium">{preset.name}</span>
              </div>
            </button>
          {/each}
        </div>

        <!-- Custom upload section -->
        <div class="border-t pt-4" style="border-color: var(--ds-border);">
          <div class="flex items-center gap-3">
            <Button
              variant="default"
              size="sm"
              onclick={() => showBackgroundUpload = !showBackgroundUpload}
              icon={Upload}
              disabled={!attachmentStatus.enabled}
            >
              {t('lookAndFeel.uploadCustomImage')}
            </Button>
            {#if !attachmentStatus.enabled}
              <span class="text-xs" style="color: var(--ds-text-warning);">
                {t('workspaceSettings.attachmentsRequired')}
              </span>
            {/if}
          </div>

          {#if showBackgroundUpload && attachmentStatus.enabled}
            <div class="mt-3 p-4 rounded-lg border" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
              <input
                type="file"
                accept="image/*"
                onchange={(e) => handleBackgroundUpload(e.target.files)}
                disabled={uploadingBackground}
                class="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100 disabled:opacity-50"
              />
              {#if uploadingBackground}
                <div class="mt-2 text-sm text-blue-600">{t('workspaceSettings.uploading')}</div>
              {/if}
              <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
                {t('lookAndFeel.backgroundUploadRecommendation')}
              </p>
            </div>
          {/if}
        </div>
      </div>
    </div>

    <!-- Section 3: Workspace Identity -->
    <div class="rounded-xl p-6 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: {hasCustomBackground ? 'transparent' : 'var(--ds-border)'};">
      <h3 class="text-lg font-medium mb-2" style="color: var(--ds-text);">{t('lookAndFeel.identityTitle')}</h3>
      <p class="text-sm mb-6" style="color: var(--ds-text-subtle);">{t('lookAndFeel.identityDescription')}</p>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-8">
        <!-- Icon and Color Selection -->
        <div>
          <IconSelector
            selectedIcon={icon}
            selectedColor={color}
            label={t('workspaceSettings.workspaceIconColor')}
            compact={true}
            onchange={handleIconChange}
          />
        </div>

        <!-- Avatar Upload -->
        <div>
          <Label class="mb-2">{t('workspaceSettings.workspaceAvatar')}</Label>

          <div class="space-y-4">
            {#if avatarUrl}
              <div class="flex items-center gap-4 p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                <img src={avatarUrl} alt="Workspace avatar" class="w-16 h-16 rounded object-cover" />
                <div class="flex-1">
                  <div class="text-sm font-medium" style="color: var(--ds-text);">{t('workspaceSettings.customAvatar')}</div>
                  <div class="text-xs" style="color: var(--ds-text-subtle);">{t('workspaceSettings.imageUploadedSuccessfully')}</div>
                </div>
                <Button
                  variant="default"
                  size="sm"
                  onclick={removeAvatar}
                  icon={Trash2}
                >
                  {t('workspaceSettings.remove')}
                </Button>
              </div>
            {:else}
              <div class="flex items-center gap-4 p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                <div class="w-16 h-16 rounded flex items-center justify-center" style="background-color: {color};">
                  <svelte:component this={iconMap[icon] || Package} size={32} color="white" />
                </div>
                <div class="flex-1">
                  <div class="text-sm font-medium" style="color: var(--ds-text);">{t('workspaceSettings.defaultIcon')}</div>
                  <div class="text-xs" style="color: var(--ds-text-subtle);">{t('workspaceSettings.usingSelectedIconColor')}</div>
                </div>
              </div>
            {/if}

            <div>
              <Button
                variant="default"
                size="sm"
                onclick={() => showAvatarUpload = !showAvatarUpload}
                icon={Camera}
                disabled={!attachmentStatus.enabled}
              >
                {avatarUrl ? t('workspaceSettings.changeAvatar') : t('workspaceSettings.uploadAvatar')}
              </Button>
              {#if !attachmentStatus.enabled}
                <p class="text-xs mt-1" style="color: var(--ds-text-warning);">
                  {t('workspaceSettings.attachmentsRequired')}
                </p>
              {/if}
            </div>

            {#if showAvatarUpload && attachmentStatus.enabled}
              <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                <input
                  type="file"
                  accept="image/*"
                  onchange={(e) => handleAvatarUpload(e.target.files)}
                  disabled={uploadingAvatar}
                  class="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100 disabled:opacity-50"
                />
                {#if uploadingAvatar}
                  <div class="mt-2 text-sm text-blue-600">{t('workspaceSettings.uploading')}</div>
                {/if}
                <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
                  {t('workspaceSettings.uploadRecommendation')}
                </p>
              </div>
            {/if}
          </div>

          <p class="text-xs mt-3" style="color: var(--ds-text-subtle);">
            {t('workspaceSettings.avatarOrIconNote')}
          </p>
        </div>
      </div>
    </div>

  </div>
  </div>
  </div>
{:else}
  <div class="rounded-xl p-6 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <p class="text-center" style="color: var(--ds-text-subtle);">{t('workspaceSettings.workspaceNotFound')}</p>
  </div>
{/if}

<style>
  .look-and-feel-wrapper {
    width: 100%;
    min-height: 100%;
    position: relative;
    background-size: 200% 200%;
    animation: gradient-shift 15s ease infinite;
  }

  @media (prefers-reduced-motion: reduce) {
    .look-and-feel-wrapper {
      animation: none;
    }
  }

  .mode-card-selected {
    box-shadow: 0 0 0 1px var(--ds-brand);
  }

  .category-tab {
    background-color: var(--ds-surface);
    color: var(--ds-text-subtle);
    border: 1px solid var(--ds-border);
  }

  .category-tab:hover {
    background-color: var(--ds-surface-hovered);
    color: var(--ds-text);
  }

  .category-tab-selected {
    background-color: var(--ds-interactive) !important;
    color: white !important;
    border-color: var(--ds-interactive) !important;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
  }
</style>
