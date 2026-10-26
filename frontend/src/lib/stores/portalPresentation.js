export { gradients } from '../utils/gradients.js';

import Table2 from '@lucide/svelte/icons/table-2';
import { workspaceIconMap } from '../utils/icons.js';

// Icon map for portal request types and asset reports. The workspace icon set
// already covers everything the picker offers; Table2 is the asset-report
// fallback and the only addition.
export const iconMap = {
  ...workspaceIconMap,
  Table2,
};
