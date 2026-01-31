<script>
  import { X, Check } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import Input from '../../components/Input.svelte';
  import Button from '../../components/Button.svelte';
  import Label from '../../components/Label.svelte';
  import Select from '../../components/Select.svelte';
  import Spinner from '../../components/Spinner.svelte';

  let {
    channelId,
    formData = $bindable({
      host: '',
      port: 587,
      username: '',
      password: '',
      from_email: '',
      from_name: '',
      encryption: 'tls'
    }),
    onSave = () => {}
  } = $props();

  let testResult = $state(null);
  let testEmail = $state('');
  let loading = $state(false);

  async function testSmtpSettings() {
    if (!channelId || !formData.host || !formData.from_email) {
      testResult = { success: false, message: t('channel.smtpHostAndFromRequired') };
      return;
    }

    if (!testEmail) {
      testResult = { success: false, message: t('channel.testEmailRequired') };
      return;
    }

    testResult = { success: true, message: t('channel.sendingTestEmail'), loading: true };

    try {
      // Save config first
      const configData = getConfig();
      await api.channels.updateConfig(channelId, configData);

      // Test the channel with test email
      const result = await api.channels.testWithEmail(channelId, testEmail);
      if (result.success) {
        testResult = {
          success: true,
          message: t('channel.testEmailSent'),
          loading: false
        };
        onSave();
      } else {
        testResult = {
          success: false,
          message: `${t('channel.testEmailFailed')}: ${result.message || 'Unknown error'}`,
          loading: false
        };
      }
    } catch (error) {
      console.error('Failed to test SMTP:', error);
      testResult = {
        success: false,
        message: t('channel.testEmailFailed') + ': ' + (error.message || error),
        loading: false
      };
    }
  }

  export function validate() {
    if (!formData.host?.trim()) {
      return { valid: false, message: t('channel.smtpHostRequired') };
    }
    if (!formData.from_email?.trim()) {
      return { valid: false, message: t('channel.smtpFromEmailRequired') };
    }
    return { valid: true };
  }

  export function getConfig() {
    return {
      smtp_host: formData.host,
      smtp_port: formData.port || 587,
      smtp_username: formData.username || undefined,
      smtp_password: formData.password || undefined,
      smtp_from_email: formData.from_email,
      smtp_from_name: formData.from_name || undefined,
      smtp_encryption: formData.encryption || 'tls'
    };
  }

  export function clearSecret() {
    formData.password = '';
  }
</script>

<div class="pt-6 border-t" style="border-color: var(--ds-border);">
  <h4 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('channel.smtpConfiguration')}</h4>

  <div class="space-y-6">
    <!-- Server Settings -->
    <div class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <Label color="default" required class="mb-2">{t('channel.smtpHost')}</Label>
          <Input type="text" bind:value={formData.host} required placeholder="smtp.example.com" />
        </div>
        <div>
          <Label color="default" required class="mb-2">{t('channel.smtpPort')}</Label>
          <Input type="number" bind:value={formData.port} required placeholder="587" />
        </div>
      </div>

      <div>
        <Label color="default" class="mb-2">{t('channel.smtpEncryption')}</Label>
        <Select bind:value={formData.encryption}>
          <option value="tls">TLS (Port 587)</option>
          <option value="ssl">SSL (Port 465)</option>
          <option value="none">{t('channel.noEncryption')}</option>
        </Select>
      </div>
    </div>

    <!-- Authentication -->
    <div class="pt-4 border-t" style="border-color: var(--ds-border);">
      <h5 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">{t('channel.authentication')}</h5>
      <div class="space-y-4">
        <div>
          <Label color="default" class="mb-2">{t('channel.smtpUsername')}</Label>
          <Input type="text" bind:value={formData.username} placeholder={t('channel.smtpUsernamePlaceholder')} />
        </div>
        <div>
          <Label color="default" class="mb-2">{t('channel.smtpPassword')}</Label>
          <Input type="password" bind:value={formData.password} placeholder={t('channel.secretPlaceholder')} />
          <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
            {t('channel.leaveBlankPassword')}
          </p>
        </div>
      </div>
    </div>

    <!-- Sender Settings -->
    <div class="pt-4 border-t" style="border-color: var(--ds-border);">
      <h5 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">{t('channel.senderSettings')}</h5>
      <div class="space-y-4">
        <div>
          <Label color="default" required class="mb-2">{t('channel.smtpFromEmail')}</Label>
          <Input type="email" bind:value={formData.from_email} required placeholder="noreply@example.com" />
        </div>
        <div>
          <Label color="default" class="mb-2">{t('channel.smtpFromName')}</Label>
          <Input type="text" bind:value={formData.from_name} placeholder={t('channel.smtpFromNamePlaceholder')} />
        </div>
      </div>
    </div>
  </div>

  <!-- Test SMTP Section -->
  <div class="mt-6 pt-6 border-t" style="border-color: var(--ds-border);">
    <h5 class="text-sm font-semibold mb-4" style="color: var(--ds-text);">{t('channel.testSmtp')}</h5>
    <div class="space-y-4">
      <div>
        <Label color="default" class="mb-2">{t('channel.testEmailAddress')}</Label>
        <Input type="email" bind:value={testEmail} placeholder={t('channel.testEmailPlaceholder')} />
      </div>
      <Button onclick={testSmtpSettings} variant="secondary" disabled={!formData.host || !formData.from_email || loading}>
        {t('channel.sendTestEmail')}
      </Button>
    </div>

    {#if testResult}
      <div
        class="mt-4 p-4 rounded text-sm"
        style="background-color: {testResult.success ? 'var(--ds-background-success-subtle)' : 'var(--ds-background-danger-subtle)'}; border: 1px solid {testResult.success ? 'var(--ds-border-success)' : 'var(--ds-border-danger)'}; color: {testResult.success ? 'var(--ds-text-success)' : 'var(--ds-text-danger)'};"
      >
        {#if testResult.loading}
          <div class="flex items-center gap-2">
            <Spinner size="sm" />
            <span>{testResult.message}</span>
          </div>
        {:else}
          <div class="flex items-start gap-2">
            {#if testResult.success}
              <Check class="w-4 h-4 mt-0.5 flex-shrink-0" style="color: var(--ds-icon-success);" />
            {:else}
              <X class="w-4 h-4 mt-0.5 flex-shrink-0" style="color: var(--ds-icon-danger);" />
            {/if}
            <span>{testResult.message}</span>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
