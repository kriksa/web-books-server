package main

import (
	"encoding/gob"
	"log"

	"web_books/internal/app"
	"web_books/internal/domain"
)

func main() {
	gob.Register(domain.Config{})
	log.Println("Инициализация системы...")
	app.Run()
}
