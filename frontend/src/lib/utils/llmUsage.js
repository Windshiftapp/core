/**
 * Formatting for metered LLM token/cost readings.
 *
 * Shared by every surface that shows a run's spend (the item agent log, the
 * AI chat transcript) so the same run never renders two different prices — a
 * discrepancy there reads as a billing bug even when both numbers are right.
 */

/**
 * Render a USD amount, or null when the cost is unknown.
 *
 * A null cost is a real state, not zero: tokens are metered even when the
 * provider billed nothing we can read and the catalog has no rate for a token
 * class the call used. Callers must show that as "unknown" rather than $0.00.
 *
 * Sub-cent amounts are shown as "<$0.01" because rounding them to $0.00 would
 * read as free, and four decimals below a dollar keeps small but real spends
 * visible.
 */
export function formatCostUSD(usd) {
  if (usd == null || Number.isNaN(Number(usd))) return null;
  const value = Number(usd);
  if (value === 0) return '$0.00';
  if (value > 0 && value < 0.01) return '<$0.01';
  return `$${value.toFixed(value < 1 ? 4 : 2)}`;
}

/**
 * Whether a usage reading has anything worth showing. A reading with no calls
 * means metering never ran for the turn, which is different from a turn that
 * genuinely cost nothing.
 */
export function hasMeteredUsage(usage) {
  return !!usage && Number(usage.calls) > 0;
}
