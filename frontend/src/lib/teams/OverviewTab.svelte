<script>
  import { IconEdit, IconCheck, IconX, IconUser, IconUsers, IconStack2 } from '@tabler/icons-svelte-runes';
  import { Package } from 'lucide-svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { workspaceIconMap } from '../utils/icons.js';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import StatCard from '../components/StatCard.svelte';
  import Card from '../components/Card.svelte';
  import Label from '../components/Label.svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import AvatarUpload from '../components/AvatarUpload.svelte';

  let { team, canEdit, onUpdated } = $props();

  let editing = $state(false);
  let resolvedCount = $state(null);
  let formData = $state({
    name: team.name,
    description: team.description || '',
    is_active: team.is_active,
    icon: team.icon || 'Users',
    color: team.color || '#3b82f6',
    avatar_url: team.avatar_url || '',
  });

  async function loadResolved() {
    try {
      const members = (await api.teams.getResolvedMembers(team.id)) || [];
      resolvedCount = members.length;
    } catch {
      resolvedCount = null;
    }
  }

  // Re-fetch the resolved count whenever the team's composition changes
  // (i.e. members or groups added/removed via other tabs and the parent
  // page reloads the team object).
  $effect(() => {
    void team.direct_member_count;
    void team.group_count;
    loadResolved();
  });

  function startEdit() {
    formData = {
      name: team.name,
      description: team.description || '',
      is_active: team.is_active,
      icon: team.icon || 'Users',
      color: team.color || '#3b82f6',
      avatar_url: team.avatar_url || '',
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

  function handleIconChange(event) {
    formData.icon = event.detail.icon;
    formData.color = event.detail.color;
  }

  // Persist avatar changes immediately so the upload result survives even
  // if the user cancels the surrounding name/description edit.
  async function persistIdentity(patch) {
    try {
      await api.teams.update(team.id, {
        name: team.name,
        description: team.description || '',
        is_active: team.is_active,
        icon: team.icon || 'Users',
        color: team.color || '#3b82f6',
        avatar_url: team.avatar_url || '',
        ...patch,
      });
      await onUpdated?.();
    } catch (err) {
      errorToast(err.message || t('teams.failedToSave'));
    }
  }

  function onAvatarUploaded(url) {
    formData.avatar_url = url;
    persistIdentity({ avatar_url: url });
  }

  function onAvatarRemoved() {
    formData.avatar_url = '';
    persistIdentity({ avatar_url: '' });
  }

  const TeamIcon = $derived(workspaceIconMap[team.icon] || Package);
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
      <div>
        <IconSelector
          selectedIcon={formData.icon}
          selectedColor={formData.color}
          label={t('teams.iconAndColor')}
          compact={true}
          onchange={handleIconChange}
        />
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
      <div class="flex items-start gap-3">
        {#if team.avatar_url}
          <img src={team.avatar_url} alt="{team.name} avatar" class="w-12 h-12 rounded-md object-cover" />
        {:else}
          <div class="w-12 h-12 rounded-md flex items-center justify-center" style="background-color: {team.color || '#3b82f6'};">
            <TeamIcon size={24} color="white" />
          </div>
        {/if}
        <div class="space-y-1">
          <h3 class="text-lg font-medium" style="color: var(--ds-text)" data-testid="overview-team-name">
            {team.name}
          </h3>
          <p class="text-sm" style="color: var(--ds-text-subtle)">
            {team.description || t('teams.noDescription')}
          </p>
        </div>
      </div>
      {#if canEdit}
        <Button variant="ghost" size="sm" icon={IconEdit} onclick={startEdit} dataTestid="overview-edit">
          {t('common.edit')}
        </Button>
      {/if}
    </div>

    <div class="grid grid-cols-2 md:grid-cols-3 gap-4 max-w-3xl">
      <StatCard icon={IconUser} color="blue" label={t('teams.directMembers')} value={team.direct_member_count ?? 0} />
      <StatCard icon={IconStack2} color="purple" label={t('teams.mappedGroups')} value={team.group_count ?? 0} />
      <StatCard icon={IconUsers} color="green" label={t('teams.resolvedMembers')} value={resolvedCount ?? '—'} />
    </div>

    {#if canEdit}
      <Card rounded="xl" shadow padding="spacious">
        <h3 class="text-base font-medium mb-2" style="color: var(--ds-text);">{t('teams.identityTitle')}</h3>
        <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{t('teams.identitySubtitle')}</p>

        <div class="grid grid-cols-1 lg:grid-cols-2 gap-8">
          <div>
            <Label class="mb-2">{t('teams.iconAndColor')}</Label>
            <IconSelector
              selectedIcon={formData.icon}
              selectedColor={formData.color}
              compact={true}
              onchange={(e) => {
                handleIconChange(e);
                persistIdentity({ icon: e.detail.icon, color: e.detail.color });
              }}
            />
          </div>

          <div>
            <AvatarUpload
              avatarUrl={formData.avatar_url}
              category="team_avatar"
              itemId={team.id}
              fallbackIcon={TeamIcon}
              fallbackColor={formData.color}
              label={t('teams.avatar')}
              onUploaded={onAvatarUploaded}
              onRemoved={onAvatarRemoved}
            />
          </div>
        </div>
      </Card>
    {/if}
  {/if}
</div>
