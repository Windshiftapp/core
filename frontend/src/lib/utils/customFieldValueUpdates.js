import { items } from '../api/items.js';
// Custom field updates are merged per field on the server: a patch carrying
// one field sets that field and leaves the item's other fields untouched.
// A null value clears the field.
export async function updateCustomFieldValue(itemId, fieldIdentifier, value) {
  return items.update(itemId, {
    custom_field_values: { [fieldIdentifier]: value },
  });
}
