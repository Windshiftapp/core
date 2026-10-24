<script>
  import { onMount } from 'svelte';
  import IconExternalLink from '@tabler/icons-svelte-runes/icons/external-link';
  import IconCode from '@tabler/icons-svelte-runes/icons/code';
  import IconWorldWww from '@tabler/icons-svelte-runes/icons/world-www';
  import IconBrandJavascript from '@tabler/icons-svelte-runes/icons/brand-javascript';
  import { forms as formsApi } from '../../api/forms.js';
  import { t } from '../../stores/i18n.svelte.js';
  import Button from '../../components/Button.svelte';
  import CopyButton from '../../components/CopyButton.svelte';
  import Input from '../../components/Input.svelte';
  import Label from '../../components/Label.svelte';
  import { publicBaseURL } from '../../runtime/contextPath.js';
	import TextField from '../../components/TextField.svelte';

  let { slug = '', runtimeUrl = '' } = $props();

  let activeTab = $state('hosted');
  let iframeWidth = $state('100%');
  let iframeHeight = $state('600');
  let hasAuthenticatedForms = $state(false);
  let embedModeCheckComplete = $state(false);
  let embedModeCheckFailed = $state(false);

  let baseUrl = $derived(runtimeUrl || publicBaseURL());
  let formUrl = $derived(`${baseUrl}/forms/${slug}`);
  let embedUrl = $derived(`${baseUrl}/forms/${slug}?embed=true`);

  let iframeCode = $derived(
    `<iframe src="${embedUrl}" width="${iframeWidth}" height="${iframeHeight}" frameborder="0" style="border: none;"></iframe>`
  );

  let widgetCode = $derived(
    `<div id="ws-form-${slug}"></div>\n<script src="${baseUrl}/embed/windshift-forms.js" data-slug="${slug}" data-target="ws-form-${slug}"><\/script>`
  );

  const allTabs = [
    { id: 'hosted', icon: IconWorldWww, label: t('forms.integration.hostedUrl') },
    { id: 'iframe', icon: IconCode, label: t('forms.integration.iframe') },
    { id: 'widget', icon: IconBrandJavascript, label: t('forms.integration.jsWidget') },
  ];
  let tabs = $derived(
    embedModeCheckComplete && !hasAuthenticatedForms ? allTabs : allTabs.slice(0, 1)
  );

  onMount(async () => {
    if (!slug) return;
    try {
      const forms = await formsApi.getForms(slug);
      hasAuthenticatedForms = (forms || []).some(form => form.config?.require_auth === true);
      embedModeCheckComplete = true;
      if (hasAuthenticatedForms) activeTab = 'hosted';
    } catch (error) {
      embedModeCheckFailed = true;
      console.error('Failed to inspect form authentication requirements:', error);
    }
  });
</script>

<div class="space-y-4">
  <h4 class="text-sm font-semibold" style="color: var(--ds-text);">{t('forms.integration.title')}</h4>

  {#if hasAuthenticatedForms}
    <div
      data-testid="authenticated-form-embed-warning"
      class="rounded-md border p-3 text-sm"
      style="border-color: var(--ds-border-warning, #ca8a04); background: var(--ds-background-warning, #fef9c3); color: var(--ds-text-warning, #854d0e);"
    >
      This channel has a sign-in-required form. Use the share link; embedded forms cannot pass along a Windshift sign-in.
    </div>
  {:else if embedModeCheckFailed}
    <div
      data-testid="form-embed-mode-check-failed"
      class="rounded-md border p-3 text-sm"
      style="border-color: var(--ds-border-warning, #ca8a04); background: var(--ds-background-warning, #fef9c3); color: var(--ds-text-warning, #854d0e);"
    >
      Windshift could not check whether these forms require sign-in. Use the share link, or try again before embedding them.
    </div>
  {/if}

  <!-- Tab selector -->
  <div class="flex gap-1 p-1 rounded-lg" style="background-color: var(--ds-background-neutral);">
    {#each tabs as tab}
      <button
        onclick={() => activeTab = tab.id}
        data-testid={`form-integration-tab-${tab.id}`}
        class="flex-1 flex items-center justify-center gap-2 px-3 py-2 text-xs font-medium rounded-md transition-colors"
        style={activeTab === tab.id
          ? 'background-color: var(--ds-surface); color: var(--ds-text); box-shadow: 0 1px 2px rgba(0,0,0,0.05);'
          : 'color: var(--ds-text-subtle);'}
      >
        <tab.icon class="w-3.5 h-3.5" />
        {tab.label}
      </button>
    {/each}
  </div>

  <!-- Hosted URL -->
  {#if activeTab === 'hosted'}
    <div class="space-y-3">
      <div class="flex items-center gap-2">
        <Input
          type="text"
          value={formUrl}
          readonly
          class="flex-1 font-mono"
          size="small"
        />
        <CopyButton
          text={formUrl}
          size="sm"
          label={t('forms.integration.copyCode')}
          copiedLabel={t('forms.integration.copied')}
        />
      </div>
      <Button onclick={() => window.open(formUrl, '_blank')} variant="default" size="small" icon={IconExternalLink}>
        {t('forms.integration.openInNewTab')}
      </Button>
    </div>
  {/if}

  <!-- iframe Embed -->
  {#if activeTab === 'iframe'}
    <div class="space-y-3">
      <div class="grid grid-cols-2 gap-3">
        <div>
          <TextField
            label={t('forms.integration.widthLabel')}
            labelColor="default"
            placeholder="100%"
            bind:value={iframeWidth}
          />
        </div>
        <div>
          <TextField
            label={t('forms.integration.heightLabel')}
            labelColor="default"
            placeholder="600"
            bind:value={iframeHeight}
          />
        </div>
      </div>
      <div class="relative">
        <pre class="p-3 rounded-lg text-xs overflow-x-auto font-mono" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">{iframeCode}</pre>
        <div class="absolute top-1 right-1">
          <CopyButton text={iframeCode} size="sm" title={t('forms.integration.copyCode')} />
        </div>
      </div>
    </div>
  {/if}

  <!-- JS Widget -->
  {#if activeTab === 'widget'}
    <div class="space-y-3">
      <div class="relative">
        <pre class="p-3 rounded-lg text-xs overflow-x-auto font-mono whitespace-pre-wrap" style="background-color: var(--ds-background-neutral); color: var(--ds-text);">{widgetCode}</pre>
        <div class="absolute top-1 right-1">
          <CopyButton text={widgetCode} size="sm" title={t('forms.integration.copyCode')} />
        </div>
      </div>
    </div>
  {/if}
</div>
