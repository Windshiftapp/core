<script>
  import Palette from '@lucide/svelte/icons/palette';
  import Navigation from '@lucide/svelte/icons/navigation';
  import X from '@lucide/svelte/icons/x';
  import TextCursorInput from '@lucide/svelte/icons/text-cursor-input';
  import Check from '@lucide/svelte/icons/check';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import GripVertical from '@lucide/svelte/icons/grip-vertical';
  import Settings from '@lucide/svelte/icons/settings';
  import ExternalLink from '@lucide/svelte/icons/external-link';
  import { hubStore, gradients } from '../stores/hub.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import LogoUploader from '../components/LogoUploader.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import TabStrip from '../components/TabStrip.svelte';

  let showCustomizePanelHover = $state(false);

  function handleSectionChange(section) {
    hubStore.activeSection = section;
    if (section === 'sections') {
      hubStore.isEditing = true;
    }
  }
</script>

<!-- Customization Panel Overlay (hide when editing sections so sections are visible) -->
<ModalBackdrop
  show={hubStore.showCustomizePanel && hubStore.activeSection !== 'sections'}
  opacity={0.3}
  blur={0}
  align="none"
  onclose={() => hubStore.showCustomizePanel = false}
/>

<!-- Customization Slide-in Panel -->
{#if hubStore.showCustomizePanel}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
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
    <TabStrip
      tabs={[
        { id: 'hero-gradient', label: t('portal.theme', 'Theme'), icon: Palette },
        { id: 'sections', label: t('hub.sections', 'Sections'), icon: Navigation }
      ]}
      activeTab={hubStore.activeSection}
      onTabChange={handleSectionChange}
      fill
    />

    <!-- Panel Content -->
    <div class="flex-1 overflow-y-auto p-4">
      <!-- Hero/Gradient Section -->
      {#if hubStore.activeSection === 'hero-gradient'}
        <div class="space-y-6">
          <!-- Title & Description -->
          <div>
            <span class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
              <TextCursorInput class="w-4 h-4 inline mr-1" />
              {t('hub.heroContent', 'Hero Content')}
            </span>
            <Input
              type="text"
              value={hubStore.editableTitle}
              oninput={(e) => { hubStore.editableTitle = e.currentTarget.value; hubStore.saveCustomizations(); }}
              class="mb-2"
              placeholder="Hub title"
            />
            <Textarea
              value={hubStore.editableDescription}
              oninput={(e) => { hubStore.editableDescription = e.currentTarget.value; hubStore.saveCustomizations(); }}
              placeholder="Hub description (optional)"
              rows={2}
              size="small"
            />
          </div>

          <!-- Gradient Selection -->
          <div>
            <span class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
              <Palette class="w-4 h-4 inline mr-1" />
              {t('portal.gradient', 'Gradient')}
            </span>
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
                        <Check class="w-4 h-4 text-ds-text" />
                      </div>
                    </div>
                  {/if}
                </button>
              {/each}
            </div>
          </div>

          <!-- Search Box Customization -->
          <div>
            <label for="hub-search-box" class="block text-sm font-medium mb-2" style="color: var(--ds-text);">
              {t('portal.searchBox', 'Search Box')}
            </label>
            <Input
              id="hub-search-box"
              type="text"
              value={hubStore.editableSearchPlaceholder}
              oninput={(e) => { hubStore.editableSearchPlaceholder = e.currentTarget.value; hubStore.saveCustomizations(); }}
              class="mb-2"
              placeholder="Search placeholder"
            />
            <Input
              type="text"
              value={hubStore.editableSearchHint}
              oninput={(e) => { hubStore.editableSearchHint = e.currentTarget.value; hubStore.saveCustomizations(); }}
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
                <Input
                  type="text"
                  value={section.title}
                  oninput={(e) => hubStore.updateSection(section.id, 'title', e.currentTarget.value)}
                  class="flex-1 font-medium"
                  size="small"
                  placeholder="Section title"
                />
                <button
                  onclick={() => hubStore.deleteSection(section.id)}
                  class="p-1 rounded hover:bg-ds-danger-subtle text-ds-text-danger"
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
            style="background-color: var(--ds-interactive); color: white;"
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
        style="color: var(--ds-text-link);"
      >
        <Settings class="w-4 h-4" />
        <span>{t('hub.manageChannels', 'Manage Channels')}</span>
        <ExternalLink class="w-3 h-3 ml-auto" />
      </a>
    </div>
  </div>
{/if}
