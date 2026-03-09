<script>
  import { Search, Palette, Inbox, FileText, ExternalLink } from 'lucide-svelte';
  import { hubStore, gradients, iconMap } from '../stores/hub.svelte.js';
  import { authStore, permissionStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';

  // Track if input is focused
  let inputFocused = $state(false);

  // Search results computed from search query
  const searchResults = $derived.by(() => {
    const query = hubStore.searchQuery?.toLowerCase().trim();
    if (!query) return null;

    const matchedPortals = hubStore.portals.filter(p =>
      p.name?.toLowerCase().includes(query) ||
      p.description?.toLowerCase().includes(query)
    );

    const matchedRequestTypes = [];
    for (const portal of hubStore.portals) {
      for (const rt of (portal.request_types || [])) {
        if (rt.name?.toLowerCase().includes(query) ||
            rt.description?.toLowerCase().includes(query)) {
          matchedRequestTypes.push({ ...rt, portal_slug: portal.slug, portal_name: portal.name, portal_gradient: portal.gradient });
        }
      }
    }

    return { portals: matchedPortals, requestTypes: matchedRequestTypes };
  });

  const hasResults = $derived(searchResults && (searchResults.portals.length > 0 || searchResults.requestTypes.length > 0));
  const showPopover = $derived(hubStore.searchQuery?.trim() && inputFocused);

  function navigateToPortal(portal) {
    window.location.href = `/portal/${portal.slug}`;
  }

  function navigateToRequestType(rt) {
    window.location.href = `/portal/${rt.portal_slug}?request-type=${rt.id}`;
  }

  function handleInputFocus() {
    inputFocused = true;
  }

  function handleInputBlur(e) {
    // Delay blur to allow click on results
    setTimeout(() => {
      inputFocused = false;
    }, 200);
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
            class="glass-btn flex items-center gap-2 px-3 py-1.5 rounded text-white text-sm transition-all"
            title={t('hub.inbox', 'Inbox')}
          >
            <Inbox class="w-4 h-4" />
            <span class="font-medium">{t('hub.inbox', 'Inbox')}</span>
          </button>

          <!-- Edit/Customize Button (Admin only) -->
          {#if authStore.isAuthenticated && $permissionStore.isSystemAdmin}
            <button
              onclick={() => hubStore.showCustomizePanel = !hubStore.showCustomizePanel}
              class="glass-btn flex items-center gap-2 px-3 py-1.5 rounded text-white text-sm transition-all"
              title={t('portal.customizeButton')}
            >
              <Palette class="w-4 h-4" />
              <span class="font-medium">{t('portal.customizeButton')}</span>
            </button>
          {/if}
        </div>
      </div>
    </div>

    <!-- Main Hero Content -->
    <div class="max-w-4xl mx-auto px-6 py-8 text-center">
      <!-- Hub Logo -->
      {#if hubStore.logoUrl}
        <div class="mb-6 flex justify-center">
          <img
            src={hubStore.logoUrl}
            alt="Hub logo"
            class="hub-logo"
            style="max-height: 60px; max-width: 250px; object-fit: contain;"
          />
        </div>
      {/if}

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
        <div class="relative">
          <div class="relative">
            <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
              <Search class="h-5 w-5 text-gray-400" />
            </div>
            <input
              type="text"
              bind:value={hubStore.searchQuery}
              placeholder={hubStore.editableSearchPlaceholder || 'Search portals...'}
              class="block w-full pl-10 pr-4 py-2.5 text-sm border-0 rounded-lg focus:outline-none focus:ring-2 focus:ring-white/30 transition-all shadow-lg"
              style="background-color: rgba(255, 255, 255, 0.95); color: #111827;"
              onfocus={handleInputFocus}
              onblur={handleInputBlur}
            />
          </div>

          <!-- Search Results Popover -->
          {#if showPopover}
            <div class="search-popover absolute left-0 right-0 top-full mt-2 rounded-lg shadow-2xl overflow-hidden z-50" style="background-color: var(--ds-surface-card, #fff);">
              {#if hasResults}
                <div class="max-h-80 overflow-y-auto">
                  <!-- Matched Portals -->
                  {#if searchResults.portals.length > 0}
                    <div class="px-3 py-2 border-b" style="border-color: var(--ds-border, #e5e7eb);">
                      <span class="text-xs font-medium uppercase" style="color: var(--ds-text-subtle, #6b7280);">
                        {t('hub.matchingPortals')} ({searchResults.portals.length})
                      </span>
                    </div>
                    {#each searchResults.portals as portal (portal.id)}
                      <button
                        class="w-full px-3 py-2 flex items-center gap-3 text-left hover:bg-black/5 transition-colors"
                        onclick={() => navigateToPortal(portal)}
                      >
                        <div class="w-8 h-8 rounded flex-shrink-0" style="background: {gradients[portal.gradient || 0].value};"></div>
                        <div class="flex-1 min-w-0">
                          <div class="font-medium text-sm truncate" style="color: var(--ds-text, #111827);">{portal.name}</div>
                          {#if portal.description}
                            <div class="text-xs truncate" style="color: var(--ds-text-subtle, #6b7280);">{portal.description}</div>
                          {/if}
                        </div>
                        <ExternalLink class="w-4 h-4 flex-shrink-0 opacity-50" style="color: var(--ds-text-subtle, #6b7280);" />
                      </button>
                    {/each}
                  {/if}

                  <!-- Matched Request Types -->
                  {#if searchResults.requestTypes.length > 0}
                    <div class="px-3 py-2 border-b" style="border-color: var(--ds-border, #e5e7eb);">
                      <span class="text-xs font-medium uppercase" style="color: var(--ds-text-subtle, #6b7280);">
                        {t('hub.matchingRequestTypes')} ({searchResults.requestTypes.length})
                      </span>
                    </div>
                    {#each searchResults.requestTypes as rt (rt.id)}
                      <button
                        class="w-full px-3 py-2 flex items-center gap-3 text-left hover:bg-black/5 transition-colors"
                        onclick={() => navigateToRequestType(rt)}
                      >
                        <div class="w-8 h-8 rounded flex items-center justify-center flex-shrink-0" style="background-color: {rt.color || '#6b7280'};">
                          {#if rt.icon && iconMap[rt.icon]}
                            <svelte:component this={iconMap[rt.icon]} class="w-4 h-4 text-white" />
                          {:else}
                            <FileText class="w-4 h-4 text-white" />
                          {/if}
                        </div>
                        <div class="flex-1 min-w-0">
                          <div class="font-medium text-sm truncate" style="color: var(--ds-text, #111827);">{rt.name}</div>
                          <div class="text-xs truncate" style="color: var(--ds-text-subtle, #6b7280);">{rt.portal_name}</div>
                        </div>
                        <ExternalLink class="w-4 h-4 flex-shrink-0 opacity-50" style="color: var(--ds-text-subtle, #6b7280);" />
                      </button>
                    {/each}
                  {/if}
                </div>
              {:else}
                <div class="px-4 py-6 text-center">
                  <p class="text-sm" style="color: var(--ds-text-subtle, #6b7280);">
                    {t('hub.noSearchResults')}
                  </p>
                </div>
              {/if}
            </div>
          {/if}
        </div>

        <!-- Search hint / Editable fields -->
        {#if hubStore.isEditing}
          <div class="mt-3 space-y-2">
            <div>
              <label for="hub-search-placeholder" class="text-xs text-white/60 block mb-1">Search Box Placeholder:</label>
              <input
                id="hub-search-placeholder"
                type="text"
                value={hubStore.editableSearchPlaceholder}
                oninput={(e) => hubStore.editableSearchPlaceholder = e.target.value}
                class="text-sm text-white bg-transparent text-center focus:outline-none w-full"
                placeholder="Search placeholder text"
              />
            </div>
            <div>
              <label for="hub-search-hint" class="text-xs text-white/60 block mb-1">Search Hint Text:</label>
              <input
                id="hub-search-hint"
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

  /* Search results popover */
  .search-popover {
    border: 1px solid var(--ds-border, #e5e7eb);
  }
</style>
