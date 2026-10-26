/**
 * Validate and upload a brand logo/background image through the attachments
 * API. Shared by the hub and portal customization stores.
 *
 * @param {FileList | File[] | null | undefined} files
 * @param {{
 *   category: string,
 *   entityId?: number | string | null,
 *   responseKey?: string,
 *   upload: (formData: FormData) => Promise<any>,
 * }} options
 * @returns {Promise<string | null>} the uploaded image URL, or null
 */
export async function uploadBrandImage(
  files,
  { category, entityId = null, responseKey = 'logo_url', upload }
) {
  if (!files || files.length === 0) return null;

  const file = files[0];
  if (!file.type.startsWith('image/')) {
    console.error('Please select an image file');
    return null;
  }

  const formData = new FormData();
  formData.append('file', file);
  formData.append('category', category);
  if (entityId != null) {
    formData.append('entity_id', String(entityId));
  }

  const result = await upload(formData);
  return result?.success && result[responseKey] ? result[responseKey] : null;
}
