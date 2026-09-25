/**
 * Builds the request-type deep link a portal asset-report row action points at.
 * The value travels as a `prefill.<field_identifier>` query param; the request
 * form applies it after any draft resume. See portal request routes.
 */

export function rowActionValue(asset, action) {
  switch (action?.source) {
    case 'asset_id':
      return asset?.id ?? '';
    case 'asset_tag':
      return asset?.asset_tag ?? '';
    case 'title':
      return asset?.title ?? '';
    default:
      return '';
  }
}

export function buildRowActionHref(slug, action, asset) {
  if (!slug || !action?.request_type_id || !action?.target_field) return '#';
  const params = new URLSearchParams();
  params.set(`prefill.${action.target_field}`, String(rowActionValue(asset, action)));
  return `/portal/${slug}/request/${action.request_type_id}?${params.toString()}`;
}
