<script>
  import {
    Palette, Navigation, X, TextCursorInput, Check,
    Plus, Trash2, GripVertical, Settings, ExternalLink
  } from 'lucide-svelte';
  import { hubStore, gradients } from '../stores/hub.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { navigate } from '../router.js';
  import LogoUploader from '../components/LogoUploader.svelte';

  let showCustomizePanelHover = $state(false);
</script>

<!-- Customization Panel Overlay (hide when editing sections so sections are visible) -->
{#if hubStore.showCustomizePanel && hubStore.activeSection !== 'sections'}
  <div
    class="fixed inset-0 bg-black/30 z-50 transition-opacity"
    onclick={() => hubStore.showCustomizePanel = false}
  ></div>
{/if}

<!-- Customization Slide-in Panel -->
{#if hubStore.showCustomizePanel}
  <div
    class="fixed right-0 top-0 bottom-0 w-80 shadow-2xl z-50 flex flex-col overflow-hidden"
    style="background-color: var(--ds-surface);"
    onmouseenter={() => showCustomizePanelHover = true}
    onmouseleave={() => showCustomizePanelHover = false}
  >
    <!-- Panel Header -->
    <div class="p-4 border-b flex items-center justify-between" style="border-color: var(--ds-border);">
      <h2 class="font-semibold" style="color: var(--ds-text);">{t('hub.customizeHub', 'Customize Hub')}</h2>
      <button
        onclick={() => hubStore.showCustomizePanel = false}
        class="p-1 rounded hover:bg-black/5 transition-colors"
      >
        <X class="w-5 h-5" style="color: var(--ds-text-subtle);" />
      </button>
    </div>

    <!-- Section Navigation -->
    <div class="border-b flex" style="border-color: var(--ds-border);">
      <button
        onclick={() => hubStore.activeSection = 'hero-gradient'}
        class="flex-1 p-3 text-sm font-medium flex items-center justify-center gap-2 border-b-2 transition-colors"
        style="border-color: {hubStore.activeSection === 'hero-gradient' ? (hubStore.isDarkMode ? '#60a5fa' : '#2563eb') : 'transparent'}; color: {hubStore.activeSection === 'hero-gradient' ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
      >
        <Palette class="w-4 h-4" />
        <span>{t('portal.theme', 'Theme')}</span>
      </button>
      <button
        onclick={() => { hubStore.activeSection = 'sections'; hubStore.isEditing = true; }}
        class="flex-1 p-3 text-sm font-medium flex items-center justify-center gap-2 border-b-2 transition-colors"
        style="border-color: {hubStore.activeSection === 'sections' ? (hubStore.isDarkMode ? '#60a5fa' : '#2563eb') : 'transparent'}; color: {hubStore.activeSection === 'sections' ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
      >
        <Navigation class="w-4 h-4" />
        <span>{t('hub.sections', 'Sections')}</span>
      </button>
    </div>

    <!-- Panel Content -->
    <div class="flex-1 overflow-y-auto p-4">
      <!-- Hero/Gradient Section -->
      {#if hubStore.activeSection === 'hero-gradient'}
        <div class="space-y-6">
          <!-- Title & Description -->
          <div>
            <label class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
              <TextCursorInput class="w-4 h-4 inline mr-1" />
              {t('hub.heroContent', 'Hero Content')}
            </label>
            <input
              type="text"
              value={hubStore.editableTitle}
              oninput={(e) => { hubStore.editableTitle = e.target.value; hubStore.saveCustomizations(); }}
              class="w-full px-3 py-2 rounded border text-sm mb-2"
              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); color: var(--ds-text);"
              placeholder="Hub title"
            />
            <textarea
              value={hubStore.editableDescription}
              oninput={(e) => { hubStore.editableDescription = e.target.value; hubStore.saveCustomizations(); }}
              class="w-full px-3 py-2 rounded border text-sm resize-none"
              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); color: var(--ds-text);"
              placeholder="Hub description (optional)"
              rows="2"
            ></textarea>
          </div>

          <!-- Gradient Selection -->
          <div>
            <label class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
              <Palette class="w-4 h-4 inline mr-1" />
              {t('portal.gradient', 'Gradient')}
            </label>
            <div class="grid grid-cols-3 gap-2">
              {#each gradients as gradient, index}
                <button
                  onclick={() => hubStore.selectGradient(index)}
                  class="relative h-12 rounded-lg transition-all overflow-hidden group"
                  style="background: {gradient.value};"
                  title={gradient.name}
                >
                  {#if hubStore.isDarkMode}
                    <div class="absolute inset-0 bg-black/40 group-hover:bg-black/30 transition-colors"></div>
                  {/if}
                  {#if hubStore.selectedGradient === index}
                    <div class="absolute inset-0 flex items-center justify-center">
                      <div class="w-6 h-6 rounded-full bg-white flex items-center justify-center shadow-lg">
                        <Check class="w-4 h-4 text-gray-800" />
                      </div>
                    </div>
                  {/if}
                </button>
              {/each}
            </div>
          </div>

          <!-- Search Box Customization -->
          <div>
            <label class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
              {t('portal.searchBox', 'Search Box')}
            </label>
            <input
              type="text"
              value={hubStore.editableSearchPlaceholder}
              oninput={(e) => { hubStore.editableSearchPlaceholder = e.target.value; hubStore.saveCustomizations(); }}
              class="w-full px-3 py-2 rounded border text-sm mb-2"
              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); color: var(--ds-text);"
              placeholder="Search placeholder"
            />
            <input
              type="text"
              value={hubStore.editableSearchHint}
              oninput={(e) => { hubStore.editableSearchHint = e.target.value; hubStore.saveCustomizations(); }}
              class="w-full px-3 py-2 rounded border text-sm"
              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); color: var(--ds-text);"
              placeholder="Search hint text"
            />
          </div>

          <!-- Logo Upload -->
          <div class="border-t pt-6" style="border-color: var(--ds-border);">
            <LogoUploader
              currentLogoUrl={hubStore.logoUrl}
              onUpload={(files) => hubStore.handleLogoUpload(files)}
              onRemove={() => hubStore.removeLogo()}
              uploading={hubStore.uploadingLogo}
              label={t('hub.logo', 'Hub Logo')}
              helpText={t('hub.logoHelp', 'This logo will be displayed on the Hub and used as a fallback for portals without their own logo.')}
            />
          </div>
        </div>
      {/if}

      <!-- Sections Configuration -->
      {#if hubStore.activeSection === 'sections'}
        <div class="space-y-4">
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            {t('hub.sectionsHelp', 'Organize portals into sections. Drag portals between sections on the main view.')}
          </p>

          <!-- Current Sections -->
          {#each hubStore.hubSections as section, index (section.id)}
            <div
              class="p-3 rounded border"
              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
            >
              <div class="flex items-center justify-between mb-2">
                <input
                  type="text"
                  value={section.title}
                  oninput={(e) => hubStore.updateSection(section.id, 'title', e.target.value)}
                  class="flex-1 text-sm font-medium bg-transparent focus:outline-none"
                  style="color: var(--ds-text);"
                  placeholder="Section title"
                />
                <button
                  onclick={() => hubStore.deleteSection(section.id)}
                  class="p-1 rounded hover:bg-red-100 text-red-600"
                >
                  <Trash2 class="w-4 h-4" />
                </button>
              </div>
              <div class="text-xs" style="color: var(--ds-text-subtle);">
                {section.portal_ids.length} portal{section.portal_ids.length !== 1 ? 's' : ''}
              </div>
            </div>
          {/each}

          <!-- Add Section Button -->
          <button
            onclick={() => hubStore.addSection()}
            class="w-full p-3 rounded border-2 border-dashed flex items-center justify-center gap-2 transition-all hover:border-solid"
            style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
          >
            <Plus class="w-4 h-4" />
            <span class="text-sm">{t('hub.addSection', 'Add Section')}</span>
          </button>

          <!-- Exit Edit Mode -->
          <button
            onclick={() => { hubStore.isEditing = false; hubStore.activeSection = 'hero-gradient'; }}
            class="w-full p-2 rounded text-sm font-medium transition-colors"
            style="background-color: {hubStore.isDarkMode ? '#1e40af' : '#2563eb'}; color: white;"
          >
            {t('common.done', 'Done')}
          </button>
        </div>
      {/if}
    </div>

    <!-- Panel Footer -->
    <div class="p-4 border-t" style="border-color: var(--ds-border);">
      <!-- Manage Channels Link -->
      <a
        href="/admin/channels"
        class="flex items-center gap-2 text-sm transition-colors hover:opacity-80"
        style="color: {hubStore.isDarkMode ? '#60a5fa' : '#2563eb'};"
      >
        <Settings class="w-4 h-4" />
        <span>{t('hub.manageChannels', 'Manage Channels')}</span>
        <ExternalLink class="w-3 h-3 ml-auto" />
      </a>
    </div>
  </div>
{/if}
