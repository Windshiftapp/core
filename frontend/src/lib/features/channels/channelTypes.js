import {
  IconForms,
  IconLifebuoy,
  IconMail,
  IconSend,
  IconStack2,
  IconWebhook,
  IconWorld,
} from '@tabler/icons-svelte-runes';

export const channelTypes = [
  {
    id: 'portal',
    icon: IconWorld,
    navColor: 'from-green-400 to-green-600',
    formColor: 'var(--ds-icon-accent-green)',
  },
  {
    id: 'form',
    icon: IconForms,
    navColor: 'from-teal-400 to-teal-600',
    formColor: 'var(--ds-icon-accent-teal)',
  },
  {
    id: 'webhook',
    icon: IconWebhook,
    navColor: 'from-purple-400 to-purple-600',
    formColor: 'var(--ds-icon-accent-purple)',
  },
  {
    id: 'email',
    icon: IconMail,
    navColor: 'from-blue-400 to-blue-600',
    formColor: 'var(--ds-icon-accent-blue)',
  },
  {
    id: 'smtp',
    icon: IconSend,
    navColor: 'from-orange-400 to-orange-600',
    formColor: 'var(--ds-icon-accent-orange)',
  },
];

export const allTypesEntry = { id: null, icon: IconStack2, navColor: 'from-gray-400 to-gray-600' };

export function getChannelTypeIcon(type) {
  return channelTypes.find((ct) => ct.id === type)?.icon ?? IconLifebuoy;
}
