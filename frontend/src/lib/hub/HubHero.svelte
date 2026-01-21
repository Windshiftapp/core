<script>
  import { Search, Palette, Inbox } from 'lucide-svelte';
  import { hubStore, gradients } from '../stores/hub.svelte.js';
  import { authStore, permissionStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';

  let searchQuery = $state('');

  function handleSearch(e) {
    e.preventDefault();
    // Filter portals based on search query
    // This is client-side filtering for simplicity
  }
</script>

<!-- Hero Section with Gradient -->
<div class="hero-gradient" style="background: {gradients[hubStore.selectedGradient].value};">
  <!-- Top Navigation Bar -->
  <div class="hero-content">
    <div class="max-w-7xl mx-auto px-6 py-3">
      <div class="flex items-center justify-end">
        <!-- Right side: Actions -->
        <div class="flex items-center gap-2">
          <!-- Inbox Button -->
          <button
            onclick={() => hubStore.toggleInbox()}
            class="flex items-center gap-2 px-3 py-1.5 rounded text-white text-sm bg-white/10 backdrop-blur-sm hover:bg-white/20 transition-all border border-white/20"
            title={t('hub.inbox', 'Inbox')}
          >
            <Inbox class="w-4 h-4" />
            <span class="font-medium">{t('hub.inbox', 'Inbox')}</span>
          </button>

          <!-- Edit/Customize Button (Admin only) -->
          {#if authStore.isAuthenticated && $permissionStore.isSystemAdmin}
            <button
              onclick={() => hubStore.showCustomizePanel = !hubStore.showCustomizePanel}
              class="flex items-center gap-2 px-3 py-1.5 rounded text-white text-sm bg-white/10 backdrop-blur-sm hover:bg-white/20 transition-all border border-white/20"
              title={t('portal.customize', 'Customize')}
            >
              <Palette class="w-4 h-4" />
              <span class="font-medium">{t('portal.customize', 'Customize')}</span>
            </button>
          {/if}
        </div>
      </div>
    </div>

    <!-- Main Hero Content -->
    <div class="max-w-4xl mx-auto px-6 py-8 text-center">
      <!-- Hub Title -->
      {#if hubStore.isEditing}
        <input
          type="text"
          value={hubStore.editableTitle}
          oninput={(e) => hubStore.editableTitle = e.target.value}
          class="text-3xl font-bold mb-4 text-white bg-transparent text-center w-full focus:outline-none"
          placeholder="Hub Title"
        />
      {:else}
        <h1 class="text-3xl font-bold mb-4 text-white">
          {hubStore.editableTitle || 'Portal Hub'}
        </h1>
      {/if}

      <!-- Hub Description -->
      {#if hubStore.isEditing}
        <textarea
          value={hubStore.editableDescription}
          oninput={(e) => hubStore.editableDescription = e.target.value}
          class="text-base text-white/90 mb-6 max-w-3xl mx-auto bg-transparent text-center w-full focus:outline-none resize-none"
          placeholder="Hub description (optional)"
          rows="2"
        ></textarea>
      {:else if hubStore.editableDescription}
        <p class="text-base text-white/90 mb-6 max-w-3xl mx-auto">
          {hubStore.editableDescription}
        </p>
      {/if}

      <!-- Search Box -->
      <div class="max-w-xl mx-auto relative">
        <form onsubmit={handleSearch} class="relative">
          <div class="relative">
            <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
              <Search class="h-5 w-5 text-gray-400" />
            </div>
            <input
              type="text"
              bind:value={searchQuery}
              placeholder={hubStore.editableSearchPlaceholder || 'Search portals...'}
              class="block w-full pl-10 pr-4 py-2.5 text-sm border-0 rounded-lg focus:outline-none focus:ring-2 focus:ring-white/30 transition-all shadow-lg"
              style="background-color: rgba(255, 255, 255, 0.95); color: #111827;"
            />
          </div>
        </form>

        <!-- Search hint / Editable fields -->
        {#if hubStore.isEditing}
          <div class="mt-3 space-y-2">
            <div>
              <label class="text-xs text-white/60 block mb-1">Search Box Placeholder:</label>
              <input
                type="text"
                value={hubStore.editableSearchPlaceholder}
                oninput={(e) => hubStore.editableSearchPlaceholder = e.target.value}
                class="text-sm text-white bg-transparent text-center focus:outline-none w-full"
                placeholder="Search placeholder text"
              />
            </div>
            <div>
              <label class="text-xs text-white/60 block mb-1">Search Hint Text:</label>
              <input
                type="text"
                value={hubStore.editableSearchHint}
                oninput={(e) => hubStore.editableSearchHint = e.target.value}
                class="text-sm text-white bg-transparent text-center focus:outline-none w-full"
                placeholder="Search hint text"
              />
            </div>
          </div>
        {:else if hubStore.editableSearchHint}
          <p class="mt-3 text-xs text-white/80">
            {hubStore.editableSearchHint}
          </p>
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  /* Hero gradient background */
  .hero-gradient {
    width: 100%;
    position: relative;
  }

  /* Add subtle pattern overlay for depth */
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

  /* Ensure content appears above the gradient overlay */
  .hero-content {
    position: relative;
    z-index: 1;
  }
</style>
