import { createCrudClient } from '../createCrudClient.js';

export const testLabels = createCrudClient('/test-labels', {
  parentPath: '/workspaces',
  v2: true,
  allV2: true,
});
