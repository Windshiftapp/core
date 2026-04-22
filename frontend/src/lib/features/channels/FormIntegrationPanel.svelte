<script>
  import { IconExternalLink, IconCode, IconWorldWww, IconBrandJavascript } from '@tabler/icons-svelte-runes';
  import { t } from '../../stores/i18n.svelte.js';
  import Button from '../../components/Button.svelte';
  import CopyButton from '../../components/CopyButton.svelte';
  import Input from '../../components/Input.svelte';
  import Label from '../../components/Label.svelte';

  let { slug = '', runtimeUrl = '' } = $props();

  let activeTab = $state('hosted');
  let iframeWidth = $state('100%');
  let iframeHeight = $state('600');

  let baseUrl = $derived(runtimeUrl || window.location.origin);
  let formUrl = $derived(`${baseUrl}/forms/${slug}`);
  let embedUrl = $derived(`${baseUrl}/forms/${slug}?embed=true`);

  let iframeCode = $derived(
    `<iframe src="${embedUrl}" width="${iframeWidth}" height="${iframeHeight}" frameborder="0" style="border: none;"></iframe>`
  );

  let widgetCode = $derived(
    `<div id="ws-form-${slug}"></div>\n<script src="${baseUrl}/forms/widget.js" data-slug="${slug}" data-target="ws-form-${slug}"><\/script>`
  );

  const tabs = [
    { id: 'hosted', icon: IconWorldWww, label: t('forms.integration.hostedUrl') },
    { id: 'iframe', icon: IconCode, label: t('forms.integration.iframe') },
    { id: 'widget', icon: IconBrandJavascript, label: t('forms.integration.jsWidget') },
  ];
</script>

<div class="space-y-4">
  <h4 class="text-sm font-semibold" style="color: var(--ds-text);">{t('forms.integration.title')}</h4>

  <!-- Tab selector -->
  <div class="flex gap-1 p-1 rounded-lg" style="background-color: var(--ds-background-neutral);">
    {#each tabs as tab}
      <button
        onclick={() => activeTab = tab.id}
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
        <input
          type="text"
          value={formUrl}
          readonly
          class="flex-1 px-3 py-2 text-sm rounded-lg border font-mono"
          style="background-color: var(--ds-background-neutral); border-color: var(--ds-border); color: var(--ds-text);"
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
          <Label color="default" class="mb-1">{t('forms.integration.widthLabel')}</Label>
          <Input bind:value={iframeWidth} placeholder="100%" />
        </div>
        <div>
          <Label color="default" class="mb-1">{t('forms.integration.heightLabel')}</Label>
          <Input bind:value={iframeHeight} placeholder="600" />
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
