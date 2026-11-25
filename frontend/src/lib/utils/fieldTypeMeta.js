// Shared catalog of field types: the custom-field type picker and the type
// icon/tooltip shown wherever fields are listed (screen editor, custom fields
// admin, …). Keeping the icon, label, and color for a type in one place stops
// the type picker and the type badges from drifting apart.

import AlignLeft from '@lucide/svelte/icons/align-left';
import Banknote from '@lucide/svelte/icons/banknote';
import Box from '@lucide/svelte/icons/box';
import Building2 from '@lucide/svelte/icons/building-2';
import Calendar from '@lucide/svelte/icons/calendar';
import ChevronDownCircle from '@lucide/svelte/icons/chevron-down-circle';
import FileText from '@lucide/svelte/icons/file-text';
import Flag from '@lucide/svelte/icons/flag';
import Globe from '@lucide/svelte/icons/globe';
import Hash from '@lucide/svelte/icons/hash';
import Link2 from '@lucide/svelte/icons/link-2';
import ListChecks from '@lucide/svelte/icons/list-checks';
import Repeat from '@lucide/svelte/icons/repeat';
import CheckSquare from '@lucide/svelte/icons/square-check';
import Timer from '@lucide/svelte/icons/timer';
import Type from '@lucide/svelte/icons/type';
import User from '@lucide/svelte/icons/user';
import { BOOLEAN_CUSTOM_FIELD_TYPE, canonicalCustomFieldType } from './customFieldTypes.js';

// Custom-field types, in the order the create-field picker shows them.
export const CUSTOM_FIELD_TYPES = [
  { value: 'text', label: 'Single Line Text', icon: Type, iconColor: '#4A90D9' },
  { value: 'textarea', label: 'Multi Line Text', icon: AlignLeft, iconColor: '#5B6ABF' },
  { value: 'richtext', label: 'Rich Text (Markdown)', icon: FileText, iconColor: '#7C6FD9' },
  { value: 'select', label: 'Single Select', icon: ChevronDownCircle, iconColor: '#E8853D' },
  { value: 'multiselect', label: 'Multi Select', icon: ListChecks, iconColor: '#D46B2F' },
  { value: 'number', label: 'Number', icon: Hash, iconColor: '#4CAF7D' },
  { value: 'money', label: 'Money', icon: Banknote, iconColor: '#2F9E8F' },
  { value: 'date', label: 'Date', icon: Calendar, iconColor: '#9B6DB7' },
  { value: 'user', label: 'User', icon: User, iconColor: '#5BA4C9' },
  { value: 'multi_user', label: 'Multi User', icon: User, iconColor: '#4D9DC5' },
  { value: 'iteration', label: 'Iteration', icon: Repeat, iconColor: '#D95B5B' },
  { value: 'milestone', label: 'Milestone', icon: Flag, iconColor: '#C9A84C' },
  { value: 'multi_milestone', label: 'Multi Milestone', icon: Flag, iconColor: '#C9A84C' },
  { value: 'asset', label: 'Asset', icon: Box, iconColor: '#7B8A9E' },
  { value: 'portalcustomer', label: 'Portal Customer', icon: Globe, iconColor: '#E07BAF' },
  {
    value: 'customerorganisation',
    label: 'Customer Organisation',
    icon: Building2,
    iconColor: '#8B7EC8',
  },
  { value: BOOLEAN_CUSTOM_FIELD_TYPE, label: 'Checkbox', icon: CheckSquare, iconColor: '#2F9E8F' },
  { value: 'linking', label: 'Linking', icon: Link2, iconColor: '#3B82F6' },
];

// Types that only exist on system fields. `multi-select` is the system
// spelling of the custom `multiselect`; `duration` has no custom counterpart.
const SYSTEM_FIELD_TYPES = [
  { value: 'duration', label: 'Duration', icon: Timer, iconColor: '#7B8A9E' },
  { value: 'multi-select', label: 'Multi Select', icon: ListChecks, iconColor: '#D46B2F' },
];

const FIELD_TYPE_META = new Map(
  [...CUSTOM_FIELD_TYPES, ...SYSTEM_FIELD_TYPES].map((type) => [type.value, type])
);

// Resolve a field-type string (custom, system, or the legacy `checkbox` alias)
// to its icon/label/color entry. Returns undefined for an unknown type.
export function fieldTypeMeta(fieldType) {
  if (!fieldType) return undefined;
  return FIELD_TYPE_META.get(canonicalCustomFieldType(fieldType));
}
