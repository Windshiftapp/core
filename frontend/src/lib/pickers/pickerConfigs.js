// Shared ItemPicker configs for entity lists that come straight from the API
// (id/name shaped). Components that transform their lists first (e.g.
// ItemDetailSidebar's label/value options) keep their own configs.

import { objectDisplayName } from '../utils/systemLabels.js';

export const milestonePickerConfig = {
  getValue: (item) => item.id,
  getLabel: (item) => item.name,
  searchFields: ['name'],
  groupBy: null,
};

export const iterationPickerConfig = {
  getValue: (item) => item.id,
  getLabel: (item) => item.name,
  searchFields: ['name'],
  groupBy: (item) => (item.is_global ? 'Global' : 'Team'),
};

export const priorityPickerConfig = {
  icon: {
    type: 'color-dot',
    source: (priority) => priority.color || '#6b7280',
    size: 'w-2 h-2',
  },
  primary: { text: (priority) => objectDisplayName(priority, 'priority') },
  getValue: (priority) => priority.id,
  getLabel: (priority) => objectDisplayName(priority, 'priority'),
  searchFields: ['display_name', 'name'],
};

export const projectPickerConfig = {
  getValue: (project) => project.id,
  getLabel: (project) => project.name,
  searchFields: ['name'],
};

// Status dots take their color from the status category, which callers load
// separately — hence a factory instead of a constant.
export function createStatusPickerConfig(statusCategories) {
  return {
    icon: {
      type: 'color-dot',
      source: (status) => {
        const category = statusCategories.find((sc) => sc.id === status.category_id);
        return category?.color || status.category_color || '#6b7280';
      },
      size: 'w-2 h-2',
    },
    primary: { text: (status) => objectDisplayName(status, 'status') },
    getValue: (status) => status.id,
    getLabel: (status) => objectDisplayName(status, 'status'),
    searchFields: ['display_name', 'name'],
  };
}
