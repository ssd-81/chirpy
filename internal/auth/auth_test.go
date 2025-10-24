package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword(t *testing.T) {
	name := "sample"
	msg, err := HashPassword(name)
	wantMessage, _ := bcrypt.GenerateFromPassword([]byte(name), bcrypt.DefaultCost)
	if msg == "" || err != nil {
		t.Errorf(`HashPassword("sample") = %q, %v, want match for %#q, nil`, msg, err, wantMessage)
	}
}

func TestUserPasswordAndHash(t *testing.T) {
	userPassword := "senpai_Yui"
	hashedPw, err := HashPassword("senpai_Yui")
	bcrypt.CompareHashAndPassword([]byte(hashedPw), []byte(userPassword))

	if hashedPw == "" || err != nil {
		t.Errorf(`HashPassword("something") = %q, %v, want match for %#q, nil`, hashedPw, err, userPassword)
	}
}

func TestJWTCreation(t *testing.T) {
	id := uuid.New()
	secret := "amalgami"
	duration := time.Second * 10

	_, err := MakeJWT(id, secret, duration)
	if err != nil {
		t.Errorf(`Conversion failed: %v`, err)
	}

}

func TestVerifyJWTToken(t * testing.T) {
	id := uuid.New()
	secret := "tensorflow"
	duration := time.Second * 10

	token, err := MakeJWT(id, secret, duration)
	if err != nil {
		t.Errorf(`Conversion failed: %v`, err)
	}

	verifyID, err := ValidateJWT(token, "tensorflow")
	if verifyID != id || err != nil {
		t.Errorf(`provided_token_id = %q, %v, want match for %#q, nil`, id, err, verifyID)
	}
}

