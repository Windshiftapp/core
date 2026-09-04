// Test management API - barrel export

import { coverage } from './coverage.js';
import { defects } from './defects.js';
import { reports } from './reports.js';
import { testCases } from './testCases.js';
import { testFolders } from './testFolders.js';
import { testLabels } from './testLabels.js';
import { testPlans } from './testPlans.js';
import { testResults } from './testResults.js';
import { testRuns } from './testRuns.js';
import { testRunTemplates } from './testRunTemplates.js';

export const tests = {
  testFolders,
  testLabels,
  testCases,
  testPlans,
  testRunTemplates,
  testRuns,
  testResults,
  reports,
  defects,
  coverage,
};
