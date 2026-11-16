<script>
  import { portalBoards } from '../../api/portalBoards.js';
  import { boardPublications } from '../../api/boardPublications.js';
  import AlertBox from '../../components/AlertBox.svelte';
  import Button from '../../components/Button.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import TextField from '../../components/TextField.svelte';
  import SelectField from '../../components/SelectField.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import CustomerPolicyFields from '../../customer/CustomerPolicyFields.svelte';
  import Link from '../../components/Link.svelte';

  let { channelId } = $props();
  let placements = $state([]);
  let candidates = $state([]);
  let draft = $state(null);
  let error = $state('');
  let busy = $state(false);
  let query = $state('');
  let refresh = $state(0);

  $effect(() => {
    const id = channelId;
    refresh;
    let active = true;
    error = '';
    portalBoards.list(id).then(rows => { if (active) placements = rows; }).catch(err => { if (active) error = err.message; });
    return () => { active = false; };
  });
  $effect(() => {
    const controller = new AbortController();
    boardPublications.list({ q: query, page_size: 25 }, { signal: controller.signal }).then(result => { if (!controller.signal.aborted) candidates = result.data; })
      .catch(err => { if (!controller.signal.aborted) error = err.message; });
    return () => controller.abort();
  });

  async function save() {
    busy = true;
    error = '';
    try {
      await portalBoards.save(channelId, { ...draft, collection_id: Number(draft.collection_id), display_order: Number(draft.display_order) });
      draft = null;
      refresh++;
    } catch (err) { error = err.status === 409 ? 'This placement changed. Reopen it before saving.' : err.message; }
    finally { busy = false; }
  }
  async function remove(placement) {
    busy = true;
    error = '';
    try { await portalBoards.remove(channelId, placement); refresh++; }
    catch (err) { error = err.message; }
    finally { busy = false; }
  }
</script>

<section class="space-y-3 border-t border-ds-border pt-4" data-testid="portal-board-placements">
  <h4 class="font-semibold">Content / Navigation — boards</h4>
  <DescriptionText>Link an existing publication without cloning its collection. Portal admission, the board audience, and this placement audience must all allow the visitor. Linking never changes standalone availability.</DescriptionText>
  {#if error}<AlertBox variant="error" message={error} />{/if}
  {#each placements as placement (placement.id)}
    <div class="flex flex-wrap items-center justify-between gap-2 rounded border border-ds-border p-3" data-testid={`portal-board-placement-${placement.id}`}>
      <div>
        <p class="font-medium">{placement.label}</p>
        <Link href={`/collections/${placement.collection_id}/board/configure`}>{placement.source_name || `Board ${placement.collection_id}`} settings</Link>
        <DescriptionText>{placement.enabled ? 'Enabled' : 'Disabled'} · placement audience: {placement.policy.mode} · landing key board:{placement.id}</DescriptionText>
        {#if placement.effective_policy}<DescriptionText>Effective audience: {placement.effective_policy.mode === 'public' ? 'Anyone admitted by this portal' : placement.effective_policy.organisation_ids.length ? `Organisations ${placement.effective_policy.organisation_ids.join(', ')}` : 'Nobody (empty audience intersection)'}</DescriptionText>{/if}
        {#if placement.standalone_enabled && placement.baseline_policy?.mode === 'public'}<DescriptionText>The standalone location has an independently unrestricted audience when publication is enabled.</DescriptionText>{/if}
      </div>
      <div class="flex gap-2"><Button disabled={busy} dataTestid={`portal-board-edit-${placement.id}`} onclick={() => draft = structuredClone($state.snapshot(placement))}>Configure</Button><Button variant="danger-ghost" disabled={busy} dataTestid={`portal-board-remove-${placement.id}`} onclick={() => remove(placement)}>Unlink</Button></div>
    </div>
  {/each}
  {#if !draft}
    <!-- shortcut-guard-exempt: Opens a local unsaved placement draft within channel configuration, not a global create flow; accessible with normal button keyboard activation. -->
    <Button dataTestid="portal-board-add" onclick={() => draft = { collection_id: '', label: '', display_order: placements.length, enabled: true, revision: 0, policy: { mode: 'public', organisation_ids: [] } }}>Add board</Button>
  {:else}
    <div class="space-y-3 rounded border border-ds-border p-4" data-testid="portal-board-editor">
      {#if !draft.id}
        <TextField id="portal-board-search" dataTestid="portal-board-search" label="Search eligible publications" bind:value={query} />
        <SelectField id="portal-board-source" label="Board publication" bind:value={draft.collection_id} options={[{ value: '', label: 'Select a board' }, ...candidates.map(board => ({ value: String(board.collection_id), label: board.name }))]} />
      {/if}
      <TextField id="portal-board-label" dataTestid="portal-board-label" label="Navigation label" bind:value={draft.label} />
      <TextField id="portal-board-order" dataTestid="portal-board-order" label="Display order" type="number" bind:value={draft.display_order} />
      <Checkbox id="portal-board-enabled" dataTestid="portal-board-enabled" label="Enable placement" checked={draft.enabled} onchange={enabled => draft.enabled = enabled} />
      <CustomerPolicyFields bind:policy={draft.policy} disabled={busy} />
      <div class="flex justify-end gap-2"><Button disabled={busy} dataTestid="portal-board-cancel" onclick={() => draft = null}>Cancel</Button><Button variant="primary" loading={busy} disabled={!draft.collection_id || !draft.label.trim()} dataTestid="portal-board-save" onclick={save}>Save placement</Button></div>
    </div>
  {/if}
</section>
