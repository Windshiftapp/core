function normalize(value) {
  return String(value ?? '').toLowerCase();
}

function unquoteIdentifier(value) {
  return value.startsWith('`') ? value.slice(1, value.endsWith('`') ? -1 : undefined) : value;
}

function matchesField(field, identifier) {
  const wanted = normalize(unquoteIdentifier(identifier));
  return [field.name, ...(field.aliases || [])].some(
    (candidate) => normalize(candidate) === wanted
  );
}

export function findQlCompletionField(catalog, filterField) {
  if (filterField?.completion) return filterField.completion;

  const identifiers = [filterField?.id];
  if (filterField?.customFieldId != null) {
    identifiers.unshift(`cfid_${filterField.customFieldId}`);
  }
  return (catalog?.fields || []).find((field) =>
    identifiers.some((identifier) => identifier && matchesField(field, identifier))
  );
}

export function completionFieldToFilterField(field) {
  const customFieldId = /^cfid_(\d+)$/i.exec(field?.name || '')?.[1];
  const id = (field?.aliases || []).find((alias) => alias.startsWith('cf_')) || field?.name;
  return {
    id,
    customFieldId: customFieldId ? Number(customFieldId) : undefined,
    name: field?.label || field?.name || '',
    type: field?.field_type || field?.value_type || 'text',
    description: field?.description || '',
    isCustom: Boolean(customFieldId),
    completion: field,
  };
}

function findClauseStart(text) {
  let quote = null;
  let escaped = false;
  let start = 0;

  for (let index = 0; index < text.length; index += 1) {
    const char = text[index];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (quote && char === '\\') {
      escaped = true;
      continue;
    }
    if (quote) {
      if (char === quote) quote = null;
      continue;
    }
    if (char === '"' || char === "'" || char === '`') {
      quote = char;
      continue;
    }

    if (char === '(') {
      const before = text.slice(start, index).trimEnd();
      if (!/\bIN$/i.test(before)) start = index + 1;
      continue;
    }

    const logical = text.slice(index).match(/^(AND|OR)\b/i);
    if (logical) {
      const previous = index === 0 ? '' : text[index - 1];
      if (!/[A-Za-z0-9_.-]/.test(previous)) {
        start = index + logical[0].length;
        index += logical[0].length - 1;
      }
    }
  }

  return start;
}

function fieldToken(segment) {
  const leading = segment.match(/^\s*/)?.[0].length || 0;
  const source = segment.slice(leading);
  if (source.startsWith('`')) {
    const closing = source.indexOf('`', 1);
    const end = closing === -1 ? source.length : closing + 1;
    return { value: source.slice(0, end), start: leading, end: leading + end };
  }
  const match = source.match(/^[A-Za-z_][A-Za-z0-9_.-]*/);
  if (!match) return null;
  return { value: match[0], start: leading, end: leading + match[0].length };
}

function exactOperator(remainder, operators) {
  const ordered = [...operators].sort((a, b) => b.length - a.length);
  const upper = remainder.toUpperCase();
  for (const operator of ordered) {
    const candidate = operator.toUpperCase();
    if (!upper.startsWith(candidate)) continue;
    const next = remainder[candidate.length];
    if (next === undefined || /\s|\(/.test(next)) {
      return { operator, length: candidate.length };
    }
  }
  return null;
}

function valueContext(query, cursor, field, rest, restStart) {
  let offset = restStart;
  let source = rest;
  const leading = source.match(/^\s*/)?.[0].length || 0;
  source = source.slice(leading);
  offset += leading;

  if (source.startsWith('(')) {
    source = source.slice(1);
    offset += 1;
    const comma = source.lastIndexOf(',');
    if (comma >= 0) {
      source = source.slice(comma + 1);
      offset += comma + 1;
    }
    const itemLeading = source.match(/^\s*/)?.[0].length || 0;
    source = source.slice(itemLeading);
    offset += itemLeading;
  }

  if (!source) {
    return { kind: 'value', field, fragment: '', start: offset, end: cursor };
  }

  const quote = source[0];
  if (quote === '"' || quote === "'") {
    let escaped = false;
    let closing = -1;
    for (let index = 1; index < source.length; index += 1) {
      if (escaped) {
        escaped = false;
      } else if (source[index] === '\\') {
        escaped = true;
      } else if (source[index] === quote) {
        closing = index;
        break;
      }
    }
    if (closing >= 0 && source.slice(closing + 1).trim() === '') {
      return { kind: 'logical', fragment: '', start: cursor, end: cursor };
    }
    const end = query[cursor] === quote ? cursor + 1 : cursor;
    return {
      kind: 'value',
      field,
      fragment: source.slice(1, closing >= 0 ? closing : undefined),
      start: offset,
      end,
    };
  }

  const token = source.match(/^[^\s,)]*/)?.[0] || '';
  const after = source.slice(token.length);
  if (token && after.trim() === '' && /\s$/.test(after)) {
    return { kind: 'logical', fragment: '', start: cursor, end: cursor };
  }
  return { kind: 'value', field, fragment: token, start: offset, end: offset + token.length };
}

export function getQlCompletionContext(query, cursor, catalog) {
  const fields = catalog?.fields || [];
  const beforeCursor = String(query ?? '').slice(0, cursor);
  const clauseStart = findClauseStart(beforeCursor);
  const segment = beforeCursor.slice(clauseStart);
  const token = fieldToken(segment);

  if (!token) {
    const leading = segment.match(/^\s*/)?.[0].length || 0;
    return {
      kind: 'field',
      fragment: segment.slice(leading),
      start: clauseStart + leading,
      end: cursor,
    };
  }

  const field = fields.find((candidate) => matchesField(candidate, token.value));
  const remainder = segment.slice(token.end);
  if (!field || remainder.length === 0) {
    return {
      kind: 'field',
      fragment: unquoteIdentifier(token.value),
      start: clauseStart + token.start,
      end: cursor,
    };
  }

  const operatorLeading = remainder.match(/^\s*/)?.[0].length || 0;
  const operatorSource = remainder.slice(operatorLeading);
  const operatorStart = clauseStart + token.end + operatorLeading;
  const operator = exactOperator(operatorSource, field.operators || []);
  if (!operator) {
    return {
      kind: 'operator',
      field,
      fragment: operatorSource,
      start: operatorStart,
      end: cursor,
    };
  }

  const afterOperator = operatorSource.slice(operator.length);
  if (operator.operator === 'IS NULL' || operator.operator === 'IS NOT NULL') {
    if (afterOperator.trim() === '') {
      return afterOperator.length > 0
        ? { kind: 'logical', fragment: '', start: cursor, end: cursor }
        : {
            kind: 'operator',
            field,
            fragment: operatorSource,
            start: operatorStart,
            end: cursor,
          };
    }
  }

  return valueContext(
    String(query ?? ''),
    cursor,
    field,
    afterOperator,
    operatorStart + operator.length
  );
}

function score(candidate, fragment) {
  const normalizedCandidate = normalize(candidate);
  const normalizedFragment = normalize(fragment);
  if (!normalizedFragment) return 1;
  if (normalizedCandidate.startsWith(normalizedFragment)) return 0;
  if (normalizedCandidate.includes(normalizedFragment)) return 1;
  return 2;
}

function matches(candidate, fragment) {
  return !fragment || normalize(candidate).includes(normalize(fragment));
}

export function buildQlSuggestions(context, catalog, values = []) {
  if (!context) return [];

  if (context.kind === 'field') {
    return (catalog?.fields || [])
      .filter((field) =>
        [field.name, field.label, ...(field.aliases || [])].some((candidate) =>
          matches(candidate, context.fragment)
        )
      )
      .map((field) => ({ kind: 'field', value: field.name, label: field.label, field }))
      .sort(
        (left, right) =>
          score(left.value, context.fragment) - score(right.value, context.fragment) ||
          left.value.localeCompare(right.value)
      );
  }

  if (context.kind === 'operator') {
    return (context.field?.operators || [])
      .filter((operator) => matches(operator, context.fragment.trim()))
      .map((operator) => ({ kind: 'operator', value: operator, label: operator }));
  }

  if (context.kind === 'value') {
    return values
      .filter(
        (value) => matches(value.label, context.fragment) || matches(value.value, context.fragment)
      )
      .map((value) => ({ kind: 'value', value: value.value, label: value.label }));
  }

  return (catalog?.logical_operators || []).map((operator) => ({
    kind: 'logical',
    value: operator,
    label: operator,
  }));
}

export function formatQlValue(value, valueType) {
  if (valueType === 'number' || valueType === 'boolean') return String(value);
  return `"${String(value).replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

export function applyQlSuggestion(query, context, suggestion) {
  let insertion = String(suggestion.value);
  if (
    suggestion.kind === 'field' ||
    suggestion.kind === 'operator' ||
    suggestion.kind === 'logical'
  ) {
    insertion += ' ';
  } else if (suggestion.kind === 'value') {
    insertion = formatQlValue(suggestion.value, context.field?.value_type);
  }

  const nextQuery = `${query.slice(0, context.start)}${insertion}${query.slice(context.end)}`;
  return { query: nextQuery, cursor: context.start + insertion.length };
}

export function completionValues(rows, valueHelp) {
  return (rows || [])
    .map((row) => {
      const value = row?.value ?? row?.[valueHelp.value_field];
      const label = row?.label;
      return value == null ? null : { value, label: String(label ?? value) };
    })
    .filter(Boolean);
}
