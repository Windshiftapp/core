import { milestones as milestonesApi } from '../../api/milestones.js';

/** Load test statistics for every visible milestone with one batch request. */
export async function loadMilestoneTestStatistics(milestones) {
  const ids = [
    ...new Set(
      (Array.isArray(milestones) ? milestones : [])
        .map((milestone) => milestone?.id)
        .filter((id) => id != null)
    ),
  ];
  if (ids.length === 0) return {};
  const statistics = await milestonesApi.getTestStatisticsMany(ids);
  return statistics && typeof statistics === 'object' && !Array.isArray(statistics)
    ? statistics
    : {};
}
