<script>
  import { onMount } from 'svelte';
  import { tests } from '../../api/tests/index.js';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import MarkdownPrintView from '../print/MarkdownPrintView.svelte';

  let { workspaceId, runId } = $props();

  let content = $state('');
  let loading = $state(true);
  let error = $state('');

  onMount(async () => {
    try {
      const summary = await tests.testRuns.getSummary(workspaceId, runId);
      content = summary.markdown;
    } catch (err) {
      console.error('Failed to load test run summary:', err);
      error = t('testing.failedToLoadSummary');
    } finally {
      loading = false;
    }
  });
</script>

<MarkdownPrintView
  {content}
  title={t('testing.testRunSummary')}
  {loading}
  {error}
  loadingLabel={t('testing.printLoading')}
  backLabel={t('testing.backToRun')}
  onback={() => navigate(`/workspaces/${workspaceId}/tests/runs/${runId}`)}
  testId="test-run-summary-print"
/>
