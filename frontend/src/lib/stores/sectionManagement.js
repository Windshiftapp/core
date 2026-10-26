/**
 * Pure section-list transforms shared by the hub and portal customization
 * stores. Both keep their own reactive array and reassign it with the result.
 */

/** Append a new empty section and return the new list. */
export function addSection(sections, membershipKey, id) {
  return [
    ...sections,
    { id, title: '', subtitle: '', display_order: sections.length, [membershipKey]: [] },
  ];
}

/** Drop a section and renumber the remaining ones. */
export function removeSection(sections, sectionId) {
  return sections
    .filter((section) => section.id !== sectionId)
    .map((section, index) => ({ ...section, display_order: index }));
}

/** Patch one field on one section. */
export function updateSectionField(sections, sectionId, field, value) {
  return sections.map((section) =>
    section.id === sectionId ? { ...section, [field]: value } : section
  );
}

/** Swap a section with its neighbour. `direction` is -1 (up) or 1 (down). */
export function moveSection(sections, index, direction) {
  const target = index + direction;
  if (target < 0 || target >= sections.length) return sections;
  const next = [...sections];
  [next[index], next[target]] = [next[target], next[index]];
  return next.map((section, i) => ({ ...section, display_order: i }));
}

/** Add a member id to a section's membership list, ignoring duplicates. */
export function addSectionMember(sections, sectionId, membershipKey, id) {
  return sections.map((section) => {
    if (section.id !== sectionId) return section;
    const members = section[membershipKey] || [];
    if (members.includes(id)) return section;
    return { ...section, [membershipKey]: [...members, id] };
  });
}

/** Remove a member id from a section's membership list. */
export function removeSectionMember(sections, sectionId, membershipKey, id) {
  return sections.map((section) =>
    section.id === sectionId
      ? { ...section, [membershipKey]: (section[membershipKey] || []).filter((m) => m !== id) }
      : section
  );
}
