package auth

import "fmt"

// MaxPasswordBytes is bcrypt's input limit. x/crypto/bcrypt returns an error
// for longer inputs in GenerateFromPassword instead of truncating them, and
// CompareHashAndPassword silently truncates. Every password path must reject
// over-long input before hashing or comparing so a long passphrase fails with
// a validation error and cannot match on its first 72 bytes.
const MaxPasswordBytes = 72

// ErrPasswordTooLong is returned when a password exceeds bcrypt's input limit.
var ErrPasswordTooLong = fmt.Errorf("password must be %d bytes or fewer (bcrypt input limit)", MaxPasswordBytes)

// ValidatePasswordLength rejects passwords longer than bcrypt's input limit.
// len counts bytes, which is the unit bcrypt measures.
func ValidatePasswordLength(password string) error {
	if len(password) > MaxPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}
