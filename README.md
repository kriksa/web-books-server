Web Books Server — сервер для личной библиотеки электронных книг. Программа работает с INPX-каталогами, такими как архивы библиотеки Flibusta, автоматически парсит их в SQLite. Предоставляет веб-интерфейс с поиском, группировкой по авторам/сериям, избранным и OPDS-каталог для ридеров.
В веб интерфейсе есть возможность использовать та
кие читалки как Liberama (https://github.com/bookpauk/liberama)
<details>
<summary>Скриншоты</summary>
<img width="1279" height="895" alt="Screenshot_20260317_161328" src="https://github.com/user-attachments/assets/31432786-1a1c-4b33-95fd-b67e41ad7cd0" />
Светлая тема

<img width="1279" height="895" alt="Screenshot_20260317_161252" src="https://github.com/user-attachments/assets/7b59b510-281d-47ca-9984-ed9bfa123848" />
Темная тема

<img width="1279" height="895" alt="Screenshot_20260317_161408" src="https://github.com/user-attachments/assets/a369caa5-c509-44c6-bca3-ffd394903d03" />
Модальное окно с информацией о книге
</details>




Установка и запуск

Вариант 1: Готовые бинарники (релизы)

Скачайте последнюю версию на странице релизов (готовая сборка есть только под Linux):

Запустите скачанный файл и откройте браузер на http://localhost:8080.

Вариант 2: Сборка из исходников

Требования: Go 1.23+, Node.js 18+, npm.

**Linux / macOS**

```bash
make build          # web → internal/spaembed/spa + bin/web_books (без Liberama)
./bin/web_books
```

Полная сборка с Liberama (как раньше `./script.sh`):

```bash
chmod +x script.sh
./script.sh
```

Только фронт + Go (без Liberama): `./scripts/build.sh` или `make build`.

**Windows (PowerShell)**

```powershell
.\scripts\build.ps1
.\bin\web_books.exe
```

**Полная сборка с Liberama** (опционально): клонируйте [liberama](https://github.com/bookpauk/liberama) в `third_party/liberama`, затем `make build-full` (Linux) или соберите Liberama вручную в `internal/spaembed/spa/liberama/`.

При первом запуске создайте администратора и укажите путь к папке с INPX/ZIP файлами. База заполнится автоматически.

### Структура репозитория

```
cmd/web_books/          # точка входа
internal/app/           # HTTP API, OPDS, парсер, SQLite, конвертация
internal/formats/       # epub, docx
internal/spaembed/      # встроенный фронт (артефакт Vite, не в git)
web/                    # Vue + Vite, data/genres.js, languages.js
assets/                 # favicon, фоны, liberama-modern.css
liberama-1.3.2/         # исходники Liberama (без node_modules/dist)
config/                 # config.json создаётся при первом запуске
scripts/                # build.sh, build.ps1
deploy/systemd/         # unit-файл для Linux
script.sh               # полная сборка на Linux
Makefile
```

### Что переносить на сервер

Достаточно исходников: `cmd/`, `internal/`, `web/`, `assets/`, `liberama-1.3.2/` (или клон в `third_party/liberama`), `go.mod`, `go.sum`, `script.sh`, `Makefile`.

**Не переносить** (создаются при сборке/запуске): `node_modules`, `bin/`, `internal/spaembed/spa/*`, `config/config.json`, базы SQLite, `uploads/`.

На сервере: `chmod +x script.sh && ./script.sh` → `bin/web_books`.

**systemd:** скопируйте `deploy/systemd/web-books.service`, поправьте `User` и `WorkingDirectory`, затем `systemctl enable --now web-books`.
