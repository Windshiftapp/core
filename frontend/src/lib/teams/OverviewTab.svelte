<script>
  import { IconEdit, IconCheck, IconX } from '@tabler/icons-svelte-runes';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import StatCard from '../components/StatCard.svelte';

  let { team, canEdit, onUpdated } = $props();

  let editing = $state(false);
  let formData = $state({
    name: team.name,
    description: team.description || '',
    is_active: team.is_active,
  });

  function startEdit() {
    formData = {
      name: team.name,
      description: team.description || '',
      is_active: team.is_active,
    };
    editing = true;
  }

  function cancelEdit() {
    editing = false;
  }

  async function save() {
    if (!formData.name?.trim()) {
      errorToast(t('teams.nameRequired'));
      return;
    }
    try {
      await api.teams.update(team.id, formData);
      successToast(t('teams.updated'));
      editing = false;
      await onUpdated?.();
    } catch (err) {
      errorToast(err.message || t('teams.failedToSave'));
    }
  }
</script>

<div class="space-y-6">
  {#if editing}
    <div class="space-y-4 max-w-xl">
      <div>
        <label for="overview-team-name" class="block text-sm font-medium" style="color: var(--ds-text)">
          {t('teams.name')}
        </label>
        <Input id="overview-team-name" bind:value={formData.name} required />
      </div>
      <div>
        <label for="overview-team-description" class="block text-sm font-medium" style="color: var(--ds-text)">
          {t('teams.descriptionOptional')}
        </label>
        <Textarea id="overview-team-description" bind:value={formData.description} rows={3} />
      </div>
      <div class="flex items-center gap-2">
        <input
          id="overview-team-is-active"
          type="checkbox"
          bind:checked={formData.is_active}
        />
        <label for="overview-team-is-active" class="text-sm" style="color: var(--ds-text)">
          {t('teams.active')}
        </label>
      </div>
      <div class="flex gap-2">
        <Button variant="primary" icon={IconCheck} onclick={save} dataTestid="overview-save">
          {t('common.save')}
        </Button>
        <Button variant="ghost" icon={IconX} onclick={cancelEdit}>
          {t('common.cancel')}
        </Button>
      </div>
    </div>
  {:else}
    <div class="flex items-start justify-between max-w-3xl">
      <div class="space-y-2">
        <h3 class="text-lg font-medium" style="color: var(--ds-text)" data-testid="overview-team-name">
          {team.name}
        </h3>
        <p class="text-sm" style="color: var(--ds-text-subtle)">
          {team.description || t('teams.noDescription')}
        </p>
      </div>
      {#if canEdit}
        <Button variant="ghost" size="sm" icon={IconEdit} onclick={startEdit} dataTestid="overview-edit">
          {t('common.edit')}
        </Button>
      {/if}
    </div>

    <div class="grid grid-cols-2 md:grid-cols-3 gap-4 max-w-3xl">
      <StatCard label={t('teams.directMembers')} value={team.direct_member_count ?? 0} />
      <StatCard label={t('teams.mappedGroups')} value={team.group_count ?? 0} />
      <StatCard label={t('teams.resolvedMembers')} value={team.resolved_member_count ?? 0} />
    </div>
  {/if}
</div>
