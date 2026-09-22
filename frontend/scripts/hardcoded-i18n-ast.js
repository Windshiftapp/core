import { parse } from 'svelte/compiler';

const copyNames = new Set([
  'label',
  'title',
  'subtitle',
  'description',
  'placeholder',
  'aria-label',
  'ariaLabel',
  'message',
  'emptyMessage',
  'emptyDescription',
  'errorMessage',
  'emptyText',
  'confirmLabel',
  'cancelLabel',
  'clearLabel',
  'saveLabel',
  'confirmText',
  'cancelText',
  'pageTitle',
  'error',
]);
const toasts = new Set(['errorToast', 'infoToast', 'successToast', 'warningToast']);
const intentionalLiterals = new Set(['Esc']);

/** Inspect copy-bearing AST positions, leaving routes, IDs and user data alone.
 * This is a regression guard, not general data-flow analysis: arbitrary helper
 * return values still need review when a new screen is migrated.
 */
export function findHardcodedCopy(source, { scriptOnly = false } = {}) {
  const prefix = scriptOnly ? '<script>' : '';
  const ast = parse(scriptOnly ? `${prefix}${source}</script>` : source, { modern: true });
  const findings = new Map();

  function report(node, text) {
    if (typeof text !== 'string') return;
    const copy = text.trim();
    if (!/[a-zA-Z]/.test(copy) || intentionalLiterals.has(copy)) return;
    const offset = Math.max(0, node.start - prefix.length);
    findings.set(`${offset}:${copy}`, {
      line: source.slice(0, offset).split('\n').length,
      text: copy,
    });
  }

  function nameOf(node) {
    return node?.name ?? node?.value;
  }

  // Only follow expression branches that can become displayed values. A
  // comparison such as mode === 'edit' is not itself translated copy.
  function displayValue(node) {
    if (!node) return;
    switch (node.type) {
      case 'Text':
        report(node, node.data);
        break;
      case 'Literal':
        report(node, node.value);
        break;
      case 'ExpressionTag':
        displayValue(node.expression);
        break;
      case 'TemplateLiteral':
        for (const part of node.quasis) report(part, part.value.cooked);
        for (const expression of node.expressions) displayValue(expression);
        break;
      case 'ConditionalExpression':
        displayValue(node.consequent);
        displayValue(node.alternate);
        break;
      case 'LogicalExpression':
        displayValue(node.left);
        displayValue(node.right);
        break;
      case 'BinaryExpression':
        if (node.operator === '+') {
          displayValue(node.left);
          displayValue(node.right);
        }
        break;
      case 'AssignmentPattern':
        displayValue(node.right);
        break;
      case 'ArrowFunctionExpression':
        displayValue(node.body);
        break;
      case 'CallExpression':
        if (['$derived', '$state', '$bindable'].includes(nameOf(node.callee))) {
          for (const argument of node.arguments) displayValue(argument);
        }
        break;
      default:
        break;
    }
  }

  function walk(node, visitor) {
    if (!node || typeof node !== 'object') return;
    if (Array.isArray(node)) {
      for (const child of node) walk(child, visitor);
      return;
    }
    if (visitor(node) === false) return;
    for (const [key, child] of Object.entries(node)) {
      if (['loc', 'name_loc', 'comments', 'leadingComments', 'trailingComments'].includes(key))
        continue;
      if (child && typeof child === 'object') walk(child, visitor);
    }
  }

  function scriptCopy(node) {
    if (node.type === 'Property' && !node.computed && copyNames.has(nameOf(node.key))) {
      displayValue(node.value);
    } else if (node.type === 'VariableDeclarator' && copyNames.has(nameOf(node.id))) {
      displayValue(node.init);
    } else if (node.type === 'AssignmentExpression' && copyNames.has(nameOf(node.left))) {
      displayValue(node.right);
    } else if (node.type === 'CallExpression' && toasts.has(nameOf(node.callee))) {
      displayValue(node.arguments[0]);
    } else if (
      node.type === 'FunctionDeclaration' &&
      /^(labelFor|getLabel)/.test(node.id?.name ?? '')
    ) {
      walk(node.body, (child) => {
        if (child.type === 'ReturnStatement') displayValue(child.argument);
      });
    }
  }

  for (const script of [ast.instance, ast.module]) walk(script?.content, scriptCopy);
  walk(ast.fragment, (node) => {
    if (node.type?.endsWith('Directive')) {
      walk(node.expression ?? node.value, scriptCopy);
      return false;
    }
    if (node.type === 'Attribute') {
      if (copyNames.has(node.name)) {
        for (const value of Array.isArray(node.value) ? node.value : [node.value])
          displayValue(value);
      }
      // Inline option objects can contain labels even in a non-copy prop.
      walk(node.value, scriptCopy);
      return false;
    }
    if (node.type === 'ExpressionTag') {
      displayValue(node.expression);
      walk(node.expression, scriptCopy);
      return false;
    }
    if (node.type === 'Text') report(node, node.data);
    scriptCopy(node);
  });
  return [...findings.values()];
}
