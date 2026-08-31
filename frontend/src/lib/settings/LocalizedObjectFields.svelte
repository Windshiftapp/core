<script>
  import { ChevronDown, Languages, Trash2 } from '@lucide/svelte';
  import { api } from '../api.js';
  import AlertBox from '../components/AlertBox.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Label from '../components/Label.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import { i18n, t } from '../stores/i18n.svelte.js';
  import { isSystemAdmin } from '../stores/permissions.svelte.js';

  let {
    objectType,
    objectId,
    canonicalName = $bindable(''),
    canonicalDescription = $bindable(''),
    displayName = '',
    displayDescription = '',
    fields = ['name', 'description'],
    canWrite = $isSystemAdmin,
    canonicalEditable = true,
  } = $props();

  let loading = $state(true);
  let loadError = $state('');
  let translations = $state([]);
  let overrides = $state({});
  let originalOverrides = $state({});
  let primaryValues = $state({ name: '', description: '' });
  let originalCanonical = $state({ name: '', description: '' });
  let canonicalDecision = $state('');
  let initializedKey = $state('');

  const activeLocale = $derived(i18n.locale);
  const supportedLocales = $derived(i18n.supportedLocales);
  const editableLocales = $derived.by(() => {
    const locales = new Map(supportedLocales.map((locale) => [locale.code, locale]));
    locales.set(activeLocale, {
      code: activeLocale,
      name: locales.get(activeLocale)?.name || activeLocale,
      direction: locales.get(activeLocale)?.direction || 'ltr',
    });
    for (const translation of translations) {
      if (!locales.has(translation.locale)) {
        locales.set(translation.locale, {
          code: translation.locale,
          name: translation.locale,
          direction: 'ltr',
        });
      }
    }
    return [...locales.values()];
  });
  const includesDescription = $derived(fields.includes('description'));
  const canonicalChanged = $derived(
    canonicalEditable && (canonicalName !== originalCanonical.name ||
      (includesDescription && canonicalDescription !== originalCanonical.description)
    )
  );
  const affectedTranslations = $derived(
    translations.filter((translation) => fields.includes(translation.field))
  );
  const hasTranslations = $derived(
    affectedTranslations.length > 0 ||
      Object.values(overrides).some((values) =>
        fields.some((field) => values[field]?.trim())
      )
  );
  const instanceTranslations = $derived(
    translations.filter(
      (translation) => translation.source === 'instance' && fields.includes(translation.field)
    )
  );
  const canonicalChoiceOptions = $derived([
    { value: '', label: t('settings.localizedObjects.chooseCanonicalEffect') },
    { value: 'keep', label: t('settings.localizedObjects.keepOverrides') },
    { value: 'remove-instance', label: t('settings.localizedObjects.removeInstanceOverrides') },
  ]);

  function localeName(code) {
    return supportedLocales.find((locale) => locale.code === code)?.name || code;
  }

  function buildOverrideMap(rows) {
    const next = {};
    for (const row of rows) {
      if (row.source !== 'instance' || !fields.includes(row.field)) continue;
      next[row.locale] = { ...(next[row.locale] || {}), [row.field]: row.value };
    }
    return next;
  }

  async function initialize(key, name, description, shownName, shownDescription) {
    originalCanonical = { name, description };
    primaryValues = {
      name: shownName || name,
      description: shownDescription || description,
    };
    canonicalDecision = '';
    translations = [];
    overrides = {};
    originalOverrides = {};
    loadError = '';

    if (!objectId || !canWrite) {
      loading = false;
      return;
    }

    loading = true;
    try {
      const rows = (await api.objectTranslations.list(objectType, objectId)) || [];
      if (initializedKey !== key) return;
      translations = rows;
      overrides = buildOverrideMap(rows);
      originalOverrides = Object.fromEntries(
        Object.entries(overrides).map(([locale, values]) => [locale, { ...values }])
      );
      const activeOverride = overrides[activeLocale] || {};
      primaryValues = {
        name: activeOverride.name ?? shownName ?? name,
        description: activeOverride.description ?? shownDescription ?? description,
      };
    } catch (error) {
      loadError = error?.message || t('settings.localizedObjects.loadFailed');
    } finally {
      if (initializedKey === key) loading = false;
    }
  }

  $effect(() => {
    const key = `${objectType}:${objectId || 'new'}:${activeLocale}:${canWrite}`;
    if (key === initializedKey) return;
    initializedKey = key;
    void initialize(
      key,
      canonicalName,
      canonicalDescription,
      displayName,
      displayDescription
    );
  });

  function setOverride(locale, field, value) {
    overrides = {
      ...overrides,
      [locale]: { ...(overrides[locale] || {}), [field]: value },
    };
    if (locale === activeLocale) {
      primaryValues = { ...primaryValues, [field]: value };
    }
  }

  function setPrimary(field, value) {
    primaryValues = { ...primaryValues, [field]: value };
    setOverride(activeLocale, field, value);
  }

  function systemValue(locale, field) {
    return translations.find(
      (translation) =>
        translation.locale === locale &&
        translation.field === field &&
        translation.source === 'system'
    )?.value;
  }

  async function resolveFallback(locale, field) {
    const fallback = field === 'name' ? canonicalName : canonicalDescription;
    const [resolved] = await api.objectTranslations.resolve(locale, [
      { object_type: objectType, object_id: objectId, field, fallback },
    ]);
    return resolved?.value ?? fallback;
  }

  async function removeOverride(locale, field) {
    const existing = originalOverrides[locale]?.[field];
    if (existing !== undefined) {
      await api.objectTranslations.delete(objectType, objectId, field, locale);
    }
    const nextLocale = { ...(overrides[locale] || {}) };
    delete nextLocale[field];
    overrides = { ...overrides, [locale]: nextLocale };
    const nextOriginalLocale = { ...(originalOverrides[locale] || {}) };
    delete nextOriginalLocale[field];
    originalOverrides = { ...originalOverrides, [locale]: nextOriginalLocale };
    translations = translations.filter(
      (translation) =>
        !(
          translation.source === 'instance' &&
          translation.locale === locale &&
          translation.field === field
        )
    );
    if (locale === activeLocale) {
      primaryValues = {
        ...primaryValues,
        [field]: await resolveFallback(locale, field),
      };
    }
    window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
  }

  export function validate() {
    if (!objectId || !canWrite) return;
    if (!primaryValues.name.trim()) {
      throw new Error(t('settings.localizedObjects.localizedNameRequired'));
    }
    if (canonicalChanged && affectedTranslations.length > 0 && !canonicalDecision) {
      throw new Error(t('settings.localizedObjects.canonicalChoiceRequired'));
    }
  }

  export async function save() {
    validate();
    if (!objectId || !canWrite) return;

    if (canonicalDecision === 'remove-instance') {
      for (const translation of instanceTranslations) {
        await api.objectTranslations.delete(
          objectType,
          objectId,
          translation.field,
          translation.locale
        );
      }
    } else {
      const locales = new Set([
        ...Object.keys(originalOverrides),
        ...Object.keys(overrides),
      ]);
      for (const locale of locales) {
        for (const field of fields) {
          const before = originalOverrides[locale]?.[field];
          const after = overrides[locale]?.[field]?.trim();
          if (after === before) continue;
          if (after) {
            await api.objectTranslations.upsert(objectType, objectId, field, locale, after);
          } else if (before !== undefined) {
            await api.objectTranslations.delete(objectType, objectId, field, locale);
          }
        }
      }
    }
    window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
  }
</script>

{#if !objectId}
  <div class="space-y-4">
    <div>
      <Label required>{t('settings.localizedObjects.baseName')}</Label>
      <Input bind:value={canonicalName} required />
    </div>
    {#if includesDescription}
      <div>
        <Label>{t('settings.localizedObjects.baseDescription')}</Label>
        <Textarea bind:value={canonicalDescription} rows={2} />
      </div>
    {/if}
  </div>
{:else if !canWrite}
  <AlertBox variant="info" message={t('settings.localizedObjects.permissionRequired')} />
{:else}
  <div class="space-y-5" data-testid="localized-object-editor">
    <div class="rounded-lg border p-4" style="border-color: var(--ds-border); background: var(--ds-surface);">
      <div class="mb-3 flex items-center gap-2">
        <Languages class="h-4 w-4" style="color: var(--ds-icon);" />
        <div>
          <p class="text-sm font-medium" style="color: var(--ds-text);">
            {t('settings.localizedObjects.displayLabelFor', { locale: localeName(activeLocale) })}
          </p>
          <p class="text-xs" style="color: var(--ds-text-subtle);">
            {t('settings.localizedObjects.primaryHelp')}
          </p>
        </div>
      </div>

      {#if loading}
        <p class="text-sm" style="color: var(--ds-text-subtle);">{t('common.loading')}</p>
      {:else}
        {#if loadError}<AlertBox variant="error" message={loadError} class="mb-3" />{/if}
        <div class="space-y-3">
          <div>
            <Label required>{t('common.name')}</Label>
            <Input
              value={primaryValues.name}
              oninput={(event) => setPrimary('name', event.currentTarget.value)}
              dataTestid={`localized-object-name-${activeLocale}`}
              required
            />
          </div>
          {#if includesDescription}
            <div>
              <Label>{t('common.description')}</Label>
              <Textarea
                value={primaryValues.description}
                oninput={(event) => setPrimary('description', event.currentTarget.value)}
                data-testid={`localized-object-description-${activeLocale}`}
                rows={2}
              />
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <details class="rounded-lg border" style="border-color: var(--ds-border);">
      <summary
        class="flex cursor-pointer list-none items-center justify-between px-4 py-3 text-sm font-medium"
        style="color: var(--ds-text);"
        data-testid="localized-object-overrides-toggle"
      >
        {t('settings.localizedObjects.localeOverrides')}
        <ChevronDown class="h-4 w-4" />
      </summary>
      <div class="space-y-4 border-t p-4" style="border-color: var(--ds-border);">
        {#each editableLocales as locale (locale.code)}
          <div class="rounded border p-3" style="border-color: var(--ds-border);">
            <p class="mb-2 text-sm font-medium" style="color: var(--ds-text);">{locale.name} <span class="font-normal" style="color: var(--ds-text-subtle);">({locale.code})</span></p>
            {#each fields as field (field)}
              <div class="mb-2 last:mb-0">
                <Label>{field === 'name' ? t('common.name') : t('common.description')}</Label>
                <div class="flex items-start gap-2">
                  {#if field === 'name'}
                    <Input
                      value={overrides[locale.code]?.[field] || ''}
                      placeholder={systemValue(locale.code, field) || (field === 'name' ? canonicalName : canonicalDescription)}
                      oninput={(event) => setOverride(locale.code, field, event.currentTarget.value)}
                      dataTestid={`localized-object-override-${field}-${locale.code}`}
                    />
                  {:else}
                    <Textarea
                      value={overrides[locale.code]?.[field] || ''}
                      placeholder={systemValue(locale.code, field) || canonicalDescription}
                      oninput={(event) => setOverride(locale.code, field, event.currentTarget.value)}
                      data-testid={`localized-object-override-${field}-${locale.code}`}
                      rows={2}
                    />
                  {/if}
                  {#if originalOverrides[locale.code]?.[field] !== undefined}
                    <Button
                      variant="default"
                      size="sm"
                      icon={Trash2}
                      onclick={() => removeOverride(locale.code, field)}
                      dataTestid={`localized-object-remove-${field}-${locale.code}`}
                    >
                      {t('common.remove')}
                    </Button>
                  {/if}
                </div>
                {#if systemValue(locale.code, field)}
                  <p class="mt-1 text-xs" style="color: var(--ds-text-subtle);">
                    {t('settings.localizedObjects.shippedFallback', { value: systemValue(locale.code, field) })}
                  </p>
                {/if}
              </div>
            {/each}
          </div>
        {/each}
      </div>
    </details>

    {#if canonicalEditable}
    <details class="rounded-lg border" style="border-color: var(--ds-border);">
      <summary
        class="flex cursor-pointer list-none items-center justify-between px-4 py-3 text-sm font-medium"
        style="color: var(--ds-text);"
        data-testid="localized-object-canonical-toggle"
      >
        {t('settings.localizedObjects.canonicalFallback')}
        <ChevronDown class="h-4 w-4" />
      </summary>
      <div class="space-y-3 border-t p-4" style="border-color: var(--ds-border);">
        <p class="text-xs" style="color: var(--ds-text-subtle);">
          {t('settings.localizedObjects.canonicalHelp')}
        </p>
        <div>
          <Label required>{t('settings.localizedObjects.baseName')}</Label>
          <Input bind:value={canonicalName} required dataTestid="localized-object-canonical-name" />
        </div>
        {#if includesDescription}
          <div>
            <Label>{t('settings.localizedObjects.baseDescription')}</Label>
            <Textarea bind:value={canonicalDescription} rows={2} data-testid="localized-object-canonical-description" />
          </div>
        {/if}
        {#if canonicalChanged && hasTranslations}
          <AlertBox variant="warning">
            <div class="space-y-2">
              <p>{t('settings.localizedObjects.canonicalWarning')}</p>
              <ul class="list-disc pl-5 text-xs">
                {#each affectedTranslations as translation}
                  <li>{localeName(translation.locale)} · {translation.field} · {translation.source}</li>
                {/each}
              </ul>
              <Select
                id="localized-object-canonical-decision"
                bind:value={canonicalDecision}
                options={canonicalChoiceOptions}
                required
              />
            </div>
          </AlertBox>
        {/if}
      </div>
    </details>
    {/if}
  </div>
{/if}
