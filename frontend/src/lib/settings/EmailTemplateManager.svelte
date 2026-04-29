<script>
  import { onMount } from 'svelte';
  import { writable } from 'svelte/store';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Label from '../components/Label.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import Textarea from '../components/Textarea.svelte';
  import { Mail, Edit, Eye } from 'lucide-svelte';

  // Per-template variable hints shown in the editor sidebar so admins know
  // which {{.Variables}} are available for each row. Mirrors the data
  // injected by senders in internal/smtp + internal/services.
  const VARIABLES = {
    magic_link: [
      { name: '{{.FirstName}}', description: "Recipient's first name (or \"there\" if unset)." },
      { name: '{{.MagicLinkURL}}', description: 'Sign-in URL with the token in the fragment.' },
    ],
    email_verification: [
      { name: '{{.FirstName}}', description: "Recipient's first name." },
      { name: '{{.VerificationURL}}', description: 'Email-verification URL with the token.' },
    ],
    invitation: [
      { name: '{{.FirstName}}', description: "Recipient's first name." },
      { name: '{{.InvitationURL}}', description: 'Set-password URL with the invitation token.' },
    ],
    notification_batch: [
      { name: '{{.UserName}}', description: 'Display name of the recipient.' },
      { name: '{{.NotificationCount}}', description: 'Number of notifications in this batch.' },
      { name: '{{range .Notifications}} … {{end}}', description: 'Iterates over each notification.' },
      { name: '{{.Title}}', description: 'Per-notification title (inside the range).' },
      { name: '{{.Message}}', description: 'Per-notification message body (inside the range).' },
      { name: '{{.Type}}', description: 'Notification kind (assignment, comment, etc.).' },
      { name: '{{.AccentColor}}', description: 'Hex color for the left-border accent.' },
      { name: '{{.FormattedTime}}', description: 'Human-readable timestamp.' },
    ],
    portal_reply: [
      { name: '{{.AuthorName}}', description: 'Internal user who wrote the reply.' },
      { name: '{{.ItemKey}}', description: 'Item key (e.g. ACME-42).' },
      { name: '{{.ItemTitle}}', description: 'Item title.' },
      { name: '{{.Content}}', description: 'The reply text. Auto-escaped for HTML safety.' },
      { name: '{{.OriginalSubject}}', description: 'Used as the email subject (with "Re:" prefix added by the threading flow).' },
    ],
  };

  const templates = writable([]);
  let loading = $state(true);

  let showEditor = $state(false);
  let editing = $state(null);
  let formData = $state({ subject: '', html_body: '', text_body: '', description: '', is_active: true });

  let showPreview = $state(false);
  let previewSubject = $state('');
  let previewHTML = $state('');
  let previewText = $state('');
  let previewLoading = $state(false);

  onMount(loadTemplates);

  async function loadTemplates() {
    loading = true;
    try {
      const list = await api.emailTemplates.getAll();
      templates.set(list || []);
    } catch (err) {
      errorToast(t('settings.emailTemplates.loadFailed', { error: err.message }));
    } finally {
      loading = false;
    }
  }

  function openEditor(tmpl) {
    editing = tmpl;
    formData = {
      subject: tmpl.subject || '',
      html_body: tmpl.html_body || '',
      text_body: tmpl.text_body || '',
      description: tmpl.description || '',
      is_active: tmpl.is_active,
    };
    showEditor = true;
  }

  async function save() {
    if (!editing) return;
    try {
      await api.emailTemplates.update(editing.id, formData);
      successToast(t('settings.emailTemplates.saved'));
      showEditor = false;
      editing = null;
      await loadTemplates();
    } catch (err) {
      errorToast(t('settings.emailTemplates.saveFailed', { error: err.message }));
    }
  }

  async function preview() {
    if (!editing) return;
    previewLoading = true;
    try {
      const result = await api.emailTemplates.preview({
        name: editing.name,
        subject: formData.subject,
        html_body: formData.html_body,
        text_body: formData.text_body,
      });
      previewSubject = result.subject;
      previewHTML = result.html_body;
      previewText = result.text_body;
      showPreview = true;
    } catch (err) {
      errorToast(t('settings.emailTemplates.previewFailed', { error: err.message }));
    } finally {
      previewLoading = false;
    }
  }

  function templateLabel(name) {
    return t(`settings.emailTemplates.labels.${name}`) || name;
  }

  const columns = $derived([
    { key: 'name', label: t('settings.emailTemplates.template'), slot: 'name' },
    { key: 'description', label: t('settings.emailTemplates.description'), slot: 'description' },
    { key: 'status', label: t('settings.emailTemplates.status'), slot: 'status' },
    { key: 'actions', label: t('settings.emailTemplates.actions') },
  ]);

  function buildActions(tmpl) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('settings.emailTemplates.edit'),
        hoverClass: 'hover-bg',
        onClick: () => openEditor(tmpl),
      },
    ];
  }
</script>

<PageHeader
  icon={Mail}
  title={t('settings.emailTemplates.title')}
  subtitle={t('settings.emailTemplates.subtitle')}
/>

<DataTable
  {columns}
  data={$templates}
  keyField="id"
  emptyMessage={loading ? t('settings.emailTemplates.loading') : t('settings.emailTemplates.empty')}
  emptyIcon={Mail}
  actionItems={buildActions}
>
  {#snippet name(tmpl)}
    <button
      type="button"
      class="text-left"
      onclick={() => openEditor(tmpl)}
      style="color: var(--ds-text); font-weight: 500;"
    >
      {templateLabel(tmpl.name)}
      <div class="text-xs font-mono" style="color: var(--ds-text-subtle);">{tmpl.name}</div>
    </button>
  {/snippet}

  {#snippet description(tmpl)}
    <span style="color: var(--ds-text-subtle);">{tmpl.description || '—'}</span>
  {/snippet}

  {#snippet status(tmpl)}
    {#if tmpl.is_active}
      <Lozenge color="green" text={t('settings.emailTemplates.active')} />
    {:else}
      <Lozenge color="gray" text={t('settings.emailTemplates.inactive')} />
    {/if}
  {/snippet}
</DataTable>

<Modal isOpen={showEditor} onclose={() => (showEditor = false)} maxWidth="max-w-5xl">
  <ModalHeader
    title={editing
      ? t('settings.emailTemplates.editTitle', { name: templateLabel(editing.name) })
      : t('settings.emailTemplates.editFallbackTitle')}
    showCloseButton={false}
  />

  <div class="px-6 py-4 grid grid-cols-1 lg:grid-cols-3 gap-6">
    <div class="lg:col-span-2 flex flex-col gap-4">
      <div>
        <Label color="default" class="mb-2">{t('settings.emailTemplates.subject')}</Label>
        <input
          type="text"
          bind:value={formData.subject}
          class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
          style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
        />
      </div>

      <div>
        <Label color="default" class="mb-2">{t('settings.emailTemplates.htmlBody')}</Label>
        <Textarea
          bind:value={formData.html_body}
          rows={14}
          style="font-family: 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px;"
        />
      </div>

      <div>
        <Label color="default" class="mb-2">{t('settings.emailTemplates.textBody')}</Label>
        <Textarea
          bind:value={formData.text_body}
          rows={6}
          style="font-family: 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px;"
        />
      </div>

      <div>
        <Label color="default" class="mb-2">{t('settings.emailTemplates.adminNotes')}</Label>
        <Textarea bind:value={formData.description} rows={2} />
      </div>

      <div>
        <Checkbox bind:checked={formData.is_active} label={t('settings.emailTemplates.activeHint')} size="small" />
      </div>
    </div>

    <aside
      class="lg:col-span-1 rounded-lg p-4"
      style="background: var(--ds-surface-subtle); border: 1px solid var(--ds-border);"
    >
      <h3 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">
        {t('settings.emailTemplates.availableVariables')}
      </h3>
      <ul class="flex flex-col gap-3">
        {#each VARIABLES[editing?.name] ?? [] as v}
          <li>
            <code
              class="text-xs px-1.5 py-0.5 rounded"
              style="background: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text);"
              >{v.name}</code
            >
            <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">{v.description}</div>
          </li>
        {/each}
      </ul>
      <p class="text-xs mt-4" style="color: var(--ds-text-subtle);">
        {t('settings.emailTemplates.syntaxHint', { open: '{{', close: '}}' })}
      </p>
    </aside>
  </div>

  <div class="px-6 pb-2 flex justify-end">
    <Button variant="secondary" onclick={preview} icon={Eye} disabled={previewLoading}>
      {previewLoading ? t('settings.emailTemplates.previewLoading') : t('settings.emailTemplates.preview')}
    </Button>
  </div>

  <DialogFooter
    onCancel={() => (showEditor = false)}
    onConfirm={save}
    confirmLabel={t('settings.emailTemplates.save')}
    disabled={!formData.subject || !formData.html_body}
  />
</Modal>

<Modal isOpen={showPreview} onclose={() => (showPreview = false)} maxWidth="max-w-4xl">
  <ModalHeader
    title={t('settings.emailTemplates.previewTitle')}
    showCloseButton={true}
    onClose={() => (showPreview = false)}
  />
  <div class="px-6 py-4">
    <div class="mb-3 text-sm" style="color: var(--ds-text-subtle);">
      <strong style="color: var(--ds-text);">{t('settings.emailTemplates.previewSubject')}</strong> {previewSubject}
    </div>
    <iframe
      title={t('settings.emailTemplates.previewTitle')}
      srcdoc={previewHTML}
      sandbox=""
      style="width: 100%; height: 600px; border: 1px solid var(--ds-border); border-radius: 8px; background: #f4f5f7;"
    ></iframe>
    <details class="mt-4">
      <summary class="cursor-pointer text-sm font-medium" style="color: var(--ds-text);">
        {t('settings.emailTemplates.plainTextVersion')}
      </summary>
      <pre
        class="mt-2 p-3 rounded text-xs whitespace-pre-wrap"
        style="background: var(--ds-surface-subtle); color: var(--ds-text); border: 1px solid var(--ds-border);">{previewText}</pre>
    </details>
  </div>
</Modal>
