<script>
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import { homepageStore } from '../stores/homepageStore.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import Button from '../components/Button.svelte';
  import GradientPicker from '../components/GradientPicker.svelte';
  import BackgroundPresetGrid from '../components/BackgroundPresetGrid.svelte';
  import Label from '../components/Label.svelte';
</script>

<div class="space-y-6">
  <div>
    <h3 class="text-sm font-medium mb-1" style="color: var(--ds-text);">{t('lookAndFeel.gradientTitle')}</h3>
    <p class="text-xs mb-4" style="color: var(--ds-text-subtle);">{t('lookAndFeel.gradientDescription')}</p>
    <GradientPicker
      selectedGradient={homepageStore.gradient}
      hasBackgroundImage={homepageStore.hasBackgroundImage}
      onSelect={(index) => homepageStore.selectGradient(index)}
      testIdPrefix="dashboard-gradient-swatch"
    />
  </div>

  <div class="border-t pt-6" style="border-color: var(--ds-border);">
    <Label class="mb-4">{t('lookAndFeel.backgroundImages')}</Label>

    {#if homepageStore.hasBackgroundImage}
      <div class="mb-4 rounded-lg border overflow-hidden" style="border-color: var(--ds-border-focused);">
        <img
          src={homepageStore.backgroundImageUrl}
          alt={t('lookAndFeel.currentBackground')}
          class="w-full object-cover"
          style="aspect-ratio: 16 / 9;"
        />
        <div class="flex items-center justify-between gap-3 p-3" style="background-color: var(--ds-surface);">
          <div class="text-xs font-medium" style="color: var(--ds-text);">
            {t('lookAndFeel.currentBackground')}
          </div>
          <Button
            variant="default"
            size="sm"
            onclick={() => homepageStore.removeBackgroundImage()}
            icon={Trash2}
            dataTestid="dashboard-background-remove"
          >
            {t('workspaceSettings.remove')}
          </Button>
        </div>
      </div>
    {/if}

    <BackgroundPresetGrid
      currentImageUrl={homepageStore.backgroundImageUrl}
      onSelectImage={(url) => homepageStore.selectBackgroundImage(url)}
      presetAspectRatio="16 / 9"
      columns={2}
      testIdPrefix="dashboard-background-preset"
    />
  </div>
</div>
