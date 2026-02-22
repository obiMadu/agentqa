package auth

import (
  "errors"
  "time"

  "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
  UserID string `json:"user_id"`
  Email  string `json:"email"`
  jwt.RegisteredClaims
}

type TokenService struct {
  secret []byte
}

func NewTokenService(secret string) *TokenService {
  return &TokenService{secret: []byte(secret)}
}

func (t *TokenService) Issue(userID, email string) (accessToken string, refreshToken string, err error) {
  if len(t.secret) == 0 {
    return "", "", errors.New("missing JWT secret")
  }

  accessToken, err = t.sign(userID, email, 15*time.Minute)
  if err != nil {
    return "", "", err
  }

  refreshToken, err = t.sign(userID, email, 30*24*time.Hour)
  if err != nil {
    return "", "", err
  }

  return accessToken, refreshToken, nil
}

func (t *TokenService) Parse(token string) (Claims, error) {
  if len(t.secret) == 0 {
    return Claims{}, errors.New("missing JWT secret")
  }

  parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(_ *jwt.Token) (interface{}, error) {
    return t.secret, nil
  })
  if err != nil {
    return Claims{}, err
  }

  claims, ok := parsed.Claims.(*Claims)
  if !ok || !parsed.Valid {
    return Claims{}, errors.New("invalid token")
  }

  return *claims, nil
}

func (t *TokenService) sign(userID, email string, ttl time.Duration) (string, error) {
  now := time.Now()
  claims := Claims{
    UserID: userID,
    Email:  email,
    RegisteredClaims: jwt.RegisteredClaims{
      IssuedAt:  jwt.NewNumericDate(now),
      ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
    },
  }

  token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
  return token.SignedString(t.secret)
}
