package utils

import (
	"errors"
	"os"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
)

// Claims represents the JWT claims
type Claims struct {
	UserID  uuid.UUID `json:"user_id"`
	Email   string    `json:"email"`
	IsAdmin bool      `json:"is_admin"`
	jwt.StandardClaims
}

// TokenPair represents an access and refresh token pair
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds
	TokenType    string `json:"token_type"`
}

// getJWTSecret returns the JWT secret from environment variable or a default for development
func getJWTSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// Default secret for development only
		// In production, this should be set as an environment variable
		return "revaspay_development_jwt_secret_key"
	}
	return secret
}

// GenerateTokenPair creates access and refresh tokens
func GenerateTokenPair(userID uuid.UUID, email string, isAdmin bool) (TokenPair, error) {
	// Set expiration times
	accessExpiration := time.Now().Add(15 * time.Minute)
	refreshExpiration := time.Now().Add(7 * 24 * time.Hour)

	// Create claims for access token
	accessClaims := Claims{
		UserID:  userID,
		Email:   email,
		IsAdmin: isAdmin,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: accessExpiration.Unix(),
		},
	}

	// Create claims for refresh token
	refreshClaims := Claims{
		UserID:  userID,
		Email:   email,
		IsAdmin: isAdmin,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: refreshExpiration.Unix(),
		},
	}

	// Create tokens
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)

	// Sign tokens
	jwtSecret := getJWTSecret()
	accessTokenString, err := accessToken.SignedString([]byte(jwtSecret))
	if err != nil {
		return TokenPair{}, err
	}

	refreshTokenString, err := refreshToken.SignedString([]byte(jwtSecret))
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresIn:    accessExpiration.Unix() - time.Now().Unix(),
		TokenType:    "Bearer",
	}, nil
}

// ValidateToken validates a JWT token and returns the claims
func ValidateToken(tokenString string) (*Claims, error) {
	// Get JWT secret
	jwtSecret := getJWTSecret()
	
	// Parse token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(jwtSecret), nil
	})

	if err != nil {
		return nil, err
	}

	// Validate token
	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	// Extract claims
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("failed to parse token claims")
	}

	return claims, nil
}
