/** Pure reusable data helpers for the /api-docs renderer. */

/** @typedef {{ $ref?: string, name?: string, in?: string }} OpenAPIParameter */
/** @typedef {{ summary?: string, tags?: string[], parameters?: OpenAPIParameter[], security?: Record<string, string[]>[], 'x-required-scopes'?: string[] }} OpenAPIOperation */
/** @typedef {{ parameters?: OpenAPIParameter[], get?: OpenAPIOperation, post?: OpenAPIOperation, put?: OpenAPIOperation, patch?: OpenAPIOperation, delete?: OpenAPIOperation, head?: OpenAPIOperation, options?: OpenAPIOperation }} OpenAPIPathItem */
/** @typedef {{ paths?: Record<string, OpenAPIPathItem> }} OpenAPISpec */
/** @typedef {{ tag: string, path: string, method: string, operation: OpenAPIOperation, pathParameters: OpenAPIParameter[], id: string }} OperationEntry */
/** @typedef {{ tag: string, operations: OperationEntry[] }} OperationGroup */

const HTTP_METHODS = ['get', 'post', 'put', 'patch', 'delete', 'head', 'options'];

export const API_SPEC_VERSIONS = [
  { value: 'v2', label: 'API v2', url: '/api/v2/openapi.json' },
  { value: 'v1', label: 'API v1 (deprecated)', url: '/rest/api/v1/openapi.json' },
];

/**
 * Fetch the public embedded OpenAPI spec.
 * @param {string} url
 * @returns {Promise<OpenAPISpec>}
 */
export async function loadSpec(url = API_SPEC_VERSIONS[0].url) {
  const res = await fetch(url, { headers: { Accept: 'application/json' } });
  if (!res.ok) {
    throw new Error(`Failed to load OpenAPI spec: ${res.status} ${res.statusText}`);
  }
  return /** @type {Promise<OpenAPISpec>} */ (res.json());
}

/**
 * Resolve a local JSON Pointer or return null.
 * @param {unknown} spec
 * @param {string} ref
 */
export function resolveRef(spec, ref) {
  if (!ref || typeof ref !== 'string' || !ref.startsWith('#/')) return null;
  const segments = ref.slice(2).split('/');
  let cur = spec;
  for (const seg of segments) {
    if (cur == null) return null;
    if (typeof cur !== 'object') return null;
    const object = /** @type {Record<string, unknown>} */ (cur);
    cur = object[decodeURIComponent(seg).replace(/~1/g, '/').replace(/~0/g, '~')];
  }
  return cur ?? null;
}

/**
 * Group operations by first-seen tag, preserving path and method order.
 * @param {OpenAPISpec} spec
 * @returns {OperationGroup[]}
 */
export function groupOperationsByTag(spec) {
  if (!spec?.paths) return [];
  /** @type {string[]} */
  const tagOrder = [];
  /** @type {Map<string, OperationEntry[]>} */
  const byTag = new Map();
  /** @param {string} t */
  const seenTag = (t) => {
    if (!byTag.has(t)) {
      byTag.set(t, []);
      tagOrder.push(t);
    }
    return byTag.get(t) ?? [];
  };

  for (const [path, item] of Object.entries(spec.paths)) {
    for (const method of HTTP_METHODS) {
      const op = /** @type {OpenAPIOperation | undefined} */ (item[method]);
      if (!op) continue;
      const tags = op.tags?.length ? op.tags : ['untagged'];
      const entry = {
        tag: tags[0],
        path,
        method,
        operation: op,
        pathParameters: Array.isArray(item.parameters) ? item.parameters : [],
        id: operationId(method, path),
      };
      for (const tag of tags) {
        seenTag(tag).push(entry);
      }
    }
  }
  return tagOrder.map((tag) => ({ tag, operations: byTag.get(tag) ?? [] }));
}

/**
 * Resolve and merge path-level and operation-level parameters. OpenAPI lets an
 * operation override an inherited parameter with the same name and location.
 * @param {unknown} spec
 * @param {OperationEntry} entry
 * @returns {Record<string, unknown>[]}
 */
export function resolveOperationParameters(spec, entry) {
  const merged = new Map();
  for (const parameter of [
    ...(entry.pathParameters || []),
    ...(entry.operation.parameters || []),
  ]) {
    const resolved = parameter.$ref ? resolveRef(spec, parameter.$ref) : parameter;
    if (!resolved || typeof resolved !== 'object') continue;
    const value = /** @type {Record<string, unknown>} */ (resolved);
    const key = `${String(value.in || '')}:${String(value.name || '')}`;
    merged.set(key, value);
  }
  return [...merged.values()];
}

/**
 * Return the scopes a browser should show for an operation. V2 uses the
 * standards-safe extension; v1 keeps its historical security-array encoding.
 * @param {OpenAPIOperation} operation
 */
export function operationRequiredScopes(operation) {
  if (Array.isArray(operation['x-required-scopes'])) {
    return operation['x-required-scopes'];
  }
  return [
    ...new Set(
      (operation.security || []).flatMap((requirement) =>
        Object.values(requirement).flatMap((scopes) => (Array.isArray(scopes) ? scopes : []))
      )
    ),
  ];
}

/**
 * Stable id for an operation — used as the URL hash + scroll-target.
 * Mirrors the convention common in OpenAPI viewers: lowercase method,
 * path with slashes replaced by dashes, curly braces stripped.
 * @param {string} method
 * @param {string} path
 */
export function operationId(method, path) {
  const slug = path.replace(/[{}]/g, '').replace(/^\//, '').replace(/\//g, '-');
  return `op-${method.toLowerCase()}-${slug || 'root'}`;
}

/**
 * Filter the grouped operations by a free-text query against method, path,
 * tag, operation ID, and summary.
 * Empty groups are dropped.
 * @param {OperationGroup[]} groups
 * @param {string} query
 * @returns {OperationGroup[]}
 */
export function filterGroups(groups, query) {
  const q = (query || '').trim().toLowerCase();
  if (!q) return groups;
  return groups
    .map(({ tag, operations }) => ({
      tag,
      operations: operations.filter((entry) => {
        const haystack =
          `${tag} ${entry.method} ${entry.path} ${entry.id} ${entry.operation.summary || ''}`.toLowerCase();
        return haystack.includes(q);
      }),
    }))
    .filter((g) => g.operations.length > 0);
}
