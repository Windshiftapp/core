// Testing store - manages all test-related state
class TestingStore {
  testCases = $state([]);
  testSets = $state([]);
  testRuns = $state([]);
  selectedSet = $state(null);
  selectedRun = $state(null);
  currentView = $state('test-cases');
}

export const testingStore = new TestingStore();

// For backward compatibility with existing code that might use individual named imports,
// you can import testingStore and access properties directly: testingStore.testCases, etc.
