<script>
  import { Search, X, BookOpen } from 'lucide-svelte';
  import Spinner from '../components/Spinner.svelte';
  import { portalStore, gradients } from '../stores/portal.svelte.js';

  function handleSearch(e) {
    e.preventDefault();
    portalStore.performSearch();
  }

  function handleSearchKeydown(e) {
    if (e.key === 'Escape') {
      portalStore.closeSearchResults();
      e.preventDefault();
    }
  }

  function handleSearchInput(e) {
    portalStore.searchQuery = e.target.value;
    portalStore.debouncedSearch();
  }

  // Compute background style - image takes priority over gradient
  const backgroundStyle = $derived(() => {
    if (portalStore.backgroundImageUrl) {
      return `background: linear-gradient(rgba(0,0,0,0.4), rgba(0,0,0,0.4)), url(${portalStore.backgroundImageUrl}) center/cover no-repeat;`;
    }
    // Use selected gradient, or fall back to first gradient with a value (index 1) if "None" (index 0) is selected
    const gradientValue = gradients[portalStore.selectedGradient]?.value;
    if (gradientValue) {
      return `background: ${gradientValue};`;
    }
    // Fall back to first gradient with a value (index 1 = "Blue to Purple")
    return `background: ${gradients[1].value};`;
  });
</script>

<!-- Hero Section with Gradient or Background Image -->
<div class="hero-gradient {portalStore.isDarkMode ? 'dark-mode' : ''} {portalStore.hasBackgroundImage ? 'has-image' : ''}" style="{backgroundStyle()}">
  <div class="hero-content max-w-4xl mx-auto px-6 py-12 text-center">
    <!-- Search Box -->
    <div class="max-w-2xl mx-auto relative">
      <form onsubmit={handleSearch} class="relative">
        <div class="relative">
          <div class="absolute inset-y-0 left-0 pl-4 flex items-center pointer-events-none">
            <Search class="h-6 w-6 text-gray-400" />
          </div>
          <input
            type="text"
            value={portalStore.searchQuery}
            oninput={handleSearchInput}
            onkeydown={handleSearchKeydown}
            placeholder={portalStore.editableSearchPlaceholder}
            class="block w-full pl-12 pr-4 py-5 text-lg border-0 rounded-xl focus:outline-none focus:ring-2 focus:ring-white/30 transition-all shadow-xl"
            style="background-color: rgba(255, 255, 255, 0.95); color: #111827;"
          />
        </div>
      </form>

      <!-- Search hint / Editable fields -->
      {#if portalStore.isEditing}
        <div class="mt-4 space-y-3">
          <div>
            <label class="text-xs text-white/60 block mb-1">Search Box Placeholder:</label>
            <input
              type="text"
              value={portalStore.editableSearchPlaceholder}
              oninput={(e) => portalStore.editableSearchPlaceholder = e.target.value}
              class="text-sm text-white bg-transparent text-center focus:outline-none w-full"
              placeholder="Search placeholder text"
            />
          </div>
          <div>
            <label class="text-xs text-white/60 block mb-1">Search Hint Text:</label>
            <input
              type="text"
              value={portalStore.editableSearchHint}
              oninput={(e) => portalStore.editableSearchHint = e.target.value}
              class="text-sm text-white bg-transparent text-center focus:outline-none w-full"
              placeholder="Search hint text"
            />
          </div>
        </div>
      {:else}
        <p class="mt-4 text-sm text-white/80">
          {portalStore.editableSearchHint}
        </p>
      {/if}

      <!-- Search Results Dropdown -->
      {#if portalStore.showSearchResults}
        <div
          class="absolute left-0 right-0 mt-2 rounded shadow-2xl max-h-[70vh] overflow-hidden flex flex-col z-50"
          style="background-color: var(--ds-surface-card);"
        >
          <!-- Results Content -->
          <div class="flex-1 overflow-y-auto p-6 text-left">
            {#if portalStore.searchLoading}
              <!-- Loading State -->
              <div class="flex flex-col items-center justify-center py-12">
                <Spinner size="lg" class="mb-4" />
                <p class="text-sm" style="color: var(--ds-text-subtle);">Searching knowledge base...</p>
              </div>
            {:else if portalStore.searchError}
              <!-- Error State -->
              <div class="flex flex-col items-center justify-center py-12">
                <div class="w-12 h-12 rounded-full flex items-center justify-center mb-4" style="background-color: {portalStore.isDarkMode ? 'rgba(220, 38, 38, 0.1)' : '#fee2e2'};">
                  <X class="w-6 h-6" style="color: #dc2626;" />
                </div>
                <h3 class="text-lg font-semibold mb-2" style="color: var(--ds-text);">Search Failed</h3>
                <p class="text-sm text-center" style="color: var(--ds-text-subtle);">
                  {portalStore.searchError}
                </p>
              </div>
            {:else if portalStore.searchResults && portalStore.searchResults.data && portalStore.searchResults.data.length > 0}
              <!-- Results List -->
              <div class="space-y-3">
                <div class="text-sm mb-4" style="color: var(--ds-text-subtle);">
                  Found {portalStore.searchResults.data.length} result{portalStore.searchResults.data.length !== 1 ? 's' : ''} for "{portalStore.searchQuery}"
                </div>
                {#each portalStore.searchResults.data as result}
                  {@const parsed = portalStore.parseDocmostShareLink(portalStore.knowledgeBaseShareLink)}
                  <a
                    href="{parsed.baseURL}/share/{parsed.shareID}/p/{result.slugId}"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="block p-4 rounded border transition-all hover:shadow-md"
                    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
                  >
                    <div class="flex items-start gap-3">
                      <div class="flex-shrink-0 mt-1">
                        <BookOpen class="w-5 h-5" style="color: {portalStore.isDarkMode ? '#60a5fa' : '#2563eb'};" />
                      </div>
                      <div class="flex-1 min-w-0">
                        <h3 class="font-medium mb-1" style="color: var(--ds-text);">
                          {result.title}
                        </h3>
                        {#if result.highlight}
                          <p class="text-sm line-clamp-2 mb-1" style="color: var(--ds-text-subtle);">
                            {result.highlight.replace(/<[^>]*>/g, '')}
                          </p>
                        {:else if result.excerpt}
                          <p class="text-sm line-clamp-2" style="color: var(--ds-text-subtle);">
                            {result.excerpt}
                          </p>
                        {/if}
                      </div>
                      <div class="flex-shrink-0">
                        <svg class="w-5 h-5" style="color: var(--ds-text-subtle);" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                        </svg>
                      </div>
                    </div>
                  </a>
                {/each}
              </div>
            {:else}
              <!-- Empty State -->
              <div class="flex flex-col items-center justify-center py-12">
                <div class="w-12 h-12 rounded-full flex items-center justify-center mb-4" style="background-color: var(--ds-background-neutral);">
                  <Search class="w-6 h-6" style="color: var(--ds-text-subtle);" />
                </div>
                <h3 class="text-lg font-semibold mb-2" style="color: var(--ds-text);">No Results Found</h3>
                <p class="text-sm text-center" style="color: var(--ds-text-subtle);">
                  We couldn't find any articles matching "{portalStore.searchQuery}"
                </p>
              </div>
            {/if}
          </div>
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  /* Hero gradient background */
  .hero-gradient {
    width: 100%;
    position: relative;
  }

  /* Add subtle pattern overlay for depth (only for gradients, not images) */
  .hero-gradient:not(.has-image)::before {
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

  /* Dark mode overlay - dims the gradient (not needed for images as they have built-in overlay) */
  .hero-gradient.dark-mode:not(.has-image)::after {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background-color: rgba(0, 0, 0, 0.4);
    pointer-events: none;
    z-index: 0;
  }

  /* Ensure content appears above the gradient overlay */
  .hero-content {
    position: relative;
    z-index: 1;
  }

  /* Line clamp utility for truncating text */
  .line-clamp-2 {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  /* Style for search term highlights in knowledge base results */
  .line-clamp-2 :global(b) {
    font-weight: 600;
    color: var(--ds-interactive, #2563eb);
    background-color: var(--ds-interactive-subtle, #eff6ff);
    padding: 0 2px;
    border-radius: 2px;
  }
</style>
