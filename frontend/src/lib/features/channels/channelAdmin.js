import { channels as channelsApi } from '../../api/channels.js';
import { BACKGROUND_READ } from '../../api/readScheduler.js';

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
 * Email channels whose tickets surface in a portal. An intake linked to the
 * portal (portal_channel_id) is the only link since WI-1644. The channel list
 * carries its intakes, so this resolves every mailbox in one background read
 * instead of one intake request per channel; a failed load rejects rather
 * than masquerading as "no connected mailboxes".
 */
export async function loadPortalConnectedMailboxes(portalId) {
  const emailChannels = await channelsApi.getAll(
    {
      type: 'email',
      direction: 'inbound',
      include_disabled: true,
      include_intakes: true,
    },
    BACKGROUND_READ
  );
  const channels = Array.isArray(emailChannels) ? emailChannels : [];
  return channels.filter((channel) =>
    (channel.intakes ?? []).some((intake) => intake.portal_channel_id === portalId)
  );
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
    await channelsApi.updateConfig(channel.id, configRef.getConfig());
  }

  await channelsApi.update(channel.id, {
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
    await channelsApi.toggle(channel.id);
  }
}

export async function prepareFormChannelForWorkspace({ channel, workspaceIds, workspaceId }) {
  const currentWorkspaceIds = workspaceIds || [];
  const nextWorkspaceIds = currentWorkspaceIds.includes(workspaceId)
    ? currentWorkspaceIds
    : [...currentWorkspaceIds, workspaceId];

  if (nextWorkspaceIds !== currentWorkspaceIds) {
    await channelsApi.updateConfig(channel.id, {
      form_workspace_ids: nextWorkspaceIds,
    });
  }

  let status = channel.status;
  if (status !== 'enabled') {
    const updated = await channelsApi.toggle(channel.id);
    status = updated?.status || 'enabled';
  }

  return { workspaceIds: nextWorkspaceIds, status };
}
