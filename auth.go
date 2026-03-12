package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// auth.go — HTTP-обработчики для авторизации и управления пользователями.
//
// В проекте поддерживаются два источника “администратора”:
// 1) “Конфиг-админ” — хранится в `config/config.json` (AdminUsername/AdminPasswordHash).
//    Он нужен для первичной настройки, когда БД ещё пустая/не инициализирована.
//    Для него используется userID == 0 (он не хранится в таблице users).
// 2) Пользователи в БД — таблица `users`.
//
// Важно: логика авторизации сначала проверяет конфиг-админа, и только потом — БД.
// Это упрощает установку и восстановление доступа.

func handleSetup(sm *SystemManager, signingKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		var req AuthRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Некорректный запрос", http.StatusBadRequest)
			return
		}
		if req.Username == "" || req.Password == "" {
			http.Error(w, "Имя пользователя и пароль обязательны", http.StatusBadRequest)
			return
		}
		if len(req.Password) < 3 {
			http.Error(w, "Пароль должен содержать минимум 3 символа", http.StatusBadRequest)
			return
		}

		cfg, err := LoadConfig()
		if err != nil {
			http.Error(w, "Ошибка чтения конфигурации", http.StatusInternalServerError)
			return
		}
		if cfg.AdminPasswordHash != "" {
			http.Error(w, "Настройка уже выполнена", http.StatusForbidden)
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Ошибка хеширования пароля", http.StatusInternalServerError)
			return
		}

		cfg.AdminUsername = req.Username
		cfg.AdminPasswordHash = string(hash)
		if err := SaveConfig(cfg); err != nil {
			http.Error(w, "Ошибка сохранения конфигурации", http.StatusInternalServerError)
			return
		}

		log.Printf("Создан администратор (config): %s", req.Username)
		// Для конфиг-админа используем специальный userID (0), т.к. он не хранится в БД.
		token, user := issueToken(req.Username, 0, "admin", signingKey)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token": token,
			"user":  user,
		})
	}
}

func handleSetupStatus(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		cfg, err := LoadConfig()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]bool{"setup_required": true})
			return
		}

		// ВАЖНО: администратор хранится ТОЛЬКО в config.json.
		// База данных может быть удалена/пересоздана, но это не должно сбрасывать “ответственного за настройки”.
		// Поэтому решение о необходимости первичной настройки зависит только от наличия пароля админа в конфиге.
		json.NewEncoder(w).Encode(map[string]bool{"setup_required": cfg.AdminPasswordHash == ""})
	}
}

func handleLogin(sm *SystemManager, signingKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req AuthRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Некорректный запрос", http.StatusBadRequest)
			return
		}
		// 1. Пробуем авторизовать конфиг-админа (данные берём из config.json)
		if cfg, err := LoadConfig(); err == nil && cfg.AdminPasswordHash != "" {
			adminUser := cfg.AdminUsername
			if adminUser == "" {
				adminUser = "admin"
			}
			if strings.EqualFold(req.Username, adminUser) {
				if err := bcrypt.CompareHashAndPassword([]byte(cfg.AdminPasswordHash), []byte(req.Password)); err == nil {
					token, user := issueToken(adminUser, 0, "admin", signingKey)
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]interface{}{
						"token": token,
						"user":  user,
					})
					return
				}
				// Логин совпадает с админом из конфига, но пароль неверный — сразу ошибка,
				// не пытаемся падать обратно в БД.
				http.Error(w, "Неверное имя пользователя или пароль", http.StatusUnauthorized)
				return
			}
		}

		// 2. Обычные пользователи — авторизация через таблицу users в БД
		dm := sm.GetDB()
		if dm == nil {
			http.Error(w, "База данных недоступна", http.StatusServiceUnavailable)
			return
		}
		id, hash, role, err := dm.GetUserByUsername(req.Username)
		if err != nil {
			http.Error(w, "Неверное имя пользователя или пароль", http.StatusUnauthorized)
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
			http.Error(w, "Неверное имя пользователя или пароль", http.StatusUnauthorized)
			return
		}
		token, user := issueToken(req.Username, id, role, signingKey)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token": token,
			"user":  user,
		})
	}
}

func issueToken(username string, userID int64, role string, signingKey []byte) (string, AuthUser) {
	// JWT выдаём на 24 часа; этого достаточно для домашней библиотеки и упрощает UX.
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString(signingKey)
	return tokenString, AuthUser{Username: username, Role: role}
}

func handleUserStatus(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(claimsContextKey).(*Claims)
	if !ok {
		http.Error(w, "Не удалось получить данные пользователя", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthUser{
		Username: claims.Username,
		Role:     claims.Role,
	})
}

// Смена пароля пользователем (с проверкой старого пароля)
func handleChangePassword(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(claimsContextKey).(*Claims)
		if !ok {
			http.Error(w, "Не удалось получить данные пользователя", http.StatusInternalServerError)
			return
		}
		var req PasswordChangeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Некорректный запрос", http.StatusBadRequest)
			return
		}
		if req.OldPassword == "" || req.NewPassword == "" {
			http.Error(w, "Старый и новый пароль не могут быть пустыми", http.StatusBadRequest)
			return
		}
		if len(req.NewPassword) < 3 {
			http.Error(w, "Новый пароль должен содержать минимум 3 символа", http.StatusBadRequest)
			return
		}
		// Отдельный путь для конфиг-админа (userID == 0, данные в config.json)
		if claims.Role == "admin" && claims.UserID == 0 {
			cfg, err := LoadConfig()
			if err != nil || cfg.AdminPasswordHash == "" {
				http.Error(w, "Конфигурация администратора недоступна", http.StatusInternalServerError)
				return
			}
			if err := bcrypt.CompareHashAndPassword([]byte(cfg.AdminPasswordHash), []byte(req.OldPassword)); err != nil {
				http.Error(w, "Неверный старый пароль", http.StatusUnauthorized)
				return
			}
			newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
			if err != nil {
				http.Error(w, "Ошибка хеширования пароля", http.StatusInternalServerError)
				return
			}
			cfg.AdminPasswordHash = string(newHash)
			if err := SaveConfig(cfg); err != nil {
				http.Error(w, "Ошибка сохранения конфигурации", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Пароль успешно изменён"})
			return
		}

		// Обычные пользователи (и админ, созданный в БД в старой схеме)
		dm := sm.GetDB()
		if dm == nil {
			http.Error(w, "База данных недоступна", http.StatusServiceUnavailable)
			return
		}
		_, hash, _, err := dm.GetUserByID(claims.UserID)
		if err != nil {
			http.Error(w, "Пользователь не найден", http.StatusNotFound)
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)); err != nil {
			http.Error(w, "Неверный старый пароль", http.StatusUnauthorized)
			return
		}
		newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Ошибка хеширования пароля", http.StatusInternalServerError)
			return
		}
		if err := dm.UpdateUserPassword(claims.UserID, string(newHash)); err != nil {
			http.Error(w, "Ошибка сохранения пароля", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Пароль успешно изменён"})
	}
}

// Сброс пароля администратором для любого пользователя
func handleResetPassword(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(claimsContextKey).(*Claims)
		if !ok || claims.Role != "admin" {
			http.Error(w, "Доступ запрещён", http.StatusForbidden)
			return
		}
		var req UserResetPasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Некорректный запрос", http.StatusBadRequest)
			return
		}
		if req.NewPassword == "" || len(req.NewPassword) < 3 {
			http.Error(w, "Новый пароль должен содержать минимум 3 символа", http.StatusBadRequest)
			return
		}
		dm := sm.GetDB()
		if dm == nil {
			http.Error(w, "База данных недоступна", http.StatusServiceUnavailable)
			return
		}
		newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Ошибка хеширования пароля", http.StatusInternalServerError)
			return
		}
		if err := dm.UpdateUserPassword(req.UserID, string(newHash)); err != nil {
			http.Error(w, "Ошибка обновления пароля", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Пароль успешно сброшен"})
	}
}

// Обновление имени пользователя и/или пароля (сам пользователь)
func handleUpdateProfile(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(claimsContextKey).(*Claims)
		if !ok {
			http.Error(w, "Не удалось получить данные пользователя", http.StatusInternalServerError)
			return
		}
		var req UserUpdateSelfRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Некорректный запрос", http.StatusBadRequest)
			return
		}
		// Конфиг-админ (userID == 0): обновляем данные в config.json
		if claims.Role == "admin" && claims.UserID == 0 {
			cfg, err := LoadConfig()
			if err != nil || cfg.AdminPasswordHash == "" {
				http.Error(w, "Конфигурация администратора недоступна", http.StatusInternalServerError)
				return
			}

			// Смена пароля
			if req.NewPassword != "" {
				if req.OldPassword == "" {
					http.Error(w, "Для смены пароля укажите старый пароль", http.StatusBadRequest)
					return
				}
				if err := bcrypt.CompareHashAndPassword([]byte(cfg.AdminPasswordHash), []byte(req.OldPassword)); err != nil {
					http.Error(w, "Неверный старый пароль", http.StatusUnauthorized)
					return
				}
				if len(req.NewPassword) < 3 {
					http.Error(w, "Новый пароль должен содержать минимум 3 символа", http.StatusBadRequest)
					return
				}
				newHash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
				cfg.AdminPasswordHash = string(newHash)
			}

			// Смена имени администратора
			if req.NewUsername != "" && req.NewUsername != cfg.AdminUsername {
				cfg.AdminUsername = req.NewUsername
			}

			if err := SaveConfig(cfg); err != nil {
				http.Error(w, "Ошибка сохранения конфигурации", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Профиль обновлён"})
			return
		}

		// Обычные пользователи (и админ, созданный в БД в старой схеме)
		dm := sm.GetDB()
		if dm == nil {
			http.Error(w, "База данных недоступна", http.StatusServiceUnavailable)
			return
		}
		// ИСПРАВЛЕНИЕ: Правильное получение пользователя (убрана неиспользуемая переменная role)
		username, hash, _, err := dm.GetUserByID(claims.UserID)
		if err != nil {
			http.Error(w, "Пользователь не найден", http.StatusNotFound)
			return
		}
		if req.NewPassword != "" {
			if req.OldPassword == "" {
				http.Error(w, "Для смены пароля укажите старый пароль", http.StatusBadRequest)
				return
			}
			if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)); err != nil {
				http.Error(w, "Неверный старый пароль", http.StatusUnauthorized)
				return
			}
			if len(req.NewPassword) < 3 {
				http.Error(w, "Новый пароль должен содержать минимум 3 символа", http.StatusBadRequest)
				return
			}
			newHash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
			if err := dm.UpdateUserPassword(claims.UserID, string(newHash)); err != nil {
				http.Error(w, "Ошибка сохранения пароля", http.StatusInternalServerError)
				return
			}
		}
		if req.NewUsername != "" && req.NewUsername != username {
			if err := dm.UpdateUsername(claims.UserID, req.NewUsername); err != nil {
				http.Error(w, "Имя пользователя занято или недопустимо", http.StatusBadRequest)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Профиль обновлён"})
	}
}

func handleListUsers(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(claimsContextKey).(*Claims)
		if !ok || claims.Role != "admin" {
			http.Error(w, "Доступ запрещён", http.StatusForbidden)
			return
		}
		dm := sm.GetDB()
		if dm == nil {
			http.Error(w, "База данных недоступна", http.StatusServiceUnavailable)
			return
		}
		list, err := dm.ListUsers()
		if err != nil {
			http.Error(w, "Ошибка получения списка пользователей", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	}
}

func handleCreateUser(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(claimsContextKey).(*Claims)
		if !ok || claims.Role != "admin" {
			http.Error(w, "Доступ запрещён", http.StatusForbidden)
			return
		}
		var req UserCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Некорректный запрос", http.StatusBadRequest)
			return
		}
		if req.Username == "" || req.Password == "" {
			http.Error(w, "Имя пользователя и пароль обязательны", http.StatusBadRequest)
			return
		}
		if len(req.Password) < 3 {
			http.Error(w, "Пароль должен содержать минимум 3 символа", http.StatusBadRequest)
			return
		}
		dm := sm.GetDB()
		if dm == nil {
			http.Error(w, "База данных недоступна", http.StatusServiceUnavailable)
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Ошибка хеширования пароля", http.StatusInternalServerError)
			return
		}
		if err := dm.CreateUser(req.Username, string(hash), "user"); err != nil {
			http.Error(w, "Пользователь с таким именем уже существует", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"message": "Пользователь создан"})
	}
}

func handleDeleteUser(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(claimsContextKey).(*Claims)
		if !ok || claims.Role != "admin" {
			http.Error(w, "Доступ запрещён", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		var req UserDeleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Некорректный запрос", http.StatusBadRequest)
			return
		}
		if req.UserID <= 0 {
			http.Error(w, "Некорректный user_id", http.StatusBadRequest)
			return
		}
		if req.UserID == claims.UserID {
			http.Error(w, "Нельзя удалить самого себя", http.StatusBadRequest)
			return
		}

		dm := sm.GetDB()
		if dm == nil {
			http.Error(w, "База данных недоступна", http.StatusServiceUnavailable)
			return
		}

		_, _, role, err := dm.GetUserByID(req.UserID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Пользователь не найден", http.StatusNotFound)
				return
			}
			http.Error(w, "Ошибка получения пользователя", http.StatusInternalServerError)
			return
		}
		if role == "admin" {
			http.Error(w, "Нельзя удалить администратора", http.StatusBadRequest)
			return
		}

		deleted, err := dm.DeleteUser(req.UserID)
		if err != nil {
			http.Error(w, "Ошибка удаления пользователя", http.StatusInternalServerError)
			return
		}
		if !deleted {
			http.Error(w, "Пользователь не найден", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Пользователь удалён"})
	}
}

// /api/favorites — GET список ID, POST добавить, DELETE убрать (body: { "book_id": number })
func handleFavorites(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(claimsContextKey).(*Claims)
		if !ok {
			http.Error(w, "Не авторизован", http.StatusUnauthorized)
			return
		}
		dm := sm.GetDB()
		if dm == nil {
			http.Error(w, "База данных недоступна", http.StatusServiceUnavailable)
			return
		}

		switch r.Method {
		case http.MethodGet:
			ids, err := dm.GetFavoriteBookIDs(claims.UserID)
			if err != nil {
				http.Error(w, "Ошибка получения избранного", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"book_ids": ids})
			return
		case http.MethodPost:
			var req struct {
				BookID int64 `json:"book_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BookID <= 0 {
				http.Error(w, "Некорректный book_id", http.StatusBadRequest)
				return
			}
			if err := dm.AddFavorite(claims.UserID, req.BookID); err != nil {
				http.Error(w, "Ошибка добавления в избранное", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Добавлено в избранное"})
			return
		case http.MethodDelete:
			var req struct {
				BookID int64 `json:"book_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BookID <= 0 {
				http.Error(w, "Некорректный book_id", http.StatusBadRequest)
				return
			}
			if err := dm.RemoveFavorite(claims.UserID, req.BookID); err != nil {
				http.Error(w, "Ошибка удаления из избранного", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "Удалено из избранного"})
			return
		default:
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		}
	}
}

// GET /api/favorites/books — полные данные книг из избранного
func handleFavoritesBooks(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		claims, ok := r.Context().Value(claimsContextKey).(*Claims)
		if !ok {
			http.Error(w, "Не авторизован", http.StatusUnauthorized)
			return
		}
		dm := sm.GetDB()
		if dm == nil {
			http.Error(w, "База данных недоступна", http.StatusServiceUnavailable)
			return
		}
		ids, err := dm.GetFavoriteBookIDs(claims.UserID)
		if err != nil {
			http.Error(w, "Ошибка получения избранного", http.StatusInternalServerError)
			return
		}
		if len(ids) == 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"books": []interface{}{}})
			return
		}
		books, err := dm.getBooksByIDs(ids)
		if err != nil {
			http.Error(w, "Ошибка загрузки книг", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"books": books})
	}
}

// GET /api/library-stats — статистика библиотеки (только admin)
func handleLibraryStats(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(claimsContextKey).(*Claims)
		if !ok || claims.Role != "admin" {
			http.Error(w, "Доступ запрещён", http.StatusForbidden)
			return
		}
		dm := sm.GetDB()
		if dm == nil {
			http.Error(w, "База данных недоступна", http.StatusServiceUnavailable)
			return
		}
		totalBooks, totalAuthors, uniqueAuthors, genresCount, languagesCount, lastAddedAt, err := dm.GetLibraryStats()
		if err != nil {
			http.Error(w, "Ошибка получения статистики", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		out := map[string]interface{}{
			"total_books":     totalBooks,
			"total_authors":   totalAuthors,
			"unique_authors":  uniqueAuthors,
			"genres_count":    genresCount,
			"languages_count": languagesCount,
		}
		if lastAddedAt != nil {
			out["last_added_at"] = *lastAddedAt
		} else {
			out["last_added_at"] = nil
		}
		json.NewEncoder(w).Encode(out)
	}
}

func jwtAuthMiddleware(signingKey []byte) func(http.Handler) http.Handler {
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
			claims := &Claims{}
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
			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func adminOnlyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(claimsContextKey).(*Claims)
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
