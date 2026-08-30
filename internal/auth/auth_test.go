package auth

import (
	"errors"
	"testing"
	"time"
	"github.com/google/uuid"
)

// --- Password Hashing Tests ---

func TestPassword_HashAndCheck(t *testing.T) {
	password := "supersecret123"

	// Test hashing
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("Expected no error hashing password, got: %v", err)
	}

	// Test matching password
	match, err := CheckPasswordHash(password, hash)
	if err != nil {
		t.Fatalf("Expected no error checking valid password, got: %v", err)
	}
	if !match {
		t.Error("Expected password to match its generated hash, but it failed")
	}

	// Test incorrect password
	match, err = CheckPasswordHash("wrongpassword", hash)
	if err != nil {
		t.Fatalf("Expected no error checking wrong password, got: %v", err)
	}
	if match {
		t.Error("Expected wrong password to fail matching verification, but it matched")
	}
}

// --- JWT Tests ---

func TestJWT_SuccessWorkflow(t *testing.T) {
	secret := "valid-secret-key-phrase"
	userID := uuid.New()
	duration := 15 * time.Minute

	token, err := MakeJWT(userID, secret, duration)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	validatedID, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("Failed to validate JWT: %v", err)
	}

	if validatedID != userID {
		t.Errorf("Extracted User ID mismatch. Expected %s, got %s", userID, validatedID)
	}
}

func TestJWT_RejectsExpiredToken(t *testing.T) {
	secret := "valid-secret-key-phrase"
	userID := uuid.New()
	
	// Create an already expired token using a negative duration balance
	duration := -5 * time.Minute

	token, err := MakeJWT(userID, secret, duration)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	_, err = ValidateJWT(token, secret)
	if err == nil {
		t.Fatal("Expected validation to fail for an expired token, but error was nil")
	}

	if !errors.Is(err, ErrExpiredToken) {
		t.Errorf("Expected error ErrExpiredToken, got: %v", err)
	}
}

func TestJWT_RejectsWrongSecret(t *testing.T) {
	correctSecret := "the-right-key-123"
	wrongSecret := "malicious-attacker-key"
	userID := uuid.New()
	duration := 10 * time.Minute

	token, err := MakeJWT(userID, correctSecret, duration)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	_, err = ValidateJWT(token, wrongSecret)
	if err == nil {
		t.Fatal("Expected validation to fail due to dynamic secret mismatch, but error was nil")
	}

	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Expected error ErrInvalidToken, got: %v", err)
	}
}