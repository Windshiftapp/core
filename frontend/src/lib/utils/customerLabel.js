// Shared handling for the built-in "no customer" organisation. Projects and
// worklogs created without a customer are attributed to this system row, so
// time views label it specially while customer-admin surfaces hide it.

export const NO_CUSTOMER_BUILTIN_KEY = 'no_customer';

/** True when the organisation is the built-in no-customer placeholder. */
export function isNoCustomer(customer) {
  return customer?.builtin_key === NO_CUSTOMER_BUILTIN_KEY;
}

/** Localized display name for a customer reference (name + builtin_key). */
export function customerDisplayName(name, builtinKey, t) {
  if (builtinKey === NO_CUSTOMER_BUILTIN_KEY) return t('time.noCustomer');
  return name || '';
}

/** Localized display name for a customer organisation object. */
export function customerOrganisationName(customer, t) {
  return customerDisplayName(customer?.name, customer?.builtin_key, t);
}

/** Customers excluding the built-in placeholder. */
export function realCustomers(customers) {
  return (customers || []).filter((customer) => !isNoCustomer(customer));
}
