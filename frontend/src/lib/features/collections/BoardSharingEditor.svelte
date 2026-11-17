<script>
  import { boardPublications } from '../../api/boardPublications.js';
  import AlertBox from '../../components/AlertBox.svelte';
  import Button from '../../components/Button.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import CopyButton from '../../components/CopyButton.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import TextField from '../../components/TextField.svelte';
  import CustomerPolicyFields from '../../customer/CustomerPolicyFields.svelte';
  import BoardPublicationLocations from './BoardPublicationLocations.svelte';
  import { publicBaseURL } from '../../runtime/contextPath.js';

  let { collectionId, onsaved = null, oncancel = null, onbeforesave = null } = $props();
  let original = $state(null);
  let draft = $state(null);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');
  let history = $state(null);
  let historyLoading = $state(false);
  let generation = 0;

  $effect(() => {
    const id = collectionId;
    const current = ++generation;
    const controller = new AbortController();
    original = null;
    draft = null;
    history = null;
    error = '';
    loading = true;
    saving = false;
    historyLoading = false;
    boardPublications.get(id, { signal: controller.signal }).then(value => {
      if (current !== generation) return;
      original = value;
      draft = structuredClone(value);
    }).catch(err => { if (current === generation && err.name !== 'AbortError') error = err.message; })
      .finally(() => { if (current === generation) loading = false; });
    return () => {
      controller.abort();
      if (current === generation) generation++;
    };
  });

  const locked = $derived(!!original?.revoked_at);
  const invalid = $derived(draft && (
    (draft.enabled && draft.standalone_enabled && !/^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$/.test(draft.slug || '')) ||
    (draft.policy.mode === 'organisations' && draft.policy.organisation_ids.some(id => !id))
  ));

  async function mutate(operation, close = false) {
    const current = generation;
    const id = collectionId;
    saving = true;
    error = '';
    try {
      const saved = await operation(id);
      if (current !== generation) return;
      original = saved;
      draft = structuredClone(saved);
      history = null;
      onsaved?.(saved, close);
    } catch (err) {
      if (current === generation) error = err.status === 409 ? 'Sharing changed while you were editing. Cancel and reopen to load the latest version; your draft has not been saved.' : err.message;
    } finally {
      if (current === generation) saving = false;
    }
  }

  async function loadHistory() {
    const current = generation;
    historyLoading = true;
    try {
      const rows = await boardPublications.history(collectionId);
      if (current === generation) history = rows;
    } catch (err) { if (current === generation) error = err.message; }
    finally { if (current === generation) historyLoading = false; }
  }

  async function savePublication(id) {
    const current = generation;
    const publication = { revision: draft.revision, enabled: draft.enabled, standalone_enabled: draft.standalone_enabled, slug: draft.slug || null, milestone_swimlanes: draft.milestone_swimlanes, policy: draft.policy };
    const prepared = await onbeforesave?.(publication.revision);
    if (current !== generation) return;
    if (prepared?.revision != null) publication.revision = prepared.revision;
    return boardPublications.save(id, publication);
  }
</script>

<div class="space-y-5" data-testid="board-sharing-editor">
  {#if error}<AlertBox variant="error" message={error} />{/if}
  {#if loading}<Spinner />{:else if draft}
    <div>
      <h3 class="font-semibold">{original.name}</h3>
      <DescriptionText>Owner: {original.owner_name || 'Missing owner'}. Source scope: {original.scope || 'Unscoped collection'}.</DescriptionText>
    </div>
    {#if !original.instance_enabled}<AlertBox variant="warning" message="Public boards are paused on this instance. Configuration is retained; disabling remains available." />{/if}
    {#if locked}<AlertBox variant="warning" message={`Revoked on ${original.revoked_at}: ${original.revoke_reason}. An administrator must allow re-publishing.`} />{/if}
    <Checkbox id="public-board-enabled-input" dataTestid="public-board-enabled" label="Enable board publication" checked={draft.enabled} disabled={saving || (!draft.enabled && (locked || !original.instance_enabled))} onchange={value => draft.enabled = value} />
    <Checkbox id="public-board-standalone-input" dataTestid="public-board-standalone" label="Available at a standalone board URL" checked={draft.standalone_enabled} disabled={saving || locked} onchange={value => draft.standalone_enabled = value} />
    <TextField id="public-board-slug" dataTestid="public-board-slug" label="Board URL slug" value={draft.slug || ''} disabled={saving || locked} oninput={event => draft.slug = event.currentTarget.value.trim()} helper="3–64 lowercase letters, numbers, and hyphens. Existing URLs remain valid until you change the slug." />
    {#if original.enabled && original.standalone_enabled && original.slug}
      <div class="flex flex-wrap gap-2">
        <CopyButton getText={() => `${publicBaseURL()}/board/${original.slug}`} label="Copy link" />
        <Button href={`${publicBaseURL()}/board/${original.slug}`} target="_blank" dataTestid="public-board-preview">Open shared board</Button>
      </div>
    {/if}
    <CustomerPolicyFields bind:policy={draft.policy} disabled={saving || locked} />
    <section class="space-y-2"><h4 class="font-medium">Used by / current effective access</h4><BoardPublicationLocations publication={original} /></section>
    <DescriptionText>{draft.policy.mode === 'public' && draft.standalone_enabled ? 'Anyone with the standalone link can view this board when publication is enabled.' : 'Every placement must also satisfy this board audience. An empty restricted allowlist admits nobody.'}</DescriptionText>
    <Checkbox id="public-board-milestone-swimlanes-input" dataTestid="public-board-milestone-swimlanes" label="Milestone swimlanes" checked={draft.milestone_swimlanes} disabled={saving || locked} onchange={value => draft.milestone_swimlanes = value} />
    <div class="flex flex-wrap justify-end gap-2">
      <Button dataTestid="public-board-cancel" disabled={saving} onclick={() => oncancel?.()}>Cancel</Button>
      <Button variant="primary" dataTestid="public-board-save" loading={saving} disabled={!!invalid || locked} onclick={() => mutate(savePublication, true)}>Save sharing</Button>
    </div>
    <Button dataTestid="public-board-history" loading={historyLoading} onclick={loadHistory}>Sharing history</Button>
    {#if history}
      <div class="space-y-3" data-testid="public-board-history-list">
        {#each history as event (event.id)}
          <div class="rounded border border-ds-border p-3 text-sm">
            <p>{event.actor_name || 'Deleted actor'} · {event.created_at} · {event.action}</p>
            {#if event.reason}<p>{event.reason}</p>{/if}
            <details><summary>Before / after</summary><pre class="overflow-auto whitespace-pre-wrap">{JSON.stringify({ before: event.before, after: event.after }, null, 2)}</pre></details>
          </div>
        {:else}<DescriptionText>No recorded sharing history is available.</DescriptionText>{/each}
      </div>
    {/if}
  {/if}
</div>
