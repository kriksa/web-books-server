package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"web_books/internal/domain"
)

type contextKey string

// ClaimsKey is the context key for JWT claims.
const ClaimsKey contextKey = "claims"

// Claims is an alias for domain JWT claims.
type Claims = domain.Claims

// JWT returns middleware that requires a valid Bearer token.
func JWT(signingKey []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Отсутствует заголовок Authorization", http.StatusUnauthorized)
				return
			}
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenString == authHeader {
				http.Error(w, "Некорректный формат токена", http.StatusUnauthorized)
				return
			}
			claims := &domain.Claims{}
			token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return signingKey, nil
			})
			if err != nil || !token.Valid {
				if errors.Is(err, jwt.ErrSignatureInvalid) {
					http.Error(w, "Неверная подпись токена", http.StatusUnauthorized)
					return
				}
				http.Error(w, "Невалидный токен", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminOnly restricts handlers to admin role.
func AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := FromContext(r)
		if !ok {
			http.Error(w, "Ошибка авторизации: нет данных пользователя", http.StatusUnauthorized)
			return
		}
		if claims.Role != "admin" {
			http.Error(w, "Доступ запрещен: требуются права администратора", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// FromContext returns JWT claims attached by JWT middleware.
func FromContext(r *http.Request) (*domain.Claims, bool) {
	claims, ok := r.Context().Value(ClaimsKey).(*domain.Claims)
	return claims, ok
}
