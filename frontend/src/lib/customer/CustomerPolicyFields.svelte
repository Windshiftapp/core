<script>
  import CustomerOrganisationPicker from '../pickers/CustomerOrganisationPicker.svelte';
  import Button from '../components/Button.svelte';
  import SelectField from '../components/SelectField.svelte';
  import AlertBox from '../components/AlertBox.svelte';

  let { policy = $bindable({ mode: 'public', organisation_ids: [] }), disabled = false, organisations = null } = $props();
</script>

<section class="space-y-3" data-testid="customer-policy-fields">
  <SelectField id="customer-policy-mode" label="Customer access" bind:value={policy.mode} {disabled} options={[
    { value: 'public', label: 'No additional audience restriction' },
    { value: 'organisations', label: 'Selected Windshift customer organisations' },
  ]} />
  {#if policy.mode === 'organisations'}
    {#each policy.organisation_ids as id, index (index)}
      <div class="flex items-center gap-2">
        <div class="min-w-0 flex-1"><CustomerOrganisationPicker id={`customer-policy-organisation-${index}`} value={id} {disabled} {organisations} onSelect={item => policy.organisation_ids = policy.organisation_ids.map((value, i) => i === index ? item?.id ?? null : value)} /></div>
        <Button variant="danger-ghost" {disabled} dataTestid={`customer-policy-remove-${index}`} onclick={() => policy.organisation_ids = policy.organisation_ids.filter((_, i) => i !== index)}>Remove</Button>
      </div>
    {/each}
    <!-- shortcut-guard-exempt: Appends an allowlist field inside this form, not a resource creation action; standard button keyboard activation avoids duplicate creation shortcuts. -->
    <Button variant="ghost" {disabled} dataTestid="customer-policy-add" onclick={() => policy.organisation_ids = [...policy.organisation_ids, null]}>Add organisation</Button>
    {#if policy.organisation_ids.length === 0}<AlertBox variant="warning" message="No organisations selected: nobody can access this resource." />{/if}
  {/if}
</section>
