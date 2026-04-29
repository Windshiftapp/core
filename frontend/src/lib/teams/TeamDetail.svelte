<script>
  import { onMount } from 'svelte';
  import { IconUsersGroup, IconUsers, IconStack2, IconBellRinging, IconArrowLeft } from '@tabler/icons-svelte-runes';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { authStore, permissionStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import PageHeader from '../layout/PageHeader.svelte';
  import Button from '../components/Button.svelte';
  import Tabs from '../components/Tabs.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import OverviewTab from './OverviewTab.svelte';
  import MembersTab from './MembersTab.svelte';
  import GroupsTab from './GroupsTab.svelte';
  import OnCallTab from './OnCallTab.svelte';

  let { teamId, section = 'overview' } = $props();

  let team = $state(null);
  let myRole = $state(null);
  let loading = $state(true);
  let error = $state('');

  const hasGlobalManage = $derived(permissionStore.hasPermissionKey('teams.manage'));
  const canEdit = $derived(hasGlobalManage || myRole === 'admin');

  async function loadTeam() {
    loading = true;
    try {
      team = await api.teams.get(Number(teamId));
      error = '';
    } catch (err) {
      error = err.message || t('teams.failedToLoad');
      team = null;
    } finally {
      loading = false;
    }
  }

  async function loadMyRole() {
    const userId = $authStore.currentUser?.id;
    if (!userId) return;
    try {
      const myTeams = await api.teams.getTeamsForUser(userId);
      const match = (myTeams || []).find((t) => t.id === Number(teamId));
      myRole = match?.role || null;
    } catch (err) {
      myRole = null;
    }
  }

  function changeSection(tab) {
    navigate(`/teams/${teamId}/${tab.tab}`);
  }

  const tabs = $derived([
    { id: 'overview', label: t('teams.tabs.overview'), icon: IconUsersGroup, testid: 'team-tab-overview' },
    { id: 'members', label: t('teams.tabs.members'), icon: IconUsers, badge: team?.direct_member_count, testid: 'team-tab-members' },
    { id: 'groups', label: t('teams.tabs.groups'), icon: IconStack2, badge: team?.group_count, testid: 'team-tab-groups' },
    { id: 'on-call', label: t('teams.tabs.onCall'), icon: IconBellRinging, testid: 'team-tab-on-call' },
  ]);

  let activeTab = $state(section);
  $effect(() => {
    activeTab = section;
  });

  onMount(() => {
    loadTeam();
    loadMyRole();
  });

  function reload() {
    return loadTeam();
  }
</script>

<div class="space-y-4" data-testid="team-detail">
  <Button
    variant="ghost"
    size="sm"
    icon={IconArrowLeft}
    onclick={() => navigate('/teams')}
  >
    {t('teams.backToTeams')}
  </Button>

  {#if loading}
    <div class="text-center py-8" style="color: var(--ds-text-subtle)">
      {t('teams.loading')}
    </div>
  {:else if error}
    <AlertBox message={error} />
  {:else if team}
    <PageHeader
      icon={IconUsersGroup}
      title={team.name}
      subtitle={team.description || t('teams.noDescription')}
    />

    <Tabs {tabs} bind:activeTab onTabChange={changeSection}>
      {#if activeTab === 'overview'}
        <OverviewTab {team} {canEdit} onUpdated={reload} />
      {:else if activeTab === 'members'}
        <MembersTab {team} {canEdit} onUpdated={reload} />
      {:else if activeTab === 'groups'}
        <GroupsTab {team} canEdit={hasGlobalManage || myRole === 'admin'} onUpdated={reload} />
      {:else if activeTab === 'on-call'}
        <OnCallTab {team} {canEdit} />
      {/if}
    </Tabs>
  {/if}
</div>
