package config

import (
	"crypto/rand"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"web_books/internal/domain"
)

var mu sync.Mutex

// CreateDefault builds a new default configuration.
func CreateDefault() (*domain.Config, error) {
	return createDefaultConfig()
}

func createDefaultConfig() (*domain.Config, error) {
	jwtKey := make([]byte, domain.JWTKeyLength)
	if _, err := rand.Read(jwtKey); err != nil {
		return nil, fmt.Errorf("не удалось сгенерировать ключ JWT: %v", err)
	}
	return &domain.Config{
		BooksDir:          "",
		Port:              "8080",
		OPDSRoot:          "/opds",
		AdminPasswordHash: "",
		JWTSigningKey:     jwtKey,
		ReaderEnabled:     false,
		ReaderURL:         "https://reader.example.com/#/read?url=",
	}, nil
}

// SaveInternal persists configuration without extra validation (tests).
func SaveInternal(cfg *domain.Config) error {
	return saveInternal(cfg)
}

func saveInternal(cfg *domain.Config) error {
	configDir := "config"
	configPath := filepath.Join(configDir, "config.json")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		if err := os.Mkdir(configDir, 0755); err != nil {
			return fmt.Errorf("не удалось создать папку config: %v", err)
		}
	}
	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("ошибка создания config.json: %v", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("ошибка кодирования config.json: %v", err)
	}
	return nil
}

func Load() (*domain.Config, error) {
	mu.Lock()
	defer mu.Unlock()

	configDir := "config"
	configPath := filepath.Join(configDir, "config.json")
	oldConfigPath := filepath.Join(configDir, "config.bin")

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		if err := os.Mkdir(configDir, 0755); err != nil {
			return nil, fmt.Errorf("не удалось создать папку config: %v", err)
		}
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if _, errBin := os.Stat(oldConfigPath); errBin == nil {
			log.Println("Обнаружен старый конфиг (gob), выполняется миграция на JSON...")
			oldFile, err := os.Open(oldConfigPath)
			if err == nil {
				var oldCfg domain.Config
				if err := gob.NewDecoder(oldFile).Decode(&oldCfg); err == nil {
					oldFile.Close()
					if err := saveInternal(&oldCfg); err == nil {
						log.Println("Миграция успешна. Удаление старого файла.")
						os.Remove(oldConfigPath)
						return &oldCfg, nil
					}
				}
				oldFile.Close()
			}
			log.Println("Не удалось мигрировать старый конфиг, создается новый.")
		}
		log.Printf("Файл конфигурации %s не найден, создается новый.", configPath)
		defaultCfg, err := createDefaultConfig()
		if err != nil {
			return nil, err
		}
		if err := saveInternal(defaultCfg); err != nil {
			return nil, fmt.Errorf("не удалось создать config.json: %v", err)
		}
		return defaultCfg, nil
	}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения config.json: %v", err)
	}
	defer file.Close()

	var cfg domain.Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		log.Printf("Ошибка декодирования config.json: %v. Создается новый.", err)
		defaultCfg, err := createDefaultConfig()
		if err != nil {
			return nil, err
		}
		if errSave := saveInternal(defaultCfg); errSave != nil {
			return nil, fmt.Errorf("не удалось перезаписать поврежденный config.json: %v", errSave)
		}
		return defaultCfg, nil
	}

	needsSave := false
	if cfg.AdminPasswordHash != "" && cfg.AdminUsername == "" {
		log.Println("Обнаружен admin_password_hash без admin_username — используется имя 'admin'")
		cfg.AdminUsername = "admin"
		needsSave = true
	}
	if cfg.JWTSigningKey == nil {
		log.Println("Ключ JWT отсутствует. Генерируется новый ключ.")
		jwtKey := make([]byte, domain.JWTKeyLength)
		if _, err := rand.Read(jwtKey); err != nil {
			return nil, fmt.Errorf("не удалось сгенерировать ключ JWT (миграция): %v", err)
		}
		cfg.JWTSigningKey = jwtKey
		needsSave = true
	}
	if cfg.ReaderURL == "" {
		log.Println("URL читалки отсутствует. Устанавливается значение по умолчанию.")
		cfg.ReaderEnabled = false
		cfg.ReaderURL = "https://read.books-kriksa.ru/#/reader"
		needsSave = true
	}
	if needsSave {
		if err := saveInternal(&cfg); err != nil {
			return nil, fmt.Errorf("не удалось сохранить обновленный config.json (миграция): %v", err)
		}
	}
	return &cfg, nil
}

func Save(cfg *domain.Config) error {
	mu.Lock()
	defer mu.Unlock()
	if err := saveInternal(cfg); err != nil {
		return err
	}
	log.Println("Конфигурация сохранена в config/config.json")
	return nil
}

func FindLatestINPX(c *domain.Config) (string, error) {
	files, err := filepath.Glob(filepath.Join(c.BooksDir, "*.inpx"))
	if err != nil {
		return "", fmt.Errorf("ошибка поиска INPX-файлов: %v", err)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("INPX-файлы не найдены в %s", c.BooksDir)
	}
	var latestFile string
	var latestTime time.Time
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestFile = file
		}
	}
	if latestFile == "" {
		return "", fmt.Errorf("не удалось выбрать INPX-файл")
	}
	return latestFile, nil
}
