import { createCrudClient } from '../createCrudClient.js';

export const testLabels = createCrudClient('/test-labels', { parentPath: '/workspaces' });
