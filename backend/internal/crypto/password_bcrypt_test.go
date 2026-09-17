package crypto

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestVerifyPasswordBcryptCompatibilityAndLegacyDetection(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword: %v", err)
	}
	encoded := string(hash)

	if !IsLegacyBcrypt(encoded) {
		t.Fatalf("IsLegacyBcrypt(%q) = false, want true", encoded)
	}

	ok, err := VerifyPassword("hunter2", encoded)
	if err != nil {
		t.Fatalf("VerifyPassword (match): %v", err)
	}
	if !ok {
		t.Fatal("VerifyPassword returned false for correct bcrypt password")
	}

	ok, err = VerifyPassword("wrong", encoded)
	if err != nil {
		t.Fatalf("VerifyPassword (mismatch): %v", err)
	}
	if ok {
		t.Fatal("VerifyPassword returned true for wrong bcrypt password")
	}

	native, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if IsLegacyBcrypt(native) {
		t.Fatalf("IsLegacyBcrypt(%q) = true for argon2id hash", native)
	}
	ok, err = VerifyPassword("hunter2", native)
	if err != nil {
		t.Fatalf("VerifyPassword (argon2id): %v", err)
	}
	if !ok {
		t.Fatal("VerifyPassword returned false for correct argon2id password")
	}
}
