package main

import (
	"crypto/rand"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// config.go — загрузка/сохранение конфигурации приложения.
//
// Особенности:
// - Конфиг хранится в `config/config.json`.
// - Поддерживается миграция со старого бинарного формата `config/config.bin` (gob).
// - При необходимости автоматически создаются директории и дефолтный конфиг.
//
// Важно: операции чтения/записи защищены мьютексом `configMu`, чтобы:
// - параллельные HTTP-запросы не портили файл;
// - миграция не выполнялась одновременно несколькими горутинами.

func createDefaultConfig() (*Config, error) {
	jwtKey := make([]byte, JWTKeyLength)
	if _, err := rand.Read(jwtKey); err != nil {
		return nil, fmt.Errorf("не удалось сгенерировать ключ JWT: %v", err)
	}

	return &Config{
		BooksDir:          "",
		Port:              "8080",
		OPDSRoot:          "/opds",
		AdminPasswordHash: "",
		JWTSigningKey:     jwtKey,
		ReaderEnabled:     false,
		ReaderURL:         "https://reader.example.com/#/read?url=",
	}, nil
}

func saveConfigInternal(cfg *Config) error {
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

func LoadConfig() (*Config, error) {
	configMu.Lock()
	defer configMu.Unlock()

	// Папка "config" лежит рядом с бинарником/рабочей директорией приложения.
	// Это сознательное решение: проект рассчитан на “portable” запуск.
	configDir := "config"
	configPath := filepath.Join(configDir, "config.json")
	oldConfigPath := filepath.Join(configDir, "config.bin")

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		if err := os.Mkdir(configDir, 0755); err != nil {
			return nil, fmt.Errorf("не удалось создать папку config: %v", err)
		}
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Если config.json ещё нет — пытаемся мигрировать старый gob-конфиг,
		// иначе создаём новый.
		if _, errBin := os.Stat(oldConfigPath); errBin == nil {
			log.Println("Обнаружен старый конфиг (gob), выполняется миграция на JSON...")
			oldFile, err := os.Open(oldConfigPath)
			if err == nil {
				var oldCfg Config
				if err := gob.NewDecoder(oldFile).Decode(&oldCfg); err == nil {
					oldFile.Close()
					if err := saveConfigInternal(&oldCfg); err == nil {
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
		if err := saveConfigInternal(defaultCfg); err != nil {
			return nil, fmt.Errorf("не удалось создать config.json: %v", err)
		}
		return defaultCfg, nil
	}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения config.json: %v", err)
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		log.Printf("Ошибка декодирования config.json: %v. Создается новый.", err)
		defaultCfg, err := createDefaultConfig()
		if err != nil {
			return nil, err
		}
		if errSave := saveConfigInternal(defaultCfg); errSave != nil {
			return nil, fmt.Errorf("не удалось перезаписать поврежденный config.json: %v", errSave)
		}
		return defaultCfg, nil
	}

	needsSave := false

	// Миграция: если есть пароль администратора, но нет имени — используем "admin" по умолчанию.
	if cfg.AdminPasswordHash != "" && cfg.AdminUsername == "" {
		log.Println("Обнаружен admin_password_hash без admin_username — используется имя 'admin'")
		cfg.AdminUsername = "admin"
		needsSave = true
	}

	if cfg.JWTSigningKey == nil {
		// Без JWT ключа авторизация работать не сможет, поэтому создаём его автоматически.
		log.Println("Ключ JWT отсутствует. Генерируется новый ключ.")
		jwtKey := make([]byte, JWTKeyLength)
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
		if err := saveConfigInternal(&cfg); err != nil {
			return nil, fmt.Errorf("не удалось сохранить обновленный config.json (миграция): %v", err)
		}
	}

	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	configMu.Lock()
	defer configMu.Unlock()

	if err := saveConfigInternal(cfg); err != nil {
		return err
	}

	log.Println("Конфигурация сохранена в config/config.json")
	return nil
}

func (c *Config) FindLatestINPX() (string, error) {
	// INPX выбираем по времени модификации: пользователь может регулярно обновлять коллекцию,
	// и при наличии нескольких inpx мы берём самый свежий.
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
