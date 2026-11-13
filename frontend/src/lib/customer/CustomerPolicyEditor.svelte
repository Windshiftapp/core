<script>
  import { customerAccess } from '../api/customerAccess.js';
  import CustomerOrganisationPicker from '../pickers/CustomerOrganisationPicker.svelte';
  import Button from '../components/Button.svelte';
  import Select from '../components/Select.svelte';
  import AlertBox from '../components/AlertBox.svelte';

  let { kind, resourceId } = $props();
  let mode = $state('public');
  let organisationIds = $state([]);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');
  let saved = $state(false);

  $effect(() => {
    const id = resourceId;
    const type = kind;
    let active = true;
    loading = true;
    error = '';
    customerAccess.getPolicy(type, id).then(policy => {
      if (active) { mode = policy.mode; organisationIds = policy.organisation_ids; }
    }).catch(err => { if (active) error = err.message; })
      .finally(() => { if (active) loading = false; });
    return () => { active = false; };
  });

  async function save() {
    saving = true;
    error = '';
    saved = false;
    try {
      const policy = await customerAccess.savePolicy(kind, resourceId, { mode, organisation_ids: organisationIds.filter(id => id > 0) });
      mode = policy.mode;
      organisationIds = policy.organisation_ids;
      saved = true;
    } catch (err) { error = err.message; }
    finally { saving = false; }
  }
</script>

<section class="space-y-3 border-t border-ds-border pt-4" data-testid="customer-policy-editor">
  <h3 class="font-medium">Customer access</h3>
  <p class="text-sm text-ds-text-subtle">Restrict admission independently of request types and content visibility. Backend sign-in does not grant customer access.</p>
  {#if error}<AlertBox variant="error" message={error} />{/if}
  {#if !loading}
    <Select id="customer-policy-mode" bind:value={mode} disabled={saving} options={[
      { value: 'public', label: 'Public — existing access rules' },
      { value: 'organisations', label: 'Selected customer organisations' },
    ]} onchange={() => saved = false} />
    {#if mode === 'organisations'}
      {#each organisationIds as id, index (index)}
        <div class="flex gap-2 items-center">
          <div class="flex-1"><CustomerOrganisationPicker id={`customer-policy-organisation-${index}`} value={id} disabled={saving} onSelect={item => { organisationIds[index] = item?.id ?? null; saved = false; }} /></div>
          <Button variant="subtle" disabled={saving} dataTestid={`customer-policy-remove-${index}`} onclick={() => { organisationIds = organisationIds.filter((_, i) => i !== index); saved = false; }}>Remove</Button>
        </div>
      {/each}
      <!-- shortcut-guard-exempt: Repeatable allowlist row inside another settings screen or dialog; avoid claiming its global Add shortcut. -->
      <Button variant="subtle" disabled={saving} dataTestid="customer-policy-add" onclick={() => { organisationIds = [...organisationIds, null]; saved = false; }}>Add organisation</Button>
      {#if organisationIds.length === 0}<AlertBox type="warning">No organisations selected: nobody can access this resource.</AlertBox>{/if}
    {/if}
    <Button variant="secondary" dataTestid="customer-policy-save" disabled={saving || (mode === 'organisations' && organisationIds.some(id => !id))} onclick={save}>{saving ? 'Saving…' : 'Save customer access'}</Button>
    {#if saved}<p role="status" data-testid="customer-policy-saved">Customer access saved.</p>{/if}
  {/if}
</section>
