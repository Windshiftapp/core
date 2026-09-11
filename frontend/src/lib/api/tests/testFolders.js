import { fetchV2Data } from '../core.js';
import { createCrudClient } from '../createCrudClient.js';

export const testFolders = {
  ...createCrudClient('/test-folders', { parentPath: '/workspaces', v2: true, allV2: true }),
  reorder: (workspaceId, data) =>
    fetchV2Data(`/workspaces/${workspaceId}/test-folders/reorder`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};
