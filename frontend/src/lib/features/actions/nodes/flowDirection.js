import { Position } from '@xyflow/svelte';

/**
 * Get input/output handle positions based on flow direction.
 * @param {'horizontal'|'vertical'} direction
 * @returns {{ input: Position, output: Position }}
 */
export function getHandlePositions(direction) {
  if (direction === 'vertical') {
    return { input: Position.Top, output: Position.Bottom };
  }
  return { input: Position.Left, output: Position.Right };
}

/**
 * Get condition node output handle positions and styles for true/false branches.
 * @param {'horizontal'|'vertical'} direction
 * @returns {{ input: Position, trueOutput: Position, falseOutput: Position, trueStyle: string, falseStyle: string }}
 */
export function getConditionOutputPositions(direction) {
  if (direction === 'vertical') {
    return {
      input: Position.Top,
      trueOutput: Position.Bottom,
      falseOutput: Position.Bottom,
      trueStyle: 'left: 35%;',
      falseStyle: 'left: 65%;',
    };
  }
  return {
    input: Position.Left,
    trueOutput: Position.Right,
    falseOutput: Position.Right,
    trueStyle: 'top: 35%;',
    falseStyle: 'top: 65%;',
  };
}
