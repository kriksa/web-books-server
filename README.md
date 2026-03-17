Web Books Server — сервер для личной библиотеки электронных книг. Программа работает с INPX-каталогами, такими как архивы библиотеки Flibusta, автоматически парсит их в SQLite. Предоставляет веб-интерфейс с поиском, группировкой по авторам/сериям, избранным и OPDS-каталог для ридеров.
В веб интерфейсе есть возможность использовать та
кие читалки как Liberama (https://github.com/bookpauk/liberama)
<details>
<summary>Скриншоты:</summary>
<img width="1279" height="895" alt="Screenshot_20260317_161328" src="https://github.com/user-attachments/assets/31432786-1a1c-4b33-95fd-b67e41ad7cd0" />

<img width="1279" height="895" alt="Screenshot_20260317_161252" src="https://github.com/user-attachments/assets/7b59b510-281d-47ca-9984-ed9bfa123848" />

<img width="1279" height="895" alt="Screenshot_20260317_161408" src="https://github.com/user-attachments/assets/a369caa5-c509-44c6-bca3-ffd394903d03" />
</details>




Установка и запуск

Вариант 1: Готовые бинарники (релизы)

Скачайте последнюю версию на странице релизов (готовая сборка есть только под Linux):

Запустите скачанный файл и откройте браузер на http://localhost:8080.

Вариант 2: Сборка из исходников

Требования: Go 1.23+, Node.js 18+, npm
bash

git clone https://github.com/kriksa/web-books-server

cd web-books-server

chmod +x script.sh

./script.sh

./web_books

При первом запуске создайте администратора и укажите путь к папке с INPX/ZIP файлами. База заполнится автоматически.
