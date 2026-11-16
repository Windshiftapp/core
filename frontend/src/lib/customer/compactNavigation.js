const labels = {
  requests: 'My requests',
  forms: 'New request',
  'knowledge-base': 'Help',
  drafts: 'Drafts',
  approvals: 'Approvals',
  profile: 'Profile',
  'asset-reports': 'Reports',
};

export function compactModules(experience) {
  const root = `/portal/${experience.slug}`;
  const items = experience.modules.includes('boards')
    ? experience.boards.map((board) => ({
        id: `board:${board.id}`,
        label: board.label,
        href: `${root}/boards/${board.id}`,
      }))
    : [];
  for (const kind of experience.modules) {
    if (
      !labels[kind] ||
      (['drafts', 'approvals', 'profile'].includes(kind) && !experience.authenticated)
    )
      continue;
    items.push({
      id: kind,
      label: labels[kind],
      href: `${root}${kind === 'knowledge-base' ? '/kb' : kind === 'profile' ? '/profile' : `?module=${kind}`}`,
    });
  }
  const order = experience.navigation || [];
  const rank = (id) => (order.indexOf(id) < 0 ? order.length : order.indexOf(id));
  return items.sort((a, b) => rank(a.id) - rank(b.id));
}

export function compactTarget(experience, route, modules) {
  const params = route.params || {};
  const query = route.query || {};
  if (params.placementId) return { kind: `board:${params.placementId}` };
  if (params.pageId) return { kind: 'knowledge-base', articleId: Number(params.pageId) };
  const suffix = route.path.replace(/^\/portal\/[^/]+/, '');
  if (suffix === '/kb') return { kind: 'knowledge-base' };
  if (suffix === '/profile') return { kind: 'profile' };
  if (params.requestTypeId) return { kind: 'forms', formId: Number(params.requestTypeId) };
  if (query.module || ['requests', 'approvals', 'drafts'].includes(query.view))
    return { kind: query.module || query.view };
  const fallback = { kind: modules[0]?.id };
  const configured = experience.default_module || '';
  if (/^form:\d+$/.test(configured) && modules.some((item) => item.id === 'forms'))
    return { kind: 'forms', formId: Number(configured.slice(5)) };
  if (/^article:\d+$/.test(configured) && modules.some((item) => item.id === 'knowledge-base'))
    return { kind: 'knowledge-base', articleId: Number(configured.slice(8)) };
  return modules.some((item) => item.id === configured) ? { kind: configured } : fallback;
}
