export function formatItemCount(itemCount, shownCount, t) {
  return [
    itemCount !== null ? `${itemCount} ${t('layout.items')}` : '',
    shownCount !== null && shownCount !== itemCount
      ? t('collections.itemsShown', { count: shownCount })
      : '',
  ]
    .filter(Boolean)
    .join(' · ');
}
