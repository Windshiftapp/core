import { collections } from '../api/collections.js';
/**
 * Load the Board Configuration editor's route-specific data with one API
 * request while reusing the shell-owned workspace/global reference snapshot.
 */
export async function loadBoardConfigurationPageData(referenceStore, workspaceId, collectionId) {
  const referenceRequest = workspaceId
    ? referenceStore.initialize(workspaceId)
    : referenceStore.initializeGlobal();
  const [bootstrap] = await Promise.all([
    collections.getBoardConfigurationBootstrap(collectionId, workspaceId),
    referenceRequest,
  ]);

  const customFieldDefinitions = Array.isArray(referenceStore.customFieldDefinitions)
    ? referenceStore.customFieldDefinitions
    : [];

  return {
    workspace: referenceStore.workspace ?? null,
    collection: bootstrap?.collection ?? null,
    boardConfiguration: removeMissingCustomFieldReferences(
      bootstrap?.board_configuration ?? null,
      customFieldDefinitions
    ),
    statuses: Array.isArray(bootstrap?.statuses)
      ? bootstrap.statuses
      : Array.isArray(referenceStore.statuses)
        ? referenceStore.statuses
        : [],
    customFieldDefinitions,
  };
}

function removeMissingCustomFieldReferences(boardConfiguration, customFieldDefinitions) {
  if (!boardConfiguration) return null;

  const existingIDs = new Set(customFieldDefinitions.map((field) => String(field.id)));
  const keepField = (field) =>
    field?.field_type !== 'custom' || existingIDs.has(customFieldID(field.field_identifier));
  const roadmapConfig = boardConfiguration.roadmap_config
    ? { ...boardConfiguration.roadmap_config }
    : null;

  if (roadmapConfig) {
    for (const key of ['start_field_id', 'end_field_id']) {
      const identifier = roadmapConfig[key];
      if (isCustomFieldIdentifier(identifier) && !existingIDs.has(customFieldID(identifier))) {
        roadmapConfig[key] = '';
      }
    }
  }

  const cleaned = { ...boardConfiguration };
  if (Array.isArray(boardConfiguration.list_columns)) {
    cleaned.list_columns = boardConfiguration.list_columns.filter(keepField);
  }
  if (Array.isArray(boardConfiguration.card_fields)) {
    cleaned.card_fields = boardConfiguration.card_fields.filter(keepField);
  }
  if (roadmapConfig) {
    cleaned.roadmap_config = roadmapConfig;
  }
  return cleaned;
}

function isCustomFieldIdentifier(identifier) {
  return typeof identifier === 'string' && /^(?:custom_field_|cf_)?\d+$/.test(identifier);
}

function customFieldID(identifier) {
  return String(identifier ?? '').replace(/^(?:custom_field_|cf_)/, '');
}
