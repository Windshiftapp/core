import { fetchAPI } from '../core.js';
import { createCrudClient } from '../createCrudClient.js';

export const testFolders = {
  ...createCrudClient('/test-folders', { parentPath: '/workspaces' }),
  reorder: (workspaceId, data) =>
    fetchAPI(`/workspaces/${workspaceId}/test-folders/reorder`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};
