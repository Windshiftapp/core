import { describe, expect, it } from 'vitest';
import { isExternalRequest } from './requestOrigin.js';

// A request is external when its content may reach a customer, because staff
// comments on it are emailed out (WI-2365). Email intake is external even when
// the sender has not been verified and no portal customer/request type exists.
describe('isExternalRequest', () => {
  it('treats portal and form submissions as external', () => {
    expect(isExternalRequest({ request_type_id: 3 })).toBe(true);
    expect(isExternalRequest({ creator_portal_customer_id: 7 })).toBe(true);
  });

  it('treats an unverified email contact as external', () => {
    expect(isExternalRequest({ contact_email: 'customer@example.com' })).toBe(true);
  });

  it('keeps an email request external after verification', () => {
    // Verification clears contact_email and sets the portal owner.
    expect(isExternalRequest({ creator_portal_customer_id: 7, contact_email: undefined })).toBe(
      true
    );
  });

  it('keeps agent-created items internal', () => {
    expect(isExternalRequest({})).toBe(false);
    expect(isExternalRequest(null)).toBe(false);
    expect(isExternalRequest(undefined)).toBe(false);
  });
});
