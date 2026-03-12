#!/bin/bash
set -euo pipefail

echo "🚀 Начинаем сборку проекта..."

# Загружаем nvm, если доступно
if [ -f "$HOME/.nvm/nvm.sh" ]; then
    export NVM_DIR="$HOME/.nvm"
    [ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
    nvm use node || true
fi

# Создаём необходимые директории
mkdir -p frontend/dist frontend/src

# Проверяем наличие Node.js
if ! command -v node &> /dev/null; then
    echo "❌ Ошибка: Node.js не установлен или не найден"
    echo "Установите Node.js или настройте nvm."
    exit 1
fi
echo "✅ Node.js: $(node --version)"
echo "✅ npm: $(npm --version)"

# Проверяем наличие Go
if ! command -v go &> /dev/null; then
    echo "❌ Ошибка: Go не установлен"
    exit 1
fi
echo "✅ Go: $(go version)"

# === Фронтенд ===
cat > frontend/package.json << 'EOF'
{
  "name": "books-frontend",
  "version": "1.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "vue": "^3.5.0",
    "lucide-vue-next": "^0.446.0"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^5.1.0",
    "vite": "^5.4.0",
    "tailwindcss": "^3.4.0",
    "autoprefixer": "^10.4.20",
    "postcss": "^8.4.47"
  }
}
EOF

cat > frontend/vite.config.js << 'EOF'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: 'dist',
    assetsDir: 'assets'
  }
})
EOF

cat > frontend/tailwind.config.js << 'EOF'
/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {}
  },
  plugins: [],
}
EOF

cat > frontend/postcss.config.js << 'EOF'
export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
}
EOF

cat > frontend/src/main.js << 'EOF'
import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
createApp(App).mount('#app')
EOF

cat > frontend/src/style.css << 'EOF'
@tailwind base;
@tailwind components;
@tailwind utilities;
body {
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
}
* {
  transition: background-color 0.3s ease, border-color 0.3s ease, color 0.3s ease;
}
EOF

cat > frontend/index.html << 'EOF'
<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Моя библиотека</title>

</head>
<body>
    <div id="app"></div>
    <script type="module" src="/src/main.js"></script>
</body>
</html>
EOF

# Копируем компоненты
for vue_file in App.vue Settings.vue; do
    if [ -f "$vue_file" ]; then
        cp "$vue_file" frontend/src/
        echo "✅ $vue_file скопирован в frontend/src/"
    else
        echo "❌ Файл $vue_file не найден"
        exit 1
    fi
done

# Копируем карты жанров и языков (нужны для импорта в App.vue)
mkdir -p frontend/src/data
for data_file in data/genres.js data/languages.js; do
    if [ -f "$data_file" ]; then
        cp "$data_file" frontend/src/data/
        echo "✅ $data_file скопирован в frontend/src/data/"
    else
        echo "❌ Файл $data_file не найден"
        exit 1
    fi
done

echo "📦 Устанавливаем зависимости Vue..."
cd frontend
npm install --legacy-peer-deps
echo "🔨 Собираем Vue приложение..."
npm run build
cd ..

# === Бэкенд ===
cat > go.mod << 'EOF'
module web_books
go 1.23

require (
    github.com/golang-jwt/jwt/v5 v5.2.1
    github.com/hashicorp/golang-lru/v2 v2.0.7
    github.com/timsims/pamphlet v0.1.6
    golang.org/x/crypto v0.25.0
    golang.org/x/text v0.16.0
    modernc.org/sqlite v1.33.1
)
EOF

echo "📥 Загружаем зависимости Go..."
go mod tidy

echo "🔨 Компилируем Go программу..."
# Собираем весь пакет (все .go файлы в папке)
go build -ldflags="-s -w" -o web_books .

echo ""
echo "✅ Сборка завершена!"
echo ""
echo "🚀 Для запуска выполните:"
echo "   (Сначала убедитесь, что все .go файлы находятся в этой же папке)"
echo "   ./web_books"
echo ""
echo "📝 Не забудьте:"
echo "   1. Настроить конфигурацию через веб-интерфейс (вкладка 'Настройки')"
echo "   2. Положить INPX-файлы в папку с книгами (указывается в настройках)"
echo ""

