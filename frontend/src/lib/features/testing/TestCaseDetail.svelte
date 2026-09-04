<script>
  import TestCaseViewModal from '../../dialogs/TestCaseViewModal.svelte';
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';

  let { workspaceId = null, testCaseId = null, from = null } = $props();

  function goBack() {
    const suffix = from === 'reports' ? '/reports' : '';
    navigate(`/workspaces/${workspaceId}/tests${suffix}`);
  }
</script>

<div
  class="min-h-screen p-6"
  style="background-color: var(--ds-surface);"
  data-testid="test-case-detail"
>
  <TestCaseViewModal
    isOpen={true}
    {testCaseId}
    {workspaceId}
    embedded={true}
    backLabel={from === 'reports' ? t('testing.testReportsTitle') : t('testing.backToTestCases')}
    onclose={goBack}
  />
</div>
