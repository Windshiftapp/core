<script>
  import DataTable from '../../components/DataTable.svelte';
  import { truncateText } from './format-utils.js';

  /**
   * Recent-failures table shared by the diagnostics sections. The error cell
   * truncates and exposes the full message via the title attribute.
   *
   * @type {{
   *   columns: Array<any>,
   *   rows: Array<any>,
   *   emptyMessage: string,
   * }}
   */
  let { columns, rows, emptyMessage } = $props();
</script>

<DataTable {columns} data={rows} keyField="id" {emptyMessage}>
  {#snippet errorMessage(row)}
    <span style="color: var(--ds-text);" title={row.error_message}>{truncateText(row.error_message, 80) || '—'}</span>
  {/snippet}
</DataTable>
