package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type MyCustomClaims struct {
	jwt.RegisteredClaims
}

func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("error while hashing password")
	}
	strHash := string(hashedPassword)
	return strHash, nil
}

func CheckPasswordHash(password, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return fmt.Errorf("the hashed password and entered password do not match")
	}
	return nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	regClaims := jwt.RegisteredClaims{
		Issuer:    "chirpy",
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
		Subject:   userID.String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, regClaims)
	// signing the token
	// made some mistakes here
	// token.SignedString([]byte(tokenSecret))
	// // jwt.NewWithClaims(jwt.SigningMethodHS256)
	// return token.Raw, nil

	signed, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", err
	}
	return signed, nil

}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claims := jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(tokenSecret), nil
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("error occured while parsing token")
	} else if claims, ok := token.Claims.(*jwt.RegisteredClaims); ok { // understand this line of code really well; type assertion
		fmt.Println(claims.Issuer)
	} else {
		return uuid.Nil, fmt.Errorf("unknown claims type")
	}

	sub, err := token.Claims.GetSubject()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error encountered while getting the subject")
	}
	userId, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid userId")
	}
	return userId, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	bToken := headers.Get("Authorization")
	if bToken == "" {
		return "", fmt.Errorf("no authorization header receiveed")
	}
	
	splitB := strings.Split(strings.TrimSpace(bToken), " ")
	tokenStr := "" 
	if len(splitB) == 2 {
		temp := splitB[0]
		if temp != "Bearer"{
			return "", fmt.Errorf("invalid authorization token received")
		}
		tokenStr = strings.TrimSpace(splitB[1])
	} else {
		return "", fmt.Errorf("header Authorization does not contain token string")
	}
	return tokenStr, nil 
}
