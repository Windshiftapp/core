/**
 * Section-editing state machine for dashboard customize mode.
 *
 * The personal Homepage and the workspace Welcome keep their dashboards
 * differently — a store versus local state with a debounced save — so
 * persistence and the value mappers are injected. This owns only the editing
 * state and the transitions between them, which both pages used to duplicate.
 *
 * @param {{
 *   updateSection: (sectionId: string, values: { title: string, subtitle: string }) => void,
 *   removeSection: (sectionId: string) => void,
 *   findSection: (sectionId: string) => { id: string } | undefined,
 *   getSectionDisplay: (section: object) => { title: string, subtitle?: string },
 *   getSectionSaveValues: (
 *     section: object,
 *     draft: { title: string, subtitle: string }
 *   ) => { title: string, subtitle: string },
 * }} config
 */
export function useDashboardCustomization({
  updateSection,
  removeSection,
  findSection,
  getSectionDisplay,
  getSectionSaveValues,
}) {
  let editingSectionId = $state(null);
  let editingSectionTitle = $state('');
  let editingSectionSubtitle = $state('');
  let isNewSection = $state(false);

  /** Enter edit mode on a section that was just created. */
  function beginNewSection(section) {
    editingSectionId = section.id;
    editingSectionTitle = section.title;
    editingSectionSubtitle = section.subtitle || '';
    isNewSection = true;
  }

  /** Enter edit mode on an existing section. */
  function startEditing(section) {
    const display = getSectionDisplay(section);
    editingSectionId = section.id;
    editingSectionTitle = display.title;
    editingSectionSubtitle = display.subtitle || '';
    isNewSection = false;
  }

  function save() {
    if (!editingSectionId) return;
    const draft = { title: editingSectionTitle, subtitle: editingSectionSubtitle };
    const section = findSection(editingSectionId);
    updateSection(editingSectionId, section ? getSectionSaveValues(section, draft) : draft);
    resetDraft();
  }

  function cancel() {
    if (isNewSection && editingSectionId) {
      removeSection(editingSectionId);
    }
    resetDraft();
  }

  /**
   * Leaving edit mode with an unsaved new section keeps it when the user typed
   * a title and discards it otherwise.
   */
  function commitOrDiscardDraft() {
    if (isNewSection && editingSectionId) {
      if (editingSectionTitle.trim()) {
        save();
        return;
      }
      removeSection(editingSectionId);
    }
    resetDraft();
  }

  function resetDraft() {
    editingSectionId = null;
    isNewSection = false;
  }

  /** Enter saves the draft, Escape cancels it. */
  function handleSectionEditKeydown(event) {
    if (event.key === 'Enter') {
      event.preventDefault();
      save();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      cancel();
    }
  }

  return {
    get editingSectionId() {
      return editingSectionId;
    },
    get editingSectionTitle() {
      return editingSectionTitle;
    },
    set editingSectionTitle(value) {
      editingSectionTitle = value;
    },
    get editingSectionSubtitle() {
      return editingSectionSubtitle;
    },
    set editingSectionSubtitle(value) {
      editingSectionSubtitle = value;
    },
    get isNewSection() {
      return isNewSection;
    },
    beginNewSection,
    startEditing,
    save,
    cancel,
    commitOrDiscardDraft,
    resetDraft,
    handleSectionEditKeydown,
  };
}
