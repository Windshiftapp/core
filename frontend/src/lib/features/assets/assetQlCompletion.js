import { isBooleanCustomFieldType } from '../../utils/customFieldTypes.js';

const equality = ['=', '!=', 'IN', 'NOT IN', 'IS NULL', 'IS NOT NULL'];
const ordered = ['=', '!=', '<', '<=', '>', '>=', 'IN', 'NOT IN', 'IS NULL', 'IS NOT NULL'];
const text = ['=', '!=', '~', 'IN', 'NOT IN', 'IS NULL', 'IS NOT NULL'];

function namedValues(rows) {
  return rows.flatMap((row) => [
    { value: row.name, label: row.name },
    ...namedValues(row.children || []),
  ]);
}

function optionValues(raw) {
  let options = raw;
  if (typeof options === 'string') {
    try {
      options = JSON.parse(options);
    } catch {
      return [];
    }
  }
  const items = Array.isArray(options) ? options : options?.items;
  if (!Array.isArray(items)) return [];
  return items.map((item) => ({
    value: item?.id ?? item,
    label: String(item?.label ?? item),
  }));
}

export function buildAssetQlCatalog({
  statuses = [],
  assetTypes = [],
  categories = [],
  customFields = [],
} = {}) {
  const field = (name, value_type = 'string', operators = equality, extra = {}) => ({
    name,
    value_type,
    operators,
    ...extra,
  });
  return {
    logical_operators: ['AND', 'OR'],
    fields: [
      ...['title', 'description'].map((name) => field(name, 'string', text)),
      field('asset_tag', 'string', text, { aliases: ['tag', 'assettag'] }),
      field('status', 'string', equality, { values: namedValues(statuses) }),
      field('type', 'string', equality, {
        aliases: ['assettype', 'asset_type'],
        values: namedValues(assetTypes),
      }),
      field('category', 'string', equality, { values: namedValues(categories) }),
      field('category_path', 'string', equality, { aliases: ['categorypath'] }),
      ...['created_at', 'updated_at'].map((name) =>
        field(name, 'date', ordered, {
          aliases: [name.replace('_', ''), name.replace('_at', '')],
        })
      ),
      field('creator', 'number', equality, {
        aliases: ['creatorid', 'creator_id', 'createdby', 'created_by'],
      }),
      field('creator_name', 'string', equality, { aliases: ['creatorname'] }),
      ...['id', 'status_id', 'type_id', 'category_id', 'set_id'].map((name) =>
        field(name, 'number', ordered)
      ),
      field('set'),
      ...customFields.map((custom) => {
        const type = custom.field_type;
        const boolean = isBooleanCustomFieldType(type);
        const valueType = boolean
          ? 'boolean'
          : type === 'number'
            ? 'number'
            : type === 'date'
              ? 'date'
              : 'string';
        const operators = boolean
          ? ['=', '!=', 'IS NULL', 'IS NOT NULL']
          : ['number', 'date'].includes(type)
            ? ordered
            : ['text', 'textarea'].includes(type)
              ? text
              : equality;
        const name = custom.field_name || custom.name;
        return field(`cfid_${custom.custom_field_id ?? custom.id}`, valueType, operators, {
          label: name,
          aliases: [`cf_${name}`, `custom.${name}`],
          values: boolean
            ? [
                { value: true, label: 'true' },
                { value: false, label: 'false' },
              ]
            : optionValues(custom.options),
        });
      }),
    ],
  };
}
