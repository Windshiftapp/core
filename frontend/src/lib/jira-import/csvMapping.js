const aliases = {
  'issue key': 'key',
  summary: 'title',
  description: 'description',
  'issue type': 'type',
  priority: 'priority',
  status: 'status',
  'due date': 'due',
  'custom field (story points)': 'points',
  comment: 'comments',
};
export const normalizeHeader = (v) => v.trim().toLowerCase();
export function suggestColumns(headers, deployment, fields) {
  const names = headers.map(normalizeHeader);
  return names.map((name) => {
    if (name !== 'comment' && names.filter((n) => n === name).length > 1) return '';
    if (Object.hasOwn(aliases, name)) return aliases[name];
    if (deployment === 'cloud' && ['reporter id', 'assignee id'].includes(name))
      return name.split(' ')[0];
    if (deployment === 'dc') {
      for (const role of ['reporter', 'assignee']) {
        if (name === `${role} id`) return `${role}-key`;
        if (name === role) return `${role}-name`;
      }
    }
    const label = name.match(/^custom field \((.+)\)$/)?.[1];
    if (!label) return '';
    const matches = Object.entries(fields).filter(
      ([k, f]) => k.startsWith('cf:') && normalizeHeader(f.name) === label
    );
    return matches.length === 1 ? matches[0][0] : '';
  });
}
export function workspaceSuggestion(staged, keyColumn) {
  const names = staged.headers.map(normalizeHeader);
  const sample = (name) =>
    staged.preview_rows?.find((r) => r[names.indexOf(name)])?.[names.indexOf(name)];
  const issue = staged.preview_rows?.find((r) => r[keyColumn])?.[keyColumn];
  return {
    name: sample('project name') || '',
    key: sample('project key') || issue?.match(/^(.+)-\d+$/)?.[1] || '',
  };
}
