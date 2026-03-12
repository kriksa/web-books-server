package main

import (
	"encoding/gob"
	"log"
)

// main.go — точка входа приложения.
//
// Приложение:
// - поднимает HTTP-сервер со SPA фронтендом (встроенным в бинарник через embed);
// - предоставляет API поиска/скачивания/OPDS;
// - при необходимости запускает фоновый парсер INPX для наполнения базы.
func main() {
	gob.Register(Config{})
	sysMgr := NewSystemManager()
	log.Println("Инициализация системы...")
	startWebServer(sysMgr)
}