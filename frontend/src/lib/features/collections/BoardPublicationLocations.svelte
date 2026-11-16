<script>
  import Link from '../../components/Link.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  let { publication } = $props();
  function audience(policy) {
    if (policy.mode === 'public') return 'Anyone with the link';
    if (!policy.organisation_ids.length) return 'Nobody (empty audience intersection)';
    return policy.organisation_ids.map(id => publication.organisations?.find(org => org.id === id)?.name || `Organisation ${id}`).join(', ');
  }
</script>
<div class="space-y-2" data-testid={`public-board-locations-${publication.collection_id}`}>
  {#each publication.locations || [] as location (location.url)}
    <div>
      <Link href={location.url} target="_blank" rel="noopener">{location.label}</Link>
      <DescriptionText>{location.enabled ? 'Active' : 'Unavailable'} · {audience(location.effective_policy)}{location.kind === 'portal' ? ' · Portal registration and domain rules also apply' : ''}</DescriptionText>
      {#if location.channel_id && publication.can_administer}<Link href={`/admin/channels/${location.channel_id}/portal`} class="text-xs">Configure portal</Link>{/if}
    </div>
  {:else}<DescriptionText>No active publication locations configured.</DescriptionText>{/each}
</div>
