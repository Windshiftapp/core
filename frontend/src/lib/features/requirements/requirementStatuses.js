/** System requirement statuses — mirrors backend models.RequirementStatuses. */
export const REQUIREMENT_STATUSES = ['draft', 'in_review', 'approved', 'deprecated'];

const STATUS_LOZENGE = {
  draft: 'grey',
  in_review: 'blue',
  approved: 'green',
  deprecated: 'red',
};

export function requirementStatusLozenge(status) {
  return STATUS_LOZENGE[status] || 'grey';
}

export function requirementStatusOptions(t) {
  return REQUIREMENT_STATUSES.map((value) => ({
    value,
    label: t(`requirements.status.${value}`),
  }));
}
