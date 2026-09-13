// Cross-component trigger for the shell-mounted mobile command palette. Any
// header button can call mobilePalette.open(); MobileCommandPalette renders
// from this shared state, so no prop plumbing through every view is needed.
const state = $state({ open: false });

export const mobilePalette = {
  get isOpen() {
    return state.open;
  },
  open() {
    state.open = true;
  },
  close() {
    state.open = false;
  },
};
