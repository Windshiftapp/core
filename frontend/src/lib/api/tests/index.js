// Test management API - barrel export

export { coverage } from './coverage.js';
export { defects } from './defects.js';
export { reports } from './reports.js';
export { testCases } from './testCases.js';
export { testFolders } from './testFolders.js';
export { testLabels } from './testLabels.js';
export { testPlans } from './testPlans.js';
export { testResults } from './testResults.js';
export { testRuns } from './testRuns.js';
export { testRunTemplates } from './testRunTemplates.js';
export { testSets } from './testSets.js';

import { coverage } from './coverage.js';
import { defects } from './defects.js';
import { reports } from './reports.js';
import { testCases } from './testCases.js';
// Aggregated tests object for backward compatibility
import { testFolders } from './testFolders.js';
import { testLabels } from './testLabels.js';
import { testPlans } from './testPlans.js';
import { testResults } from './testResults.js';
import { testRuns } from './testRuns.js';
import { testRunTemplates } from './testRunTemplates.js';
import { testSets } from './testSets.js';

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
  coverage,
};
