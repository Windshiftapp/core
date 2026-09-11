import { authStore } from '../stores/auth.svelte.js';

// QL is a JQL-like filter language. It supports comparisons, boolean
// expressions, IN clauses, and functions such as currentUser(), childrenOf(),
// and linkedOf().

/**
 * Token types for QL parsing
 */
export const TokenType = {
  IDENTIFIER: 'IDENTIFIER',
  STRING: 'STRING',
  NUMBER: 'NUMBER',
  DATE: 'DATE',
  BOOLEAN: 'BOOLEAN',

  EQUALS: 'EQUALS', // =
  NOT_EQUALS: 'NOT_EQUALS', // !=, <>
  LESS_THAN: 'LESS_THAN', // <
  LESS_EQUAL: 'LESS_EQUAL', // <=
  GREATER_THAN: 'GREATER_THAN', // >
  GREATER_EQUAL: 'GREATER_EQUAL', // >=
  CONTAINS: 'CONTAINS', // ~
  IN: 'IN', // IN
  NOT_IN: 'NOT_IN', // NOT IN
  IS: 'IS', // IS
  NULL: 'NULL', // NULL

  AND: 'AND',
  OR: 'OR',
  NOT: 'NOT',

  LPAREN: 'LPAREN', // (
  RPAREN: 'RPAREN', // )
  COMMA: 'COMMA', // ,

  EOF: 'EOF',
  FUNCTION: 'FUNCTION',
};

/**
 * QL Tokenizer - converts query string into tokens
 */
export class QLTokenizer {
  constructor(input) {
    this.input = input;
    this.position = 0;
    this.current = this.input[this.position];
  }

  error(message) {
    throw new Error(`QL Syntax Error at position ${this.position}: ${message}`);
  }

  advance() {
    this.position++;
    this.current = this.position >= this.input.length ? null : this.input[this.position];
  }

  skipWhitespace() {
    while (this.current && /\s/.test(this.current)) {
      this.advance();
    }
  }

  readString() {
    const quote = this.current; // " or '
    let value = '';
    this.advance();

    while (this.current && this.current !== quote) {
      if (this.current === '\\') {
        this.advance();
        if (this.current) {
          value += this.current;
          this.advance();
        }
      } else {
        value += this.current;
        this.advance();
      }
    }

    if (!this.current) {
      this.error('Unterminated string literal');
    }

    this.advance(); // Skip closing quote
    return value;
  }

  readNumber() {
    let value = '';
    if (this.current === '-') {
      value += this.current;
      this.advance();
    }
    while (this.current && /[\d.]/.test(this.current)) {
      value += this.current;
      this.advance();
    }
    return parseFloat(value);
  }

  readIdentifier() {
    let value = '';
    while (this.current && /[a-zA-Z0-9_.-]/.test(this.current)) {
      value += this.current;
      this.advance();
    }
    return value;
  }

  readDate() {
    let value = '';
    // Read YYYY-MM-DD format
    while (this.current && /[\d-]/.test(this.current)) {
      value += this.current;
      this.advance();
    }
    return value;
  }

  peekAhead(count = 1) {
    const pos = this.position + count;
    return pos >= this.input.length ? null : this.input[pos];
  }

  tokenize() {
    const tokens = [];

    while (this.current) {
      this.skipWhitespace();

      if (!this.current) break;

      // String literals
      if (this.current === '"' || this.current === "'") {
        tokens.push({
          type: TokenType.STRING,
          value: this.readString(),
        });
        continue;
      }

      // Backtick-quoted identifiers (for field names with spaces)
      if (this.current === '`') {
        tokens.push({
          type: TokenType.IDENTIFIER,
          value: this.readString(),
        });
        continue;
      }

      // Numbers and dates (YYYY-MM-DD)
      if (/\d/.test(this.current) || (this.current === '-' && /\d/.test(this.peekAhead()))) {
        // Check if it's a date pattern (YYYY-MM-DD)
        if (
          this.current &&
          /\d/.test(this.current) &&
          this.peekAhead(4) === '-' &&
          this.peekAhead(7) === '-'
        ) {
          tokens.push({
            type: TokenType.DATE,
            value: this.readDate(),
          });
        } else {
          tokens.push({
            type: TokenType.NUMBER,
            value: this.readNumber(),
          });
        }
        continue;
      }

      // Identifiers and keywords
      if (/[a-zA-Z_]/.test(this.current)) {
        const identifier = this.readIdentifier();

        // Check for keywords
        const upperIdent = identifier.toUpperCase();
        switch (upperIdent) {
          case 'AND':
            tokens.push({ type: TokenType.AND, value: 'AND' });
            break;
          case 'OR':
            tokens.push({ type: TokenType.OR, value: 'OR' });
            break;
          case 'NOT':
            // Look ahead to see if it's "NOT IN".
            this.skipWhitespace();
            if (
              this.current &&
              this.input.slice(this.position, this.position + 2).toUpperCase() === 'IN' &&
              !/[a-zA-Z0-9_.-]/.test(this.input[this.position + 2] || '')
            ) {
              this.advance(); // I
              this.advance(); // N
              tokens.push({ type: TokenType.NOT_IN, value: 'NOT IN' });
            } else {
              tokens.push({ type: TokenType.NOT, value: 'NOT' });
            }
            break;
          case 'IN':
            tokens.push({ type: TokenType.IN, value: 'IN' });
            break;
          case 'IS':
            tokens.push({ type: TokenType.IS, value: 'IS' });
            break;
          case 'NULL':
            tokens.push({ type: TokenType.NULL, value: null });
            break;
          case 'TRUE':
          case 'FALSE':
            tokens.push({ type: TokenType.BOOLEAN, value: upperIdent.toLowerCase() });
            break;
          default:
            // Check if it's a function (followed by parentheses)
            this.skipWhitespace();
            if (this.current === '(') {
              tokens.push({ type: TokenType.FUNCTION, value: identifier });
            } else {
              tokens.push({ type: TokenType.IDENTIFIER, value: identifier });
            }
        }
        continue;
      }

      // Two-character operators
      if (this.current === '!' && this.peekAhead() === '=') {
        this.advance();
        this.advance();
        tokens.push({ type: TokenType.NOT_EQUALS, value: '!=' });
        continue;
      }

      if (this.current === '<' && this.peekAhead() === '=') {
        this.advance();
        this.advance();
        tokens.push({ type: TokenType.LESS_EQUAL, value: '<=' });
        continue;
      }

      if (this.current === '>' && this.peekAhead() === '=') {
        this.advance();
        this.advance();
        tokens.push({ type: TokenType.GREATER_EQUAL, value: '>=' });
        continue;
      }

      if (this.current === '<' && this.peekAhead() === '>') {
        this.advance();
        this.advance();
        tokens.push({ type: TokenType.NOT_EQUALS, value: '<>' });
        continue;
      }

      // Single-character tokens
      switch (this.current) {
        case '=':
          tokens.push({ type: TokenType.EQUALS, value: '=' });
          this.advance();
          break;
        case '<':
          tokens.push({ type: TokenType.LESS_THAN, value: '<' });
          this.advance();
          break;
        case '>':
          tokens.push({ type: TokenType.GREATER_THAN, value: '>' });
          this.advance();
          break;
        case '~':
          tokens.push({ type: TokenType.CONTAINS, value: '~' });
          this.advance();
          break;
        case '(':
          tokens.push({ type: TokenType.LPAREN, value: '(' });
          this.advance();
          break;
        case ')':
          tokens.push({ type: TokenType.RPAREN, value: ')' });
          this.advance();
          break;
        case ',':
          tokens.push({ type: TokenType.COMMA, value: ',' });
          this.advance();
          break;
        default:
          this.error(`Unexpected character: ${this.current}`);
      }
    }

    tokens.push({ type: TokenType.EOF, value: null });
    return tokens;
  }
}

/**
 * AST Node types for QL
 */
export const NodeType = {
  BINARY_OP: 'BINARY_OP',
  COMPARISON: 'COMPARISON',
  IN_EXPRESSION: 'IN_EXPRESSION',
  NULL_CHECK: 'NULL_CHECK',
  IDENTIFIER: 'IDENTIFIER',
  LITERAL: 'LITERAL',
  FUNCTION_CALL: 'FUNCTION_CALL',
  LIST: 'LIST',
};

/**
 * QL Parser - converts tokens into Abstract Syntax Tree (AST)
 */
export class QLParser {
  constructor(tokens) {
    this.tokens = tokens;
    this.current = 0;
  }

  error(message) {
    const token = this.tokens[this.current];
    throw new Error(`QL Parse Error at token ${token?.value || 'EOF'}: ${message}`);
  }

  peek() {
    return this.tokens[this.current];
  }

  advance() {
    if (this.current < this.tokens.length - 1) {
      this.current++;
    }
    return this.tokens[this.current - 1];
  }

  match(...types) {
    const token = this.peek();
    return types.includes(token.type);
  }

  consume(type, message) {
    if (this.peek().type === type) {
      return this.advance();
    }
    this.error(message);
  }

  parse() {
    const ast = this.expression();
    if (this.peek().type !== TokenType.EOF) {
      this.error('Unexpected tokens after expression');
    }
    return ast;
  }

  expression() {
    return this.orExpression();
  }

  orExpression() {
    let left = this.andExpression();

    while (this.match(TokenType.OR)) {
      const operator = this.advance();
      const right = this.andExpression();
      left = {
        type: NodeType.BINARY_OP,
        operator: operator.value,
        left,
        right,
      };
    }

    return left;
  }

  andExpression() {
    let left = this.notExpression();

    while (this.match(TokenType.AND)) {
      const operator = this.advance();
      const right = this.notExpression();
      left = {
        type: NodeType.BINARY_OP,
        operator: operator.value,
        left,
        right,
      };
    }

    return left;
  }

  notExpression() {
    if (this.match(TokenType.NOT)) {
      const operator = this.advance();
      const operand = this.comparison();
      return {
        type: NodeType.BINARY_OP,
        operator: operator.value,
        left: null,
        right: operand,
      };
    }

    return this.comparison();
  }

  comparison() {
    const left = this.primary();

    if (this.match(TokenType.IS)) {
      this.advance();
      let operator = 'IS NULL';
      if (this.match(TokenType.NOT)) {
        this.advance();
        operator = 'IS NOT NULL';
      }
      this.consume(TokenType.NULL, 'Expected NULL after IS');
      return {
        type: NodeType.NULL_CHECK,
        operator,
        field: left,
      };
    }

    if (
      this.match(
        TokenType.EQUALS,
        TokenType.NOT_EQUALS,
        TokenType.LESS_THAN,
        TokenType.LESS_EQUAL,
        TokenType.GREATER_THAN,
        TokenType.GREATER_EQUAL,
        TokenType.CONTAINS
      )
    ) {
      const operator = this.advance();
      const right = this.primary();
      return {
        type: NodeType.COMPARISON,
        operator: operator.value,
        left,
        right,
      };
    }

    if (this.match(TokenType.IN, TokenType.NOT_IN)) {
      const operator = this.advance();
      this.consume(TokenType.LPAREN, 'Expected ( after IN');
      const values = this.valueList();
      this.consume(TokenType.RPAREN, 'Expected ) after IN values');
      return {
        type: NodeType.IN_EXPRESSION,
        operator: operator.value,
        field: left,
        values,
      };
    }

    return left;
  }

  primary() {
    if (this.match(TokenType.IDENTIFIER)) {
      const token = this.advance();
      return {
        type: NodeType.IDENTIFIER,
        value: token.value,
      };
    }

    if (
      this.match(
        TokenType.STRING,
        TokenType.NUMBER,
        TokenType.DATE,
        TokenType.BOOLEAN,
        TokenType.NULL
      )
    ) {
      const token = this.advance();
      return {
        type: NodeType.LITERAL,
        dataType: token.type,
        value: token.value,
      };
    }

    if (this.match(TokenType.FUNCTION)) {
      const token = this.advance();
      this.consume(TokenType.LPAREN, 'Expected ( after function name');

      const args = [];
      if (!this.match(TokenType.RPAREN)) {
        args.push(this.expression());
        while (this.match(TokenType.COMMA)) {
          this.advance();
          args.push(this.expression());
        }
      }

      this.consume(TokenType.RPAREN, 'Expected ) after function arguments');
      return {
        type: NodeType.FUNCTION_CALL,
        name: token.value,
        arguments: args,
      };
    }

    if (this.match(TokenType.LPAREN)) {
      this.advance();
      const expr = this.expression();
      this.consume(TokenType.RPAREN, 'Expected )');
      return expr;
    }

    this.error('Expected identifier, literal, function, or (');
  }

  valueList() {
    const values = [];
    this._parseValues(values);
    return {
      type: NodeType.LIST,
      values,
    };
  }

  _parseValues(values) {
    if (
      this.match(
        TokenType.STRING,
        TokenType.NUMBER,
        TokenType.DATE,
        TokenType.BOOLEAN,
        TokenType.IDENTIFIER
      )
    ) {
      const token = this.advance();
      values.push({
        type: NodeType.LITERAL,
        dataType: token.type,
        value: token.value,
      });

      while (this.match(TokenType.COMMA)) {
        this.advance();
        if (
          this.match(
            TokenType.STRING,
            TokenType.NUMBER,
            TokenType.DATE,
            TokenType.BOOLEAN,
            TokenType.IDENTIFIER
          )
        ) {
          const token = this.advance();
          values.push({
            type: NodeType.LITERAL,
            dataType: token.type,
            value: token.value,
          });
        } else {
          this.error('Expected value after comma');
        }
      }
    }
  }
}

// Shared evaluation helpers used by both QLEvaluator and AssetQLEvaluator.

const BARE_VALUE_FIELDS = new Set([
  'workspace',
  'workspacekey',
  'status',
  'priority',
  'type',
  'assettype',
  'asset_type',
  'category',
]);

function compareValuesShared(left, right, operation) {
  if (Array.isArray(left)) {
    return left.some((candidate) => compareValuesShared(candidate, right, operation));
  }
  if (left == null && right == null) return operation === 'equals';
  if (left == null || right == null) return operation !== 'equals';

  const leftStr = String(left).toLowerCase();
  const rightStr = String(right).toLowerCase();

  switch (operation) {
    case 'equals':
      return leftStr === rightStr;
    case 'contains':
      return leftStr.includes(rightStr);
    case 'less':
      return left < right;
    case 'lessEqual':
      return left <= right;
    case 'greater':
      return left > right;
    case 'greaterEqual':
      return left >= right;
    default:
      return false;
  }
}

function evaluateComparisonShared(evaluator, ast, item) {
  const left = evaluator.evaluate(ast.left, item);
  const leftField = ast.left?.type === NodeType.IDENTIFIER ? ast.left.value.toLowerCase() : '';
  const right =
    ast.right?.type === NodeType.IDENTIFIER && BARE_VALUE_FIELDS.has(leftField)
      ? ast.right.value
      : evaluator.evaluate(ast.right, item);

  switch (ast.operator) {
    case '=':
      return evaluator.compareValues(left, right, 'equals');
    case '!=':
    case '<>':
      return !evaluator.compareValues(left, right, 'equals');
    case '<':
      return evaluator.compareValues(left, right, 'less');
    case '<=':
      return evaluator.compareValues(left, right, 'lessEqual');
    case '>':
      return evaluator.compareValues(left, right, 'greater');
    case '>=':
      return evaluator.compareValues(left, right, 'greaterEqual');
    case '~':
      return evaluator.compareValues(left, right, 'contains');
    default:
      throw new Error(`Unknown comparison operator: ${ast.operator}`);
  }
}

function evaluateInExpressionShared(evaluator, ast, item) {
  const fieldValue = evaluator.evaluate(ast.field, item);
  const values = ast.values.values.map((v) => evaluator.evaluate(v, item));
  const isIn = values.some((value) => evaluator.compareValues(fieldValue, value, 'equals'));
  return ast.operator === 'IN' ? isIn : !isIn;
}

function evaluateFunctionShared(ast) {
  switch (ast.name.toLowerCase()) {
    case 'currentuser':
      return authStore.currentUser?.id?.toString() || null;
    case 'now':
      return new Date().toISOString();
    case 'startofday': {
      const start = new Date();
      start.setHours(0, 0, 0, 0);
      return start.toISOString();
    }
    case 'endofday': {
      const end = new Date();
      end.setHours(23, 59, 59, 999);
      return end.toISOString();
    }
    default:
      throw new Error(`Unknown function: ${ast.name}`);
  }
}

class QLEvaluatorEngine {
  constructor(errorLabel) {
    this.errorLabel = errorLabel;
  }

  getFieldValue(_fieldName, _record) {
    throw new Error('QL evaluator must implement field resolution');
  }

  evaluate(ast, record) {
    switch (ast.type) {
      case NodeType.BINARY_OP:
        return this.evaluateBinaryOp(ast, record);
      case NodeType.COMPARISON:
        return evaluateComparisonShared(this, ast, record);
      case NodeType.IN_EXPRESSION:
        return evaluateInExpressionShared(this, ast, record);
      case NodeType.NULL_CHECK: {
        const value = this.evaluate(ast.field, record);
        const isNull = value === null || value === undefined;
        return ast.operator === 'IS NOT NULL' ? !isNull : isNull;
      }
      case NodeType.IDENTIFIER:
        return this.getFieldValue(ast.value, record);
      case NodeType.LITERAL:
        return ast.value;
      case NodeType.FUNCTION_CALL:
        return evaluateFunctionShared(ast);
      default:
        throw new Error(`Unknown AST node type: ${ast.type}`);
    }
  }

  evaluateBinaryOp(ast, record) {
    switch (ast.operator) {
      case 'AND':
        return this.evaluate(ast.left, record) && this.evaluate(ast.right, record);
      case 'OR':
        return this.evaluate(ast.left, record) || this.evaluate(ast.right, record);
      case 'NOT':
        return !this.evaluate(ast.right, record);
      default:
        throw new Error(`Unknown binary operator: ${ast.operator}`);
    }
  }

  compareValues(left, right, operation) {
    return compareValuesShared(left, right, operation);
  }

  filter(records, queryString) {
    if (!queryString?.trim()) return records;
    try {
      const tokens = new QLTokenizer(queryString).tokenize();
      const ast = new QLParser(tokens).parse();
      return records.filter((record) => this.evaluate(ast, record));
    } catch (error) {
      console.error(this.errorLabel, error.message);
      throw error;
    }
  }
}

/**
 * QL Evaluator - executes AST against work items
 */
export class QLEvaluator extends QLEvaluatorEngine {
  constructor(workspaces = []) {
    super('QL Error:');
    this.workspaces = workspaces;
    this.workspaceMap = new Map();

    // Build workspace lookup map
    workspaces.forEach((ws) => {
      this.workspaceMap.set(ws.id, ws);
      this.workspaceMap.set(ws.name.toLowerCase(), ws);
      this.workspaceMap.set(ws.key.toLowerCase(), ws);
    });
  }

  getFieldValue(fieldName, item) {
    switch (fieldName.toLowerCase()) {
      case 'workspace': {
        const workspace = this.workspaceMap.get(item.workspace_id);
        return workspace ? [workspace.name, workspace.key] : [];
      }
      case 'workspaceid':
      case 'workspace_id':
        return item.workspace_id;
      case 'workspacekey': {
        const ws = this.workspaceMap.get(item.workspace_id);
        return ws ? ws.key : 'UNKNOWN';
      }
      case 'status':
        return item.status;
      case 'priority':
        return item.priority;
      case 'title':
        return item.title || '';
      case 'description':
        return item.description || '';
      case 'created':
        return item.created_at;
      case 'updated':
        return item.updated_at;
      case 'assignee':
        return item.assignee_id;
      case 'creator':
        return item.creator_id;
      case 'id':
        return item.id;
      default:
        throw new Error(`Unknown field: ${fieldName}`);
    }
  }
}

/**
 * Utility functions for building QL queries from UI components
 */
const BUILDER_FILTER_FIELDS = [
  {
    state: 'workspaces',
    outputField: 'workspaceKey',
    outputType: 'text',
    inputs: [
      ['workspace', 'workspace'],
      ['workspacekey', 'workspace'],
      ['workspaceid', 'workspaceId'],
      ['workspace_id', 'workspaceId'],
    ],
  },
  {
    state: 'statuses',
    outputField: 'status_id',
    outputType: 'number',
    inputs: [
      ['statusid', 'id'],
      ['status_id', 'id'],
      ['status', 'catalog'],
    ],
    catalog: 'statuses',
  },
  {
    state: 'priorities',
    outputField: 'priority_id',
    outputType: 'number',
    inputs: [
      ['priorityid', 'id'],
      ['priority_id', 'id'],
      ['priority', 'catalog'],
    ],
    catalog: 'priorities',
  },
];

const BUILDER_INPUT_FIELDS = new Map(
  BUILDER_FILTER_FIELDS.flatMap((definition) =>
    definition.inputs.map(([alias, mode]) => [alias, { ...definition, mode }])
  )
);

export class QLBuilder {
  static buildQuery(filters) {
    const conditions = [];

    for (const definition of BUILDER_FILTER_FIELDS) {
      const values = (filters[definition.state] || []).filter(
        (value) => value !== null && value !== undefined
      );
      if (values.length === 0) continue;
      const formatted = values.map((value) => QLBuilder.formatValue(value, definition.outputType));
      if (formatted.length === 1) {
        conditions.push(`${definition.outputField} = ${formatted[0]}`);
      } else {
        conditions.push(`${definition.outputField} IN (${formatted.join(', ')})`);
      }
    }

    if (filters.search?.trim()) {
      const searchTerm = filters.search.trim();
      const formattedSearch = QLBuilder.formatValue(searchTerm, 'text');
      conditions.push(
        `(title ~ ${formattedSearch} OR description ~ ${formattedSearch} OR key = ${formattedSearch})`
      );
    }

    if (filters.dynamicFields && filters.dynamicFields.length > 0) {
      filters.dynamicFields.forEach((filter) => {
        if (
          filter.field &&
          (filter.operator === 'IS NULL' ||
            filter.operator === 'IS NOT NULL' ||
            filter.value ||
            (filter.values && filter.values.length > 0))
        ) {
          const condition = QLBuilder.buildFieldCondition(filter);
          if (condition) {
            conditions.push(condition);
          }
        }
      });
    }

    return conditions.join(' AND ');
  }

  /**
   * Build a QL condition from a dynamic field filter
   */
  static buildFieldCondition(filter) {
    const { field, operator, value, values } = filter;

    if (!field?.id) return null;

    // Wrap field ID in backticks to handle names with spaces (e.g. cf_Time Estimate)
    const fieldId = `\`${field.id}\``;

    if (operator === 'IS NULL' || operator === 'IS NOT NULL') {
      return `${fieldId} ${operator}`;
    }

    if ((operator === 'IN' || operator === 'NOT IN') && values && values.length > 0) {
      const valuesList = values.map((v) => QLBuilder.formatValue(v, field.type)).join(', ');
      return `${fieldId} ${operator} (${valuesList})`;
    }

    if ((operator === 'IN' || operator === 'NOT IN') && value) {
      const valueList = value
        .split(',')
        .map((v) => v.trim())
        .filter((v) => v);
      if (valueList.length > 0) {
        const formattedValues = valueList
          .map((v) => QLBuilder.formatValue(v, field.type))
          .join(', ');
        return `${fieldId} ${operator} (${formattedValues})`;
      }
      return null; // Empty value list
    }

    if (!value && value !== 0 && value !== false) return null;

    const formattedValue = QLBuilder.formatValue(value, field.type);

    if (operator === '~') {
      return `${fieldId} ~ ${formattedValue}`;
    }

    return `${fieldId} ${operator} ${formattedValue}`;
  }

  /**
   * Format a value for QL based on its type
   */
  static formatValue(value, fieldType) {
    if (value === null || value === undefined) return 'null';

    switch (fieldType) {
      case 'text':
      case 'textarea':
      case 'select':
      case 'enum':
        // String values need quotes
        return `"${String(value).replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;

      case 'number':
      case 'boolean':
        // Numbers and booleans don't need quotes
        return String(value);

      case 'date':
        // Emit UTC YYYY-MM-DD so DATE values round-trip and TIMESTAMP filters
        // remain timezone-stable. Use UTC accessors for Date inputs.
        if (value instanceof Date) {
          const year = value.getUTCFullYear();
          const month = String(value.getUTCMonth() + 1).padStart(2, '0');
          const day = String(value.getUTCDate()).padStart(2, '0');
          return `"${year}-${month}-${day}"`;
        }
        return `"${String(value).replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;

      case 'user':
      case 'reference':
        // Numeric IDs stay unquoted; string values (e.g. group names) get quoted.
        // Number.isNaN('abc') is false, so the previous check let strings escape
        // unquoted — switch to a real numeric typeof check.
        if (typeof value === 'number' && Number.isFinite(value)) {
          return String(value);
        }
        if (typeof value === 'string' && /^-?\d+(?:\.\d+)?$/.test(value) && value !== '') {
          return value;
        }
        return `"${String(value).replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;

      case 'identifier':
        // Identifiers like work item keys (WS-123) are strings
        return `"${String(value).replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;

      default:
        // Default: treat as string
        return `"${String(value).replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
    }
  }

  // Projects parsed QL into the fields supported by the visual builder.
  // Unsupported boolean structures and fields set `dropped` so callers can
  // preserve the original query in raw mode.
  static tryParseToBuilder(queryString, options = {}) {
    if (!queryString?.trim()) return null;

    const customFieldsCatalog = options.customFields || [];
    const customFieldsById = new Map(
      customFieldsCatalog.flatMap((field) =>
        [field?.id, field?.completion?.name, ...(field?.completion?.aliases || [])]
          .filter(Boolean)
          .map((identifier) => [String(identifier).toLowerCase(), field])
      )
    );

    const result = {
      workspaces: [],
      statuses: [],
      priorities: [],
      search: '',
      dynamicFields: [],
      dropped: false,
    };

    let ast;
    try {
      ast = new QLParser(new QLTokenizer(queryString).tokenize()).parse();
    } catch {
      return null;
    }

    QLBuilder._projectBuilderNode(ast, result, options, customFieldsById);

    const hasAny =
      result.workspaces.length > 0 ||
      result.statuses.length > 0 ||
      result.priorities.length > 0 ||
      result.search !== '' ||
      result.dynamicFields.length > 0;
    return hasAny ? result : null;
  }

  static _projectBuilderNode(node, result, options, customFieldsById) {
    if (node?.type === NodeType.BINARY_OP && node.operator === 'AND') {
      QLBuilder._projectBuilderNode(node.left, result, options, customFieldsById);
      QLBuilder._projectBuilderNode(node.right, result, options, customFieldsById);
      return;
    }

    if (node?.type === NodeType.BINARY_OP && node.operator === 'OR') {
      const search = QLBuilder._searchTermFromOr(node);
      if (search !== null && (result.search === '' || result.search === search)) {
        result.search = search;
      } else {
        result.dropped = true;
        QLBuilder._projectBuilderNode(node.left, result, options, customFieldsById);
        QLBuilder._projectBuilderNode(node.right, result, options, customFieldsById);
      }
      return;
    }

    if (node?.type === NodeType.BINARY_OP && node.operator === 'NOT') {
      result.dropped = true;
      QLBuilder._projectBuilderNode(node.right, result, options, customFieldsById);
      return;
    }

    if (QLBuilder._projectBuiltInField(node, result, options)) return;
    if (QLBuilder._projectCustomField(node, result, customFieldsById)) return;
    result.dropped = true;
  }

  static _projectBuiltInField(node, result, options) {
    const isComparison = node?.type === NodeType.COMPARISON;
    const isIn = node?.type === NodeType.IN_EXPRESSION;
    if (!isComparison && !isIn) return false;

    const fieldNode = isComparison ? node.left : node.field;
    if (fieldNode?.type !== NodeType.IDENTIFIER) return false;
    const fieldName = String(fieldNode.value).toLowerCase();

    if (fieldName === 'title' && isComparison && node.operator === '~') {
      const value = QLBuilder._builderNodeValue(node.right);
      if (value === null) return false;
      if (result.search !== '' && result.search !== value) result.dropped = true;
      result.search = value;
      return true;
    }

    const definition = BUILDER_INPUT_FIELDS.get(fieldName);
    if (!definition || (node.operator !== '=' && node.operator !== 'IN')) return false;
    const valueNodes = isComparison ? [node.right] : node.values?.values || [];
    const values = valueNodes.map(QLBuilder._builderNodeValue).filter((value) => value !== null);
    if (values.length !== valueNodes.length || values.length === 0) return false;

    if (definition.mode === 'workspace') {
      result[definition.state].push(...values);
      return true;
    }

    if (definition.mode === 'workspaceId') {
      for (const value of values) {
        const match = (options.workspaces || []).find(
          (workspace) => workspace.id === Number(value)
        );
        if (match?.key) result[definition.state].push(match.key);
        else result.dropped = true;
      }
      return true;
    }

    if (definition.mode === 'id') {
      for (const value of values) {
        const id = Number(value);
        if (Number.isInteger(id)) result[definition.state].push(id);
        else result.dropped = true;
      }
      return true;
    }

    const catalog = options[definition.catalog] || [];
    for (const value of values) {
      const match = catalog.find(
        (entry) => String(entry?.name ?? '').toLowerCase() === value.toLowerCase()
      );
      if (match?.id != null) result[definition.state].push(match.id);
      else result.dropped = true;
    }
    return true;
  }

  static _projectCustomField(node, result, customFieldsById) {
    const isComparison = node?.type === NodeType.COMPARISON;
    const isIn = node?.type === NodeType.IN_EXPRESSION;
    const isNullCheck = node?.type === NodeType.NULL_CHECK;
    if (!isComparison && !isIn && !isNullCheck) return false;

    const fieldNode = isComparison ? node.left : node.field;
    if (fieldNode?.type !== NodeType.IDENTIFIER) return false;
    const fieldId = String(fieldNode.value).trim();
    const lowerField = fieldId.toLowerCase();
    if (
      !lowerField.startsWith('cf_') &&
      !lowerField.startsWith('cfid_') &&
      !lowerField.startsWith('custom.')
    )
      return false;

    if (isIn) {
      const valueNodes = node.values?.values || [];
      const values = valueNodes.map(QLBuilder._builderNodeValue).filter((value) => value !== null);
      if (values.length !== valueNodes.length || values.length === 0) return false;
      const inferred = QLBuilder._inferFieldFromNodes(valueNodes);
      result.dynamicFields.push({
        field: QLBuilder._resolveField(fieldId, customFieldsById, inferred),
        operator: node.operator,
        value: '',
        values,
      });
      return true;
    }

    if (isNullCheck) {
      result.dynamicFields.push({
        field: QLBuilder._resolveField(fieldId, customFieldsById, 'text'),
        operator: node.operator,
        value: '',
        values: [],
      });
      return true;
    }

    if (
      node.right?.dataType === TokenType.NULL &&
      (node.operator === '=' || node.operator === '!=')
    ) {
      result.dynamicFields.push({
        field: QLBuilder._resolveField(fieldId, customFieldsById, 'text'),
        operator: node.operator === '=' ? 'IS NULL' : 'IS NOT NULL',
        value: '',
        values: [],
      });
      return true;
    }

    const value = QLBuilder._builderNodeValue(node.right);
    if (value === null) return false;
    result.dynamicFields.push({
      field: QLBuilder._resolveField(
        fieldId,
        customFieldsById,
        QLBuilder._fieldTypeFromNode(node.right)
      ),
      operator: node.operator,
      value,
      values: [],
    });
    return true;
  }

  static _searchTermFromOr(node) {
    const clauses = [];
    const flatten = (candidate) => {
      if (candidate?.type === NodeType.BINARY_OP && candidate.operator === 'OR') {
        flatten(candidate.left);
        flatten(candidate.right);
      } else {
        clauses.push(candidate);
      }
    };
    flatten(node);
    if (clauses.length !== 3) return null;

    const expected = new Map([
      ['title', '~'],
      ['description', '~'],
      ['key', '='],
    ]);
    let value = null;
    for (const clause of clauses) {
      if (clause?.type !== NodeType.COMPARISON || clause.left?.type !== NodeType.IDENTIFIER) {
        return null;
      }
      const field = String(clause.left.value).toLowerCase();
      if (expected.get(field) !== clause.operator) return null;
      const clauseValue = QLBuilder._builderNodeValue(clause.right);
      if (clauseValue === null || (value !== null && value !== clauseValue)) return null;
      value = clauseValue;
      expected.delete(field);
    }
    return expected.size === 0 ? value : null;
  }

  static _builderNodeValue(node) {
    if (node?.type !== NodeType.LITERAL && node?.type !== NodeType.IDENTIFIER) return null;
    return String(node.value);
  }

  static _fieldTypeFromNode(node) {
    switch (node?.dataType) {
      case TokenType.NUMBER:
        return 'number';
      case TokenType.BOOLEAN:
        return 'boolean';
      default:
        return 'text';
    }
  }

  static _inferFieldFromNodes(nodes) {
    if (nodes.every((node) => node.dataType === TokenType.NUMBER)) return 'number';
    if (nodes.every((node) => node.dataType === TokenType.BOOLEAN)) return 'boolean';
    return 'enum';
  }

  static _resolveField(fieldId, catalog, fallbackType) {
    const known = catalog.get(String(fieldId).toLowerCase());
    if (known) {
      return {
        ...known,
        type: known.type || fallbackType || 'text',
      };
    }
    return {
      id: fieldId,
      type: fallbackType || 'text',
      name: fieldId,
    };
  }
}

/**
 * Asset QL Evaluator - executes QL AST against assets in memory
 * Similar to QLEvaluator but with asset-specific field mappings
 */
export class AssetQLEvaluator extends QLEvaluatorEngine {
  constructor(assetSets = []) {
    super('Asset QL Error:');
    this.assetSets = assetSets;
    this.setMap = new Map();

    // Build set lookup map
    assetSets.forEach((set) => {
      this.setMap.set(set.id, set);
      this.setMap.set(set.name.toLowerCase(), set);
    });
  }

  getFieldValue(fieldName, asset) {
    const lowerFieldName = fieldName.toLowerCase();

    // Handle custom fields
    if (lowerFieldName.startsWith('cf_') && asset.custom_field_values) {
      const cfName = fieldName.substring(3);
      return asset.custom_field_values[cfName];
    }
    if (lowerFieldName.startsWith('custom.') && asset.custom_field_values) {
      const cfName = fieldName.substring(7);
      return asset.custom_field_values[cfName];
    }

    switch (lowerFieldName) {
      // Set fields (equivalent to workspace for items)
      case 'set':
      case 'setname':
      case 'set_name':
        return asset.set_name || '';
      case 'setid':
      case 'set_id':
        return asset.set_id;

      // Status fields
      case 'status':
        return asset.status_name || '';
      case 'statusid':
      case 'status_id':
        return asset.status_id;

      // Type fields
      case 'type':
      case 'assettype':
      case 'asset_type':
        return asset.asset_type_name || '';
      case 'typeid':
      case 'type_id':
      case 'assettypeid':
      case 'asset_type_id':
        return asset.asset_type_id;

      // Category fields
      case 'category':
        return asset.category_name || '';
      case 'categoryid':
      case 'category_id':
        return asset.category_id;
      case 'categorypath':
      case 'category_path':
        return asset.category_path || '';

      // Basic text fields
      case 'title':
        return asset.title || '';
      case 'description':
        return asset.description || '';
      case 'tag':
      case 'assettag':
      case 'asset_tag':
        return asset.asset_tag || '';

      // Date fields
      case 'created':
      case 'created_at':
      case 'createdat':
        return asset.created_at;
      case 'updated':
      case 'updated_at':
      case 'updatedat':
        return asset.updated_at;

      // Creator fields
      case 'creator':
      case 'creatorid':
      case 'creator_id':
      case 'createdby':
      case 'created_by':
        return asset.created_by;
      case 'creatorname':
      case 'creator_name':
        return asset.creator_name || '';

      // ID
      case 'id':
        return asset.id;

      default:
        throw new Error(`Unknown asset field: ${fieldName}`);
    }
  }
}

/**
 * Utility for building QL queries from UI components for assets
 */
export class AssetQLBuilder {
  static buildQuery(filters) {
    const conditions = [];

    // Set filter
    if (filters.sets && filters.sets.length > 0) {
      if (filters.sets.length === 1) {
        conditions.push(`set = "${filters.sets[0]}"`);
      } else {
        const setNames = filters.sets.map((s) => `"${s}"`).join(', ');
        conditions.push(`set IN (${setNames})`);
      }
    }

    // Status filter
    if (filters.statuses && filters.statuses.length > 0) {
      if (filters.statuses.length === 1) {
        conditions.push(`status = "${filters.statuses[0]}"`);
      } else {
        const statusNames = filters.statuses.map((s) => `"${s}"`).join(', ');
        conditions.push(`status IN (${statusNames})`);
      }
    }

    // Type filter
    if (filters.types && filters.types.length > 0) {
      if (filters.types.length === 1) {
        conditions.push(`type = "${filters.types[0]}"`);
      } else {
        const typeNames = filters.types.map((t) => `"${t}"`).join(', ');
        conditions.push(`type IN (${typeNames})`);
      }
    }

    // Category filter
    if (filters.categories && filters.categories.length > 0) {
      if (filters.categories.length === 1) {
        conditions.push(`category = "${filters.categories[0]}"`);
      } else {
        const categoryNames = filters.categories.map((c) => `"${c}"`).join(', ');
        conditions.push(`category IN (${categoryNames})`);
      }
    }

    // Search/text filter
    if (filters.search?.trim()) {
      const searchTerm = filters.search.trim();
      conditions.push(
        `(title ~ "${searchTerm}" OR description ~ "${searchTerm}" OR tag ~ "${searchTerm}")`
      );
    }

    return conditions.join(' AND ');
  }
}
