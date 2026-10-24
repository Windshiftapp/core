<script>
  import { useResizeObserver } from 'runed';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import FileText from '@lucide/svelte/icons/file-text';
  import { currentRoute, navigate } from '../../router.js';
  import { authStore } from '../../stores/auth.svelte.js';
  import EmptyState from '../../components/EmptyState.svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import BackButton from '../../components/BackButton.svelte';
  import FormRenderer from './FormRenderer.svelte';
  import { loadPublicFormBootstrap } from './publicFormData.js';

  let slug = $derived($currentRoute.params?.slug || '');
  let requestedFormId = $derived(parseRequestedFormId($currentRoute.params?.formId));
  let embed = $derived(new URLSearchParams(window.location.search).get('embed') === 'true');

  let channel = $state(null);
  let forms = $state([]);
  let selectedFormId = $state(null);
  let selectedFormDetail = $state(null);
  let loading = $state(true);
  let error = $state(null);
  let routeLoadVersion = 0;

  let isDarkMode = $derived(channel?.theme === 'dark' || (channel?.theme === 'auto' && window.matchMedia?.('(prefers-color-scheme: dark)').matches));
  let brandColor = $derived(channel?.brand_color || null);
  let logoUrl = $derived(channel?.logo_url || '');

  // The router keeps this root component mounted while switching between the
  // channel list and individual form URLs. Treat route params as authoritative
  // so browser Back/Forward follows the URL instead of stale local selection.
  $effect(() => {
    const routeSlug = slug;
    const routeFormId = requestedFormId;
    void loadFormChannel(routeSlug, routeFormId);
  });

  // Tell the parent window (widget.js iframe host) how tall we are so it can
  // resize the iframe. Runs only when embedded inside another window.
  function postHeight() {
    const height = document.documentElement.scrollHeight;
    window.parent.postMessage({ type: 'ws-form-resize', height }, '*');
  }
  $effect(() => {
    if (!embed || window.parent === window) return;
    postHeight();
  });
  useResizeObserver(
    () => (embed && window.parent !== window ? document.documentElement : null),
    postHeight
  );

  function parseRequestedFormId(rawFormId) {
    if (rawFormId === undefined) return null;
    if (!/^[1-9]\d*$/.test(rawFormId)) return NaN;
    const parsed = Number(rawFormId);
    return Number.isSafeInteger(parsed) ? parsed : NaN;
  }

  async function loadFormChannel(routeSlug, routeFormId) {
    const version = ++routeLoadVersion;
    try {
      loading = true;
      error = null;

      const bootstrap = await loadPublicFormBootstrap(routeSlug);
      if (version !== routeLoadVersion) return;

      channel = bootstrap.channel;
      forms = bootstrap.forms || [];
      selectedFormId = null;
      selectedFormDetail = null;

      if (routeFormId !== null) {
        if (!Number.isSafeInteger(routeFormId) || !forms.some(form => form.id === routeFormId)) {
          throw new Error('Form not found');
        }
        selectedFormId = routeFormId;
        if (bootstrap.form_detail?.form_id === routeFormId) {
          selectedFormDetail = bootstrap.form_detail;
        }
      } else if (forms.length === 1) {
        // Keep the channel URL canonical for sole-form channels.
        selectedFormId = forms[0].id;
        selectedFormDetail = bootstrap.form_detail || null;
      }
    } catch (err) {
      if (version !== routeLoadVersion) return;
      console.error('Failed to load form channel:', err);
      error = err.message || 'Form not found';
    } finally {
      if (version === routeLoadVersion) loading = false;
    }
  }

  function selectForm(formId) {
    selectedFormId = formId;
    selectedFormDetail = null;
    navigate(`/forms/${encodeURIComponent(slug)}/${formId}${embed ? '?embed=true' : ''}`);
  }

  function backToList() {
    selectedFormId = null;
    selectedFormDetail = null;
    navigate(`/forms/${encodeURIComponent(slug)}${embed ? '?embed=true' : ''}`);
  }
</script>

<div
  class="public-form-shell min-h-screen flex flex-col"
  class:ds-brand-scope={Boolean(brandColor)}
  data-ds-color-mode={isDarkMode ? 'dark' : 'light'}
  style={brandColor ? `--ds-brand-color: ${brandColor}` : undefined}
  data-testid="public-form-page"
  data-ready={!loading && !error}
>
  {#if !embed}
    <header
      class="relative z-40 border-b"
      style="background-color: color-mix(in srgb, var(--ds-surface-card) 94%, transparent); border-color: var(--ds-border);"
    >
      <div class="mx-auto flex h-16 max-w-6xl items-center gap-3 px-4 sm:px-6">
        {#if logoUrl}
          <img src={logoUrl} alt="" class="h-8 w-8 flex-none object-contain sm:w-auto sm:max-w-28" />
        {/if}
        <div class="min-w-0">
          <h1 class="truncate text-sm font-semibold sm:text-base" style="color: var(--ds-text);">
            {channel?.name || 'Forms'}
          </h1>
        </div>
      </div>
    </header>
  {/if}

  <main class="flex-1" style="background-color: var(--ds-surface);">
    <div class="{embed ? 'p-3' : 'mx-auto max-w-6xl px-4 py-8 sm:px-6 sm:py-12'}">
      {#if loading}
        <StateDisplay type="loading" message="Loading form…" />
      {:else if error}
        <StateDisplay
          type="error"
          title="Form unavailable"
          message={error}
        />
      {:else if selectedFormId}
        {@const selectedForm = forms.find(f => f.id === selectedFormId)}
        <div class={embed ? 'w-full' : 'mx-auto max-w-3xl'}>
          {#if forms.length > 1 && !embed}
            <BackButton
              onclick={backToList}
              testid="public-form-back"
              label="Back to forms"
              class="mb-6"
            />
          {/if}

          {#if selectedForm}
            <div class="mb-6">
              <h2
                data-testid="public-form-title"
                class="text-2xl font-semibold leading-8 sm:text-3xl"
                style="color: var(--ds-text);"
              >
                {selectedForm.name}
              </h2>
              <p class="mt-1 text-sm leading-5" style="color: var(--ds-text-subtle);">
                {selectedForm.description || 'Complete the fields below to send your request.'}
              </p>
            </div>
          {/if}

          <div
            class="{embed ? '' : 'rounded-lg border p-5 sm:p-6'}"
            style={embed ? '' : 'background-color: var(--ds-surface-card); border-color: var(--ds-border);'}
          >
            <FormRenderer
              formSlug={slug}
              formId={selectedFormId}
              formConfig={selectedForm?.config}
              attachmentConfig={selectedForm?.config?.allow_attachments ? channel?.attachments : null}
              initialDetail={selectedFormDetail}
              authenticationRequired={selectedForm?.config?.require_auth === true && !$authStore.isAuthenticated}
              {embed}
              {brandColor}
              {isDarkMode}
            />
          </div>
        </div>
      {:else if forms.length === 0}
        <div
          class="rounded-md border p-6"
          style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
        >
          <EmptyState
            icon={FileText}
            title="No forms available"
            description="This channel has not published any forms yet."
          />
        </div>
      {:else}
        <section>
          <h2 class="mb-1 text-xl font-medium" style="color: var(--ds-text);">How can we help?</h2>
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            Select the form that best matches your request.
          </p>

          <div class="mt-5 grid grid-cols-1 gap-3 md:grid-cols-2">
            {#each forms as form}
              <button
                type="button"
                onclick={() => selectForm(form.id)}
                data-testid={`public-form-option-${form.id}`}
                class="form-option group relative m-0 w-full cursor-pointer appearance-none rounded-md border p-4 text-left font-[inherit] text-[inherit] transition-colors focus:outline-none focus-visible:ring-2"
              >
                <div class="flex items-start gap-3.5">
                  <div
                    class="flex h-9 w-9 flex-none items-center justify-center rounded-md"
                    style="background-color: color-mix(in srgb, {form.color || 'var(--ds-text-subtle)'} 12%, transparent); color: {form.color || 'var(--ds-text-subtle)'};"
                  >
                    <FileText class="h-[18px] w-[18px]" />
                  </div>
                  <div class="min-w-0 flex-1">
                    <div class="flex items-center gap-2 text-sm font-medium leading-5" style="color: var(--ds-text);">
                      <span class="truncate">{form.name}</span>
                    {#if form.config?.require_auth}
                        <span
                          class="rounded px-1.5 py-0.5 text-[10px] font-medium"
                          style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
                        >
                          SIGN-IN
                        </span>
                    {/if}
                    </div>
                    {#if form.description}
                      <p class="mt-1.5 line-clamp-2 text-sm leading-5" style="color: var(--ds-text-subtle);">
                        {form.description}
                      </p>
                    {/if}
                  </div>
                  <ChevronRight class="mt-2 h-4 w-4 flex-none" style="color: var(--ds-text-subtle);" />
                </div>
              </button>
            {/each}
          </div>
        </section>
      {/if}
    </div>
  </main>

  {#if !embed}
    <footer
      class="mt-auto border-t"
      style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
    >
      <div class="mx-auto max-w-6xl px-4 py-4 text-center sm:px-6">
        <p class="text-xs" style="color: var(--ds-text-subtle);">Powered by Windshift</p>
      </div>
    </footer>
  {/if}
</div>

<style>
  .public-form-shell {
    background: var(--ds-surface);
    color: var(--ds-text);
  }

  .form-option {
    background-color: var(--ds-surface-card);
    border-color: var(--ds-border);
    --tw-ring-color: var(--ds-border-focused);
  }

  .form-option:hover {
    border-color: var(--ds-border-focused);
  }
</style>
