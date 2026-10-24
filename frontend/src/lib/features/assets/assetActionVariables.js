import { assetTypes } from '../../api/assets.js';
export const assetActionConditionFields = [
  { value: 'asset_title', label: 'Title' },
  { value: 'asset_tag', label: 'Asset Tag' },
  { value: 'asset_type_name', label: 'Type Name' },
  { value: 'asset_status_name', label: 'Status Name' },
];

export async function loadAssetActionCustomFields(assetTypeId) {
  if (!assetTypeId) return [];
  const fields = await assetTypes.getFields(assetTypeId);
  return (fields || []).map((field) => ({
    id: String(field.custom_field_id),
    name: field.field_name,
    type: field.field_type,
    description: field.field_description || '',
    isCustom: true,
  }));
}
