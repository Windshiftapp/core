<script>
  import { onMount } from 'svelte';
  import { currentRoute } from '../../router.js';
  import { api } from '../../api.js';
  import Spinner from '../../components/Spinner.svelte';
  import FormRenderer from './FormRenderer.svelte';

  let slug = $derived($currentRoute.params?.slug || '');
  let embed = $derived(new URLSearchParams(window.location.search).get('embed') === 'true');

  let channel = $state(null);
  let forms = $state([]);
  let selectedFormId = $state(null);
  let loading = $state(true);
  let error = $state(null);

  let isDarkMode = $derived(channel?.config?.form_theme === 'dark' || (channel?.config?.form_theme === 'auto' && window.matchMedia?.('(prefers-color-scheme: dark)').matches));
  let brandColor = $derived(channel?.config?.form_brand_color || '#14b8a6');
  let logoUrl = $derived(channel?.config?.form_logo_url || '');

  onMount(async () => {
    await loadFormChannel();
  });

  async function loadFormChannel() {
    try {
      loading = true;
      error = null;

      const [channelData, formsData] = await Promise.all([
        api.forms.getChannel(slug),
        api.forms.getForms(slug),
      ]);

      channel = channelData;
      forms = formsData || [];

      // If only one form, select it automatically
      if (forms.length === 1) {
        selectedFormId = forms[0].id;
      }
    } catch (err) {
      console.error('Failed to load form channel:', err);
      error = err.message || 'Form not found';
    } finally {
      loading = false;
    }
  }

  function selectForm(formId) {
    selectedFormId = formId;
  }

  function backToList() {
    selectedFormId = null;
  }
</script>

<div
  class="min-h-screen flex flex-col"
  style="background-color: {isDarkMode ? '#0f172a' : '#f8fafc'};"
>
  {#if loading}
    <div class="flex-1 flex items-center justify-center">
      <Spinner />
    </div>
  {:else if error}
    <div class="flex-1 flex items-center justify-center">
      <div class="text-center">
        <div class="text-6xl mb-4">404</div>
        <p style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">{error}</p>
      </div>
    </div>
  {:else}
    <!-- Header (hidden in embed mode) -->
    {#if !embed}
      <header
        class="border-b"
        style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; border-color: {isDarkMode ? '#334155' : '#e2e8f0'};"
      >
        <div class="max-w-2xl mx-auto px-6 py-4 flex items-center gap-4">
          {#if logoUrl}
            <img src={logoUrl} alt="" class="h-8 w-auto" />
          {/if}
          <h1 class="text-lg font-semibold" style="color: {isDarkMode ? '#f1f5f9' : '#0f172a'};">
            {channel?.name || 'Form'}
          </h1>
        </div>
      </header>
    {/if}

    <!-- Content -->
    <main class="flex-1 flex items-start justify-center {embed ? 'p-4' : 'p-6'}">
      <div class="w-full max-w-2xl">
        {#if selectedFormId}
          <!-- Show form -->
          {#if forms.length > 1 && !embed}
            <button
              onclick={backToList}
              class="mb-4 text-sm font-medium flex items-center gap-1 transition-colors"
              style="color: {brandColor};"
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M19 12H5M12 19l-7-7 7-7" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
              Back to forms
            </button>
          {/if}

          {@const selectedForm = forms.find(f => f.id === selectedFormId)}
          <div
            class="rounded-xl border p-6 {embed ? '' : 'shadow-sm'}"
            style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; border-color: {isDarkMode ? '#334155' : '#e2e8f0'};"
          >
            {#if selectedForm}
              <div class="mb-6">
                <h2 class="text-xl font-bold" style="color: {isDarkMode ? '#f1f5f9' : '#0f172a'};">
                  {selectedForm.name}
                </h2>
                {#if selectedForm.description}
                  <p class="text-sm mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                    {selectedForm.description}
                  </p>
                {/if}
              </div>
            {/if}

            <FormRenderer
              formSlug={slug}
              formId={selectedFormId}
              {brandColor}
              {isDarkMode}
            />
          </div>
        {:else if forms.length === 0}
          <!-- No forms -->
          <div class="text-center py-12">
            <div class="w-16 h-16 mx-auto mb-4 rounded-full flex items-center justify-center" style="background-color: {brandColor}20;">
              <svg class="w-8 h-8" style="color: {brandColor};" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </div>
            <p class="text-sm" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">No forms available</p>
          </div>
        {:else}
          <!-- Form list -->
          <div class="space-y-3">
            {#each forms as form}
              <button
                onclick={() => selectForm(form.id)}
                class="w-full text-left p-5 rounded-xl border transition-all hover:shadow-md"
                style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; border-color: {isDarkMode ? '#334155' : '#e2e8f0'};"
              >
                <div class="flex items-center gap-4">
                  <div class="w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0" style="background-color: {form.color || brandColor}20;">
                    <svg class="w-5 h-5" style="color: {form.color || brandColor};" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" stroke-linecap="round" stroke-linejoin="round"/>
                    </svg>
                  </div>
                  <div class="flex-1 min-w-0">
                    <div class="font-medium" style="color: {isDarkMode ? '#f1f5f9' : '#0f172a'};">
                      {form.name}
                    </div>
                    {#if form.description}
                      <div class="text-sm mt-0.5 truncate" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                        {form.description}
                      </div>
                    {/if}
                  </div>
                  <svg class="w-5 h-5 flex-shrink-0" style="color: {isDarkMode ? '#475569' : '#9ca3af'};" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M9 18l6-6-6-6" stroke-linecap="round" stroke-linejoin="round"/>
                  </svg>
                </div>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    </main>

    <!-- Footer (hidden in embed mode) -->
    {#if !embed}
      <footer class="py-4 text-center">
        <p class="text-xs" style="color: {isDarkMode ? '#475569' : '#9ca3af'};">
          Powered by Windshift
        </p>
      </footer>
    {/if}
  {/if}
</div>
