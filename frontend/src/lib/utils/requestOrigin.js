// A request is external when it was submitted by a portal customer (portal,
// public form, or helpdesk email intake) or through a request type (portal or
// form submissions by an internal user). Items created directly by agents have
// none of these and stay internal.
//
// The request-type check alone misses helpdesk email: the email processor sets
// the channel and an unverified contact but no request type when the intake has
// no connected portal. `creator_portal_customer_id` covers a verified email
// contact, and `contact_email` covers an email contact that has not proved
// control of the address yet. Both must count as external independently of
// portal ownership: staff replies are emailed to the contact, so the internal
// note control must not disappear (WI-2365).
export function isExternalRequest(item) {
  return Boolean(item?.request_type_id || item?.creator_portal_customer_id || item?.contact_email);
}
