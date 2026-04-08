/**
 * Creates reusable wizard step navigation methods.
 * @param {() => { currentStep: number, steps: Array<{ id: string, label: string, completed: boolean }> }} getWizardState
 *   Getter function returning the current wizard state (handles reactivity after resets).
 * @returns {{ nextStep: () => void, prevStep: () => void, goToStep: (index: number) => void }}
 */
export function createWizardNavigation(getWizardState) {
  return {
    nextStep() {
      const ws = getWizardState();
      if (ws.currentStep < ws.steps.length - 1) {
        ws.currentStep++;
      }
    },
    prevStep() {
      const ws = getWizardState();
      if (ws.currentStep > 0) {
        ws.currentStep--;
      }
    },
    goToStep(stepIndex) {
      const ws = getWizardState();
      if (stepIndex >= 0 && stepIndex < ws.steps.length) {
        ws.currentStep = stepIndex;
      }
    },
  };
}
