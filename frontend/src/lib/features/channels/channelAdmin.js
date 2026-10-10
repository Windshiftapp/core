import { api } from '../../api.js';

export function parseChannelConfig(config) {
  if (config == null) return {};
  if (typeof config === 'string') {
    if (config.trim() === '') return {};
    try {
      const parsed = JSON.parse(config);
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
        throw new Error('Channel configuration must be a JSON object');
      }
      return parsed;
    } catch (error) {
      throw new Error('Channel configuration is invalid JSON', { cause: error });
    }
  }
  if (Array.isArray(config) || typeof config !== 'object') {
    throw new Error('Channel configuration must be a JSON object');
  }
  return config;
}

/**
 * Email channels whose tickets surface in a portal. Intakes linked to the
 * portal (portal_channel_id) own the link since WI-1644; the legacy
 * email_connected_portal_id is kept as a fallback so links that predate the
 * migration still render.
 */
export async function loadPortalConnectedMailboxes(portalId) {
  const emailChannels = await api.channels.getAll({
    type: 'email',
    direction: 'inbound',
    include_disabled: true,
  });
  const channels = Array.isArray(emailChannels) ? emailChannels : [];
  const linked = [];
  for (const channel of channels) {
    if (parseChannelConfig(channel.config)?.email_connected_portal_id === portalId) {
      linked.push(channel);
      continue;
    }
    let intakes = [];
    try {
      intakes = (await api.channelIntakes.list(channel.id)) ?? [];
    } catch {
      intakes = [];
    }
    if (intakes.some((intake) => intake.portal_channel_id === portalId)) {
      linked.push(channel);
    }
  }
  return linked;
}

export function channelBasicFormData(channel) {
  return {
    name: channel?.name || '',
    description: channel?.description || '',
    category_id: channel?.category_id || null,
  };
}

export async function saveChannelSettings({ channel, channelFormData, configRef, enabled }) {
  // Save config first: it is the step most likely to be rejected, and
  // committing basic information before a config failure would leave a
  // partial save behind.
  if (configRef) {
    await api.channels.updateConfig(channel.id, configRef.getConfig());
  }

  await api.channels.update(channel.id, {
    id: channel.id,
    type: channel.type,
    direction: channel.direction,
    is_default: channel.is_default,
    name: channelFormData.name,
    description: channelFormData.description,
    category_id: channelFormData.category_id,
  });

  const currentlyEnabled = channel.status === 'enabled';
  if (typeof enabled === 'boolean' && enabled !== currentlyEnabled) {
    await api.channels.toggle(channel.id);
  }
}

export async function prepareFormChannelForWorkspace({ channel, workspaceIds, workspaceId }) {
  const currentWorkspaceIds = workspaceIds || [];
  const nextWorkspaceIds = currentWorkspaceIds.includes(workspaceId)
    ? currentWorkspaceIds
    : [...currentWorkspaceIds, workspaceId];

  if (nextWorkspaceIds !== currentWorkspaceIds) {
    await api.channels.updateConfig(channel.id, {
      form_workspace_ids: nextWorkspaceIds,
    });
  }

  let status = channel.status;
  if (status !== 'enabled') {
    const updated = await api.channels.toggle(channel.id);
    status = updated?.status || 'enabled';
  }

  return { workspaceIds: nextWorkspaceIds, status };
}
