package auth

import (
	"testing"

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
