// Test management API - barrel export
export { testFolders } from './testFolders.js';
export { testLabels } from './testLabels.js';
export { testCases } from './testCases.js';
export { testSets } from './testSets.js';
export { testPlans } from './testPlans.js';
export { testRunTemplates } from './testRunTemplates.js';
export { testRuns } from './testRuns.js';
export { testResults } from './testResults.js';
export { reports } from './reports.js';
export { defects } from './defects.js';

// Aggregated tests object for backward compatibility
import { testFolders } from './testFolders.js';
import { testLabels } from './testLabels.js';
import { testCases } from './testCases.js';
import { testSets } from './testSets.js';
import { testPlans } from './testPlans.js';
import { testRunTemplates } from './testRunTemplates.js';
import { testRuns } from './testRuns.js';
import { testResults } from './testResults.js';
import { reports } from './reports.js';
import { defects } from './defects.js';

export const tests = {
  testFolders,
  testLabels,
  testCases,
  testSets,
  testPlans,
  testRunTemplates,
  testRuns,
  testResults,
  reports,
  defects,
};
