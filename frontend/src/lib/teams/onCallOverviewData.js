import { onCallSchedules } from '../api/oncall.js';
/** Load the complete team on-call tab read model with one frontend request. */
export async function loadTeamOnCallOverview(teamId) {
  const response = await onCallSchedules.listForTeam(teamId);
  const schedules = Array.isArray(response) ? response : [];
  return {
    schedules,
    currentByScheduleId: new Map(
      schedules.map((schedule) => [schedule.id, schedule.current_on_call ?? null])
    ),
  };
}
