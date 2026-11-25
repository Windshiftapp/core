/**
 * Currency formatting for monetary values.
 *
 * The instance currency is an ISO 4217 code stored in admin general settings
 * and mirrored into the brand store. These helpers render amounts with it and
 * fall back to USD when the value is missing or unrecognized, so a bad setting
 * can never break a page.
 */

import { getAppLocale } from '../stores/appLocale.svelte.js';

export const DEFAULT_CURRENCY = 'USD';

// Used only when Intl.supportedValuesOf is unavailable; the runtime list is
// authoritative everywhere else.
const FALLBACK_CURRENCIES = [
  'AED',
  'ARS',
  'AUD',
  'BRL',
  'CAD',
  'CHF',
  'CLP',
  'CNY',
  'COP',
  'CZK',
  'DKK',
  'EGP',
  'EUR',
  'GBP',
  'HKD',
  'HUF',
  'IDR',
  'ILS',
  'INR',
  'JPY',
  'KES',
  'KRW',
  'MAD',
  'MXN',
  'MYR',
  'NGN',
  'NOK',
  'NZD',
  'PEN',
  'PHP',
  'PLN',
  'RON',
  'RUB',
  'SAR',
  'SEK',
  'SGD',
  'THB',
  'TRY',
  'UAH',
  'USD',
  'VND',
  'ZAR',
];

/** Uppercase a 3-letter code, falling back to the default for anything else. */
function resolveCurrency(currency) {
  return typeof currency === 'string' && /^[A-Za-z]{3}$/.test(currency.trim())
    ? currency.trim().toUpperCase()
    : DEFAULT_CURRENCY;
}

/** Localized display names, or null when Intl.DisplayNames is unavailable. */
function currencyDisplayNames(locale) {
  try {
    return new Intl.DisplayNames([locale], { type: 'currency' });
  } catch {
    return null;
  }
}

/** All selectable ISO 4217 codes, sorted, shown by their code. The localized
 * name is carried as a search keyword so a combobox can match "Swiss" while
 * the option itself stays a compact, readable code. */
export function currencyOptions(locale = getAppLocale()) {
  let codes;
  try {
    codes =
      typeof Intl.supportedValuesOf === 'function'
        ? Intl.supportedValuesOf('currency')
        : FALLBACK_CURRENCIES;
  } catch {
    codes = FALLBACK_CURRENCIES;
  }

  const names = currencyDisplayNames(locale);
  return [...codes]
    .sort((a, b) => a.localeCompare(b))
    .map((code) => {
      const name = names?.of(code);
      return { value: code, label: code, keywords: name || '' };
    });
}

/**
 * Resolve the currency a value should render in: an explicit 3-letter code when
 * present, otherwise the instance currency, otherwise USD. Used where a stored
 * value (e.g. a time project) may inherit the instance-wide setting.
 */
export function resolveDisplayCurrency(valueCurrency, instanceCurrency = DEFAULT_CURRENCY) {
  const candidate = typeof valueCurrency === 'string' ? valueCurrency.trim() : '';
  if (/^[A-Za-z]{3}$/.test(candidate)) return candidate.toUpperCase();
  return resolveCurrency(instanceCurrency);
}

/** Format an amount in the given currency, falling back to USD on a bad code. */
export function formatMoney(amount, currency = DEFAULT_CURRENCY, locale = getAppLocale()) {
  const value = Number(amount);
  const safeValue = Number.isFinite(value) ? value : 0;
  try {
    return new Intl.NumberFormat(locale, {
      style: 'currency',
      currency: resolveCurrency(currency),
    }).format(safeValue);
  } catch {
    return new Intl.NumberFormat(locale, {
      style: 'currency',
      currency: DEFAULT_CURRENCY,
    }).format(safeValue);
  }
}

/** Format an hourly amount, e.g. `CHF 120.00/hr`. */
export function formatHourlyRate(amount, currency = DEFAULT_CURRENCY, locale = getAppLocale()) {
  return `${formatMoney(amount, currency, locale)}/hr`;
}

/** The currency symbol/abbreviation used as an input prefix, e.g. `CHF`. */
export function currencySymbol(currency = DEFAULT_CURRENCY, locale = getAppLocale()) {
  const code = resolveCurrency(currency);
  try {
    const parts = new Intl.NumberFormat(locale, {
      style: 'currency',
      currency: code,
    }).formatToParts(0);
    return parts.find((part) => part.type === 'currency')?.value || code;
  } catch {
    return code;
  }
}
