// Shared ItemPicker configs for entity lists that come straight from the API
// (id/name shaped). Components that transform their lists first (e.g.
// ItemDetailSidebar's label/value options) keep their own configs.

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
  primary: { text: (priority) => priority.name },
  getValue: (priority) => priority.id,
  getLabel: (priority) => priority.name,
  searchFields: ['name'],
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
    primary: { text: (status) => status.name },
    getValue: (status) => status.id,
    getLabel: (status) => status.name,
    searchFields: ['name'],
  };
}
