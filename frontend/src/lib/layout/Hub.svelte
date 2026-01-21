<script>
  import { onMount, onDestroy } from 'svelte';
  import { currentRoute } from '../router.js';
  import { AlertCircle, ArrowLeft, Sun, Moon } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Components
  import Spinner from '../components/Spinner.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import HubHero from '../hub/HubHero.svelte';
  import HubSections from '../hub/HubSections.svelte';
  import HubCustomizePanel from '../hub/HubCustomizePanel.svelte';
  import HubInbox from '../hub/HubInbox.svelte';

  // Store
  import { hubStore, gradients } from '../stores/hub.svelte.js';

  onMount(async () => {
    // Load hub data
    await hubStore.loadHub();
  });

  onDestroy(() => {
    hubStore.reset();
  });

  // Handle ESC key to close customize panel
  function handleKeydown(event) {
    if (event.key === 'Escape') {
      if (hubStore.showCustomizePanel) {
        hubStore.showCustomizePanel = false;
      }
    }
  }
</script>

<!-- Global keydown listener for ESC key -->
<svelte:window onkeydown={handleKeydown} />

<!-- Hub Page - Embedded in MainApp -->
<div class="flex flex-col min-h-screen">
  {#if hubStore.loading}
    <!-- Loading State -->
    <div class="flex items-center justify-center py-16">
      <div class="text-center">
        <Spinner size="lg" class="mx-auto mb-4" />
        <p style="color: var(--ds-text-subtle);">{t('hub.loading', 'Loading hub...')}</p>
      </div>
    </div>
  {:else if hubStore.error}
    <!-- Error State -->
    <div class="flex items-center justify-center px-4 py-16">
      <EmptyState
        icon={AlertCircle}
        title={t('hub.error', 'Error loading hub')}
        description={hubStore.error}
      />
    </div>
  {:else}
    <!-- Main Content Wrapper -->
    <div class="flex flex-col flex-1">
      <!-- Header (shows in inbox view) -->
      {#if hubStore.showInbox}
        <div class="hero-gradient border-b border-white/20" style="background: {gradients[hubStore.selectedGradient].value};">
          <div class="hero-content max-w-7xl mx-auto px-6 py-3">
            <div class="flex items-center justify-between">
              <!-- Left: Hub Name -->
              <div class="flex items-center gap-3">
                <h1 class="text-lg font-semibold text-white">
                  {hubStore.editableTitle || 'Portal Hub'}
                </h1>
              </div>

              <!-- Right: Back to Hub Button -->
              <button
                onclick={() => hubStore.toggleInbox()}
                class="flex items-center gap-2 px-3 py-1.5 rounded text-white text-sm bg-white/10 backdrop-blur-sm hover:bg-white/20 transition-all border border-white/20"
              >
                <ArrowLeft class="w-4 h-4" />
                <span class="font-medium">{t('hub.backToHub', 'Back to Hub')}</span>
              </button>
            </div>
          </div>
        </div>
      {:else}
        <!-- Hero Section (shown in normal hub view) -->
        <HubHero />
      {/if}

      <!-- Content Area Below Hero -->
      <div class="flex-1" style="background-color: var(--ds-surface-raised);">
        <div class="max-w-6xl mx-auto px-6 py-8">
          {#if hubStore.showInbox}
            <HubInbox />
          {:else}
            <HubSections />
          {/if}
        </div>
      </div>
    </div>

    <!-- Customization Panel -->
    <HubCustomizePanel />
  {/if}
</div>

<style>
  /* Hero gradient background */
  .hero-gradient {
    width: 100%;
    position: relative;
  }

  .hero-gradient::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background-image:
      radial-gradient(circle at 20% 50%, rgba(255, 255, 255, 0.1) 0%, transparent 50%),
      radial-gradient(circle at 80% 80%, rgba(255, 255, 255, 0.1) 0%, transparent 50%);
    pointer-events: none;
  }

  .hero-content {
    position: relative;
    z-index: 1;
  }
</style>
