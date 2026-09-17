package crypto

import (
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// verifyBcrypt checks a plaintext against a bcrypt hash ($2a$/$2b$, plus the
// PHP-style $2y$ alias). Used as a fallback in VerifyPassword for imported
// 9router credentials.
func verifyBcrypt(plaintext, encodedHash string) (bool, error) {
	// $2y$ is computation-identical to $2a$ (PHP ambiguity workaround); Go's
	// bcrypt rejects the prefix, so rewrite it before comparing.
	if strings.HasPrefix(encodedHash, "$2y$") {
		encodedHash = "$2a$" + encodedHash[len("$2y$"):]
	}
	err := bcrypt.CompareHashAndPassword([]byte(encodedHash), []byte(plaintext))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// IsLegacyBcrypt reports whether an encoded password hash is a bcrypt hash
// ($2a$/$2b$, or the $2y$ alias) rather than a native argon2id verifier.
// Callers use it to lazily upgrade an imported bcrypt password to argon2id
// after a successful login.
func IsLegacyBcrypt(encodedHash string) bool {
	return strings.HasPrefix(encodedHash, "$2a$") ||
		strings.HasPrefix(encodedHash, "$2b$") ||
		strings.HasPrefix(encodedHash, "$2y$")
}
