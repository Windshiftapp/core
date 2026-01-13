import { authStore } from '../stores/auth.svelte.js';

// QL (Query Language) - A JQL-like query language for work item filtering
//
// Syntax Examples:
// - workspace = "My Project"
// - status IN (open, in_progress)
// - priority = high AND status != completed
// - workspace IN ("Proj A", "Proj B") AND (status = open OR priority = critical)
// - title ~ "bug" AND created >= "2024-01-01"
// - assignee = currentUser() AND status != completed
// - childrenOf("priority = high") - Find all descendants of high priority items
// - linkedOf("blocks", "status = open") - Find items blocked by open items
//
// Supported Fields:
// - workspace, status, priority, title, description, created, updated, assignee, creator
//
// Supported Operators:
// - =, !=, <, <=, >, >=, ~, IN, NOT IN
// - AND, OR, NOT
// - Parentheses for grouping
//
// Supported Functions:
// - currentUser(), now(), startOfDay(), endOfDay()
// - childrenOf("ql_query") - Find all descendants (recursive) of items matching the query
// - linkedOf("link_label", "ql_query") - Find items linked via the specified link type
//   Examples: linkedOf("blocks", "priority = high") - items blocked by high priority items
//             linkedOf("is blocked by", "status = open") - items blocking open items

/**
 * Token types for QL parsing
 */
export const TokenType = {
  // Literals
  IDENTIFIER: 'IDENTIFIER',
  STRING: 'STRING',
  NUMBER: 'NUMBER',
  DATE: 'DATE',

  // Operators
  EQUALS: 'EQUALS',           // =
  NOT_EQUALS: 'NOT_EQUALS',   // !=, <>
  LESS_THAN: 'LESS_THAN',     // <
  LESS_EQUAL: 'LESS_EQUAL',   // <=
  GREATER_THAN: 'GREATER_THAN', // >
  GREATER_EQUAL: 'GREATER_EQUAL', // >=
  CONTAINS: 'CONTAINS',       // ~
  IN: 'IN',                   // IN
  NOT_IN: 'NOT_IN',          // NOT IN

  // Logical operators
  AND: 'AND',
  OR: 'OR',
  NOT: 'NOT',

  // Punctuation
  LPAREN: 'LPAREN',           // (
  RPAREN: 'RPAREN',           // )
  COMMA: 'COMMA',             // ,

  // Special
  EOF: 'EOF',
  FUNCTION: 'FUNCTION'
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
    while (this.current && /[\d.]/.test(this.current)) {
      value += this.current;
      this.advance();
    }
    return parseFloat(value);
  }

  readIdentifier() {
    let value = '';
    while (this.current && /[a-zA-Z0-9_]/.test(this.current)) {
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
          value: this.readString()
        });
        continue;
      }

      // Numbers and dates (YYYY-MM-DD)
      if (/\d/.test(this.current)) {
        const start = this.position;
        let value = '';

        // Check if it's a date pattern (YYYY-MM-DD)
        if (this.current && /\d/.test(this.current) &&
            this.peekAhead(4) === '-' &&
            this.peekAhead(7) === '-') {
          tokens.push({
            type: TokenType.DATE,
            value: this.readDate()
          });
        } else {
          tokens.push({
            type: TokenType.NUMBER,
            value: this.readNumber()
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
            // Look ahead to see if it's "NOT IN"
            this.skipWhitespace();
            if (this.current && this.input.slice(this.position, this.position + 2).toUpperCase() === 'IN') {
              this.advance(); // N
              this.advance(); // O
              this.advance(); // T
              this.skipWhitespace();
              if (this.current && this.input.slice(this.position, this.position + 2).toUpperCase() === 'IN') {
                this.advance(); // I
                this.advance(); // N
                tokens.push({ type: TokenType.NOT_IN, value: 'NOT IN' });
              } else {
                this.error('Expected IN after NOT');
              }
            } else {
              tokens.push({ type: TokenType.NOT, value: 'NOT' });
            }
            break;
          case 'IN':
            tokens.push({ type: TokenType.IN, value: 'IN' });
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
  IDENTIFIER: 'IDENTIFIER',
  LITERAL: 'LITERAL',
  FUNCTION_CALL: 'FUNCTION_CALL',
  LIST: 'LIST'
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

  // Grammar:
  // expression → or_expression
  // or_expression → and_expression ( "OR" and_expression )*
  // and_expression → not_expression ( "AND" not_expression )*
  // not_expression → "NOT" comparison | comparison
  // comparison → primary ( operator primary )*
  // primary → identifier | literal | function_call | "(" expression ")"

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
        right
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
        right
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
        right: operand
      };
    }

    return this.comparison();
  }

  comparison() {
    const left = this.primary();

    if (this.match(TokenType.EQUALS, TokenType.NOT_EQUALS, TokenType.LESS_THAN,
                    TokenType.LESS_EQUAL, TokenType.GREATER_THAN, TokenType.GREATER_EQUAL,
                    TokenType.CONTAINS)) {
      const operator = this.advance();
      const right = this.primary();
      return {
        type: NodeType.COMPARISON,
        operator: operator.value,
        left,
        right
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
        values
      };
    }

    return left;
  }

  primary() {
    if (this.match(TokenType.IDENTIFIER)) {
      const token = this.advance();
      return {
        type: NodeType.IDENTIFIER,
        value: token.value
      };
    }

    if (this.match(TokenType.STRING, TokenType.NUMBER, TokenType.DATE)) {
      const token = this.advance();
      return {
        type: NodeType.LITERAL,
        dataType: token.type,
        value: token.value
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
        arguments: args
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

    if (this.match(TokenType.STRING, TokenType.NUMBER, TokenType.DATE, TokenType.IDENTIFIER)) {
      const token = this.advance();
      values.push({
        type: NodeType.LITERAL,
        dataType: token.type,
        value: token.value
      });

      while (this.match(TokenType.COMMA)) {
        this.advance();
        if (this.match(TokenType.STRING, TokenType.NUMBER, TokenType.DATE, TokenType.IDENTIFIER)) {
          const token = this.advance();
          values.push({
            type: NodeType.LITERAL,
            dataType: token.type,
            value: token.value
          });
        } else {
          this.error('Expected value after comma');
        }
      }
    }

    return {
      type: NodeType.LIST,
      values
    };
  }
}

/**
 * QL Evaluator - executes AST against work items
 */
export class QLEvaluator {
  constructor(workspaces = []) {
    this.workspaces = workspaces;
    this.workspaceMap = new Map();

    // Build workspace lookup map
    workspaces.forEach(ws => {
      this.workspaceMap.set(ws.id, ws);
      this.workspaceMap.set(ws.name.toLowerCase(), ws);
      this.workspaceMap.set(ws.key.toLowerCase(), ws);
    });
  }

  evaluate(ast, item) {
    switch (ast.type) {
      case NodeType.BINARY_OP:
        return this.evaluateBinaryOp(ast, item);
      case NodeType.COMPARISON:
        return this.evaluateComparison(ast, item);
      case NodeType.IN_EXPRESSION:
        return this.evaluateInExpression(ast, item);
      case NodeType.IDENTIFIER:
        return this.getFieldValue(ast.value, item);
      case NodeType.LITERAL:
        return ast.value;
      case NodeType.FUNCTION_CALL:
        return this.evaluateFunction(ast, item);
      default:
        throw new Error(`Unknown AST node type: ${ast.type}`);
    }
  }

  evaluateBinaryOp(ast, item) {
    switch (ast.operator) {
      case 'AND':
        return this.evaluate(ast.left, item) && this.evaluate(ast.right, item);
      case 'OR':
        return this.evaluate(ast.left, item) || this.evaluate(ast.right, item);
      case 'NOT':
        return !this.evaluate(ast.right, item);
      default:
        throw new Error(`Unknown binary operator: ${ast.operator}`);
    }
  }

  evaluateComparison(ast, item) {
    const left = this.evaluate(ast.left, item);
    const right = this.evaluate(ast.right, item);

    switch (ast.operator) {
      case '=':
        return this.compareValues(left, right, 'equals');
      case '!=':
      case '<>':
        return !this.compareValues(left, right, 'equals');
      case '<':
        return this.compareValues(left, right, 'less');
      case '<=':
        return this.compareValues(left, right, 'lessEqual');
      case '>':
        return this.compareValues(left, right, 'greater');
      case '>=':
        return this.compareValues(left, right, 'greaterEqual');
      case '~':
        return this.compareValues(left, right, 'contains');
      default:
        throw new Error(`Unknown comparison operator: ${ast.operator}`);
    }
  }

  evaluateInExpression(ast, item) {
    const fieldValue = this.evaluate(ast.field, item);
    const values = ast.values.values.map(v => this.evaluate(v, item));

    const isIn = values.some(value => this.compareValues(fieldValue, value, 'equals'));
    return ast.operator === 'IN' ? isIn : !isIn;
  }

  evaluateFunction(ast, item) {
    switch (ast.name.toLowerCase()) {
      case 'currentuser':
        return authStore.currentUser?.id?.toString() || null;
      case 'now':
        return new Date().toISOString();
      case 'startofday':
        const start = new Date();
        start.setHours(0, 0, 0, 0);
        return start.toISOString();
      case 'endofday':
        const end = new Date();
        end.setHours(23, 59, 59, 999);
        return end.toISOString();
      default:
        throw new Error(`Unknown function: ${ast.name}`);
    }
  }

  getFieldValue(fieldName, item) {
    switch (fieldName.toLowerCase()) {
      case 'workspace':
        const workspace = this.workspaceMap.get(item.workspace_id);
        return workspace ? workspace.name : 'Unknown';
      case 'workspaceid':
        return item.workspace_id;
      case 'workspacekey':
        const ws = this.workspaceMap.get(item.workspace_id);
        return ws ? ws.key : 'UNKNOWN';
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

  compareValues(left, right, operation) {
    // Handle null/undefined values
    if (left == null && right == null) return operation === 'equals';
    if (left == null || right == null) return operation !== 'equals';

    // Convert to comparable types
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

  filter(items, queryString) {
    if (!queryString || !queryString.trim()) {
      return items;
    }

    try {
      const tokenizer = new QLTokenizer(queryString);
      const tokens = tokenizer.tokenize();

      const parser = new QLParser(tokens);
      const ast = parser.parse();

      return items.filter(item => this.evaluate(ast, item));
    } catch (error) {
      console.error('QL Error:', error.message);
      throw error;
    }
  }
}

/**
 * Utility functions for building QL queries from UI components
 */
export class QLBuilder {
  static buildQuery(filters) {
    const conditions = [];

    // Workspace filter (use workspace name field, not workspaceId which expects numeric IDs)
    if (filters.workspaces && filters.workspaces.length > 0) {
      if (filters.workspaces.length === 1) {
        conditions.push(`workspace = "${filters.workspaces[0]}"`);
      } else {
        const workspaceNames = filters.workspaces.map(w => `"${w}"`).join(', ');
        conditions.push(`workspace IN (${workspaceNames})`);
      }
    }

    // Status filter (use numeric IDs)
    if (filters.statuses && filters.statuses.length > 0) {
      const statusIds = filters.statuses.filter(id => id !== null && id !== undefined);
      if (statusIds.length === 1) {
        conditions.push(`status_id = ${statusIds[0]}`);
      } else if (statusIds.length > 1) {
        conditions.push(`status_id IN (${statusIds.join(', ')})`);
      }
    }

    // Priority filter (use numeric IDs)
    if (filters.priorities && filters.priorities.length > 0) {
      const priorityIds = filters.priorities.filter(id => id !== null && id !== undefined);
      if (priorityIds.length === 1) {
        conditions.push(`priority_id = ${priorityIds[0]}`);
      } else if (priorityIds.length > 1) {
        conditions.push(`priority_id IN (${priorityIds.join(', ')})`);
      }
    }

    // Search filter
    if (filters.search && filters.search.trim()) {
      const searchTerm = filters.search.trim();
      conditions.push(`(title ~ "${searchTerm}" OR description ~ "${searchTerm}")`);
    }

    // Dynamic field filters
    if (filters.dynamicFields && filters.dynamicFields.length > 0) {
      filters.dynamicFields.forEach(filter => {
        if (filter.field && (filter.value || (filter.values && filter.values.length > 0))) {
          const condition = this.buildFieldCondition(filter);
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

    if (!field || !field.id) return null;

    // Get the field identifier for QL
    const fieldId = field.id;

    // Handle IN/NOT IN operators with multiple values
    if ((operator === 'IN' || operator === 'NOT IN') && values && values.length > 0) {
      const valuesList = values.map(v => this.formatValue(v, field.type)).join(', ');
      return `${fieldId} ${operator} (${valuesList})`;
    }

    // Handle IN/NOT IN operators with single text value (comma-separated)
    if ((operator === 'IN' || operator === 'NOT IN') && value) {
      // Parse comma-separated values from the text input
      const valueList = value.split(',').map(v => v.trim()).filter(v => v);
      if (valueList.length > 0) {
        const formattedValues = valueList.map(v => this.formatValue(v, field.type)).join(', ');
        return `${fieldId} ${operator} (${formattedValues})`;
      }
      return null; // Empty value list
    }

    // Handle single value operators
    if (!value && value !== 0 && value !== false) return null;

    const formattedValue = this.formatValue(value, field.type);

    // Special handling for text contains operator
    if (operator === '~') {
      return `${fieldId} ~ ${formattedValue}`;
    }

    // Standard comparison operators
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
        return `"${String(value).replace(/"/g, '\\"')}"`;

      case 'number':
      case 'boolean':
        // Numbers and booleans don't need quotes
        return String(value);

      case 'date':
        // Dates in YYYY-MM-DD format
        if (value instanceof Date) {
          return `"${value.toISOString().split('T')[0]}"`;
        }
        return `"${value}"`;

      case 'user':
      case 'reference':
        // User and reference fields are usually numeric IDs
        return String(value);

      case 'identifier':
        // Identifiers like work item keys (WS-123) are strings
        return `"${String(value).replace(/"/g, '\\"')}"`;

      default:
        // Default: treat as string
        return `"${String(value).replace(/"/g, '\\"')}"`;
    }
  }

  static parseFiltersFromQuery(queryString, workspaces = [], priorities = [], statusesList = []) {
    // Parse a QL query back into UI filter objects
    // This is a simple implementation that handles common filter patterns

    const result = {
      workspaces: [],
      statuses: [],
      priorities: [],
      search: '',
      dynamicFields: []
    };

    if (!queryString || !queryString.trim()) {
      // Normalize legacy priority names to IDs if provided
      if (result.priorities.length > 0) {
        result.priorities = result.priorities.map(priorityValue => {
          if (typeof priorityValue === 'number') {
            return priorityValue;
          }
          const normalizedValue = String(priorityValue).toLowerCase();
          const matchingPriority = priorities.find(priority =>
            priority.name?.toLowerCase() === normalizedValue
          );
          return matchingPriority ? matchingPriority.id : null;
        }).filter(id => id !== null && id !== undefined);
      }

      // Normalize legacy priority names to IDs if provided
      if (result.priorities.length > 0) {
        result.priorities = result.priorities.map(priorityValue => {
          if (typeof priorityValue === 'number') {
            return priorityValue;
          }
          const normalizedValue = String(priorityValue).toLowerCase();
          const matchingPriority = priorities.find(priority =>
            priority.name?.toLowerCase() === normalizedValue
          );
          return matchingPriority ? matchingPriority.id : null;
        }).filter(id => id !== null && id !== undefined);
      }

      if (result.statuses.length > 0) {
        result.statuses = result.statuses.map(statusValue => {
          if (typeof statusValue === 'number') {
            return statusValue;
          }
          const normalizedValue = String(statusValue).toLowerCase();
          const matchingStatus = statusesList.find(status =>
            (status.name || status.key || '').toLowerCase() === normalizedValue
          );
          return matchingStatus ? matchingStatus.id : null;
        }).filter(id => id !== null && id !== undefined);
      }

      return result;
    }

    try {
      // Parse workspace filters (workspace = "X" or workspace IN ("X", "Y"))
      const workspaceMatch = queryString.match(/workspace\s*=\s*"([^"]+)"/);
      const workspaceInMatch = queryString.match(/workspace\s+IN\s*\(([^)]+)\)/);

      if (workspaceInMatch) {
        // Extract workspace names from IN clause
        const workspaceList = workspaceInMatch[1];
        result.workspaces = workspaceList
          .split(',')
          .map(w => w.trim().replace(/["']/g, ''))
          .filter(Boolean);
      } else if (workspaceMatch) {
        // Single workspace
        result.workspaces = [workspaceMatch[1]];
      }

      // Parse status filters by ID first
      const statusIdMatch = queryString.match(/status[_]?id\s*=\s*(\d+)/i);
      const statusIdInMatch = queryString.match(/status[_]?id\s+IN\s*\(([^)]+)\)/i);

      if (statusIdInMatch) {
        const statusList = statusIdInMatch[1];
        result.statuses = statusList
          .split(',')
          .map(s => parseInt(s.trim(), 10))
          .filter(id => !isNaN(id));
      } else if (statusIdMatch) {
        const parsedId = parseInt(statusIdMatch[1], 10);
        result.statuses = isNaN(parsedId) ? [] : [parsedId];
      } else {
        // Legacy support for status names
        const statusMatch = queryString.match(/status\s*=\s*["']?(\w+)["']?/i);
        const statusInMatch = queryString.match(/status\s+IN\s*\(([^)]+)\)/i);

        if (statusInMatch) {
          const statusList = statusInMatch[1];
          result.statuses = statusList
            .split(',')
            .map(s => s.trim().replace(/["']/g, ''))
            .filter(Boolean);
        } else if (statusMatch) {
          result.statuses = [statusMatch[1]];
        }
      }

      // Parse priority filters by ID first
      const priorityIdMatch = queryString.match(/priority[_]?id\s*=\s*(\d+)/i);
      const priorityIdInMatch = queryString.match(/priority[_]?id\s+IN\s*\(([^)]+)\)/i);

      if (priorityIdInMatch) {
        const priorityList = priorityIdInMatch[1];
        result.priorities = priorityList
          .split(',')
          .map(p => parseInt(p.trim(), 10))
          .filter(id => !isNaN(id));
      } else if (priorityIdMatch) {
        const parsedId = parseInt(priorityIdMatch[1], 10);
        result.priorities = isNaN(parsedId) ? [] : [parsedId];
      } else {
        // Legacy support for priority names
        const priorityMatch = queryString.match(/priority\s*=\s*["']?(\w+)["']?/i);
        const priorityInMatch = queryString.match(/priority\s+IN\s*\(([^)]+)\)/i);

        if (priorityInMatch) {
          const priorityList = priorityInMatch[1];
          result.priorities = priorityList
            .split(',')
            .map(p => p.trim().replace(/["']/g, ''))
            .filter(Boolean);
        } else if (priorityMatch) {
          result.priorities = [priorityMatch[1]];
        }
      }

      // Parse search/title filters
      const titleMatch = queryString.match(/title\s*~\s*["']([^"']+)["']/);
      if (titleMatch) {
        result.search = titleMatch[1];
      }

    } catch (error) {
      console.error('Error parsing QL filters:', error);
    }

    if (result.priorities.length > 0) {
      result.priorities = result.priorities.map(priorityValue => {
        if (typeof priorityValue === 'number') {
          return priorityValue;
        }
        const normalizedValue = String(priorityValue).toLowerCase();
        const matchingPriority = priorities.find(priority =>
          priority.name?.toLowerCase() === normalizedValue
        );
        return matchingPriority ? matchingPriority.id : null;
      }).filter(id => id !== null && id !== undefined);
    }

    if (result.statuses.length > 0) {
      result.statuses = result.statuses.map(statusValue => {
        if (typeof statusValue === 'number') {
          return statusValue;
        }
        const normalizedValue = String(statusValue).toLowerCase();
        const matchingStatus = statusesList.find(status =>
          (status.name || status.key || '').toLowerCase() === normalizedValue
        );
        return matchingStatus ? matchingStatus.id : null;
      }).filter(id => id !== null && id !== undefined);
    }

    return result;
  }
}

// Example usage:
// const ql = new QLEvaluator(workspaces);
// const filtered = ql.filter(items, 'workspace IN ("Project A", "Project B") AND priority = "high"');

/**
 * Asset QL Evaluator - executes QL AST against assets in memory
 * Similar to QLEvaluator but with asset-specific field mappings
 */
export class AssetQLEvaluator {
  constructor(assetSets = []) {
    this.assetSets = assetSets;
    this.setMap = new Map();

    // Build set lookup map
    assetSets.forEach(set => {
      this.setMap.set(set.id, set);
      this.setMap.set(set.name.toLowerCase(), set);
    });
  }

  evaluate(ast, asset) {
    switch (ast.type) {
      case NodeType.BINARY_OP:
        return this.evaluateBinaryOp(ast, asset);
      case NodeType.COMPARISON:
        return this.evaluateComparison(ast, asset);
      case NodeType.IN_EXPRESSION:
        return this.evaluateInExpression(ast, asset);
      case NodeType.IDENTIFIER:
        return this.getFieldValue(ast.value, asset);
      case NodeType.LITERAL:
        return ast.value;
      case NodeType.FUNCTION_CALL:
        return this.evaluateFunction(ast, asset);
      default:
        throw new Error(`Unknown AST node type: ${ast.type}`);
    }
  }

  evaluateBinaryOp(ast, asset) {
    switch (ast.operator) {
      case 'AND':
        return this.evaluate(ast.left, asset) && this.evaluate(ast.right, asset);
      case 'OR':
        return this.evaluate(ast.left, asset) || this.evaluate(ast.right, asset);
      case 'NOT':
        return !this.evaluate(ast.right, asset);
      default:
        throw new Error(`Unknown binary operator: ${ast.operator}`);
    }
  }

  evaluateComparison(ast, asset) {
    const left = this.evaluate(ast.left, asset);
    const right = this.evaluate(ast.right, asset);

    switch (ast.operator) {
      case '=':
        return this.compareValues(left, right, 'equals');
      case '!=':
      case '<>':
        return !this.compareValues(left, right, 'equals');
      case '<':
        return this.compareValues(left, right, 'less');
      case '<=':
        return this.compareValues(left, right, 'lessEqual');
      case '>':
        return this.compareValues(left, right, 'greater');
      case '>=':
        return this.compareValues(left, right, 'greaterEqual');
      case '~':
        return this.compareValues(left, right, 'contains');
      default:
        throw new Error(`Unknown comparison operator: ${ast.operator}`);
    }
  }

  evaluateInExpression(ast, asset) {
    const fieldValue = this.evaluate(ast.field, asset);
    const values = ast.values.values.map(v => this.evaluate(v, asset));

    const isIn = values.some(value => this.compareValues(fieldValue, value, 'equals'));
    return ast.operator === 'IN' ? isIn : !isIn;
  }

  evaluateFunction(ast, asset) {
    switch (ast.name.toLowerCase()) {
      case 'currentuser':
        return authStore.currentUser?.id?.toString() || null;
      case 'now':
        return new Date().toISOString();
      case 'startofday':
        const start = new Date();
        start.setHours(0, 0, 0, 0);
        return start.toISOString();
      case 'endofday':
        const end = new Date();
        end.setHours(23, 59, 59, 999);
        return end.toISOString();
      default:
        throw new Error(`Unknown function: ${ast.name}`);
    }
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

  compareValues(left, right, operation) {
    // Handle null/undefined values
    if (left == null && right == null) return operation === 'equals';
    if (left == null || right == null) return operation !== 'equals';

    // Convert to comparable types
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

  filter(assets, queryString) {
    if (!queryString || !queryString.trim()) {
      return assets;
    }

    try {
      const tokenizer = new QLTokenizer(queryString);
      const tokens = tokenizer.tokenize();

      const parser = new QLParser(tokens);
      const ast = parser.parse();

      return assets.filter(asset => this.evaluate(ast, asset));
    } catch (error) {
      console.error('Asset QL Error:', error.message);
      throw error;
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
        const setNames = filters.sets.map(s => `"${s}"`).join(', ');
        conditions.push(`set IN (${setNames})`);
      }
    }

    // Status filter
    if (filters.statuses && filters.statuses.length > 0) {
      if (filters.statuses.length === 1) {
        conditions.push(`status = "${filters.statuses[0]}"`);
      } else {
        const statusNames = filters.statuses.map(s => `"${s}"`).join(', ');
        conditions.push(`status IN (${statusNames})`);
      }
    }

    // Type filter
    if (filters.types && filters.types.length > 0) {
      if (filters.types.length === 1) {
        conditions.push(`type = "${filters.types[0]}"`);
      } else {
        const typeNames = filters.types.map(t => `"${t}"`).join(', ');
        conditions.push(`type IN (${typeNames})`);
      }
    }

    // Category filter
    if (filters.categories && filters.categories.length > 0) {
      if (filters.categories.length === 1) {
        conditions.push(`category = "${filters.categories[0]}"`);
      } else {
        const categoryNames = filters.categories.map(c => `"${c}"`).join(', ');
        conditions.push(`category IN (${categoryNames})`);
      }
    }

    // Search/text filter
    if (filters.search && filters.search.trim()) {
      const searchTerm = filters.search.trim();
      conditions.push(`(title ~ "${searchTerm}" OR description ~ "${searchTerm}" OR tag ~ "${searchTerm}")`);
    }

    return conditions.join(' AND ');
  }
}

// Example usage for assets:
// const assetQl = new AssetQLEvaluator(assetSets);
// const filteredAssets = assetQl.filter(assets, 'type = "Laptop" AND status = "Active"');
