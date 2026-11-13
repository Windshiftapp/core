<script>
  import VirtualRows from '../../../lib/components/VirtualRows.svelte';
  import Checkbox from '../../../lib/components/Checkbox.svelte';

  let enabled = $state(true);
  let root = $state(null);
  let body = $state(null);
  let selection = $state(new Set());
  const records = Array.from({ length: 10000 }, (_, index) => ({
    key: `record:${index}`, label: `Independent record ${index + 1}`,
    status: index % 3 === 0 ? 'Ready' : 'Draft',
  }));

  function select(key, checked) {
    const next = new Set(selection);
    if (checked) next.add(key);
    else next.delete(key);
    selection = next;
  }
</script>

<section class="p-6 space-y-4">
  <h1 class="text-2xl font-semibold">VirtualRows</h1>
  <p>One shared row renderer, independent entity keys and a native table. No collection stores or fetching.</p>
  <Checkbox bind:checked={enabled} label="Virtualize loaded rows" />
  <p>{selection.size} selected across 10,000 loaded records. Selection belongs to this consumer.</p>
  <div bind:this={root} class="overflow-auto border rounded" style="height: 480px; overflow-anchor: none;">
    <table class="w-full">
      <thead><tr><th>Selected</th><th>Record</th><th>Status</th></tr></thead>
      <tbody bind:this={body}>
        <VirtualRows items={records} getKey={(record) => record.key} {enabled}
          scrollElement={root} listElement={body} identity="catalog:independent-records" estimateSize={() => 48}>
          {#snippet row(record, virtual)}
            <tr use:virtual.attach class="border-t" style="height: 48px;">
              <td class="px-3"><Checkbox checked={selection.has(record.key)} onchange={(checked) => select(record.key, checked)} label={`Select ${record.label}`} /></td>
              <td class="px-3">{record.label}</td><td class="px-3">{record.status}</td>
            </tr>
          {/snippet}
          {#snippet spacer(size)}
            <tr aria-hidden="true"><td colspan="3" style={`height: ${size}px; padding: 0; border: 0;`}></td></tr>
          {/snippet}
        </VirtualRows>
      </tbody>
    </table>
  </div>
</section>
