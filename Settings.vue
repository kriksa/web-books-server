<!-- Settings.vue -->
<script setup>
import { ref, onMounted } from 'vue';
import { Save, X, UserPlus, BarChart3 } from 'lucide-vue-next';
import { LANGUAGES_MAP } from './data/languages.js';

const props = defineProps({
  token: String
});

const emit = defineEmits(['close', 'config-saved']);

const config = ref({
  books_dir: '',
  port: '8080',
  web_password: '',
  reader_enabled: false,
  reader_url: 'https://reader.example.com/#/read?url=',
  default_search_language: ''
});

const libraryStats = ref(null);
const defaultLanguageOptions = ref([{ code: '', name: 'Не задан' }, ...Object.entries(LANGUAGES_MAP).map(([code, name]) => ({ code, name })).sort((a, b) => a.name.localeCompare(b.name))]);
const settingsError = ref('');
const settingsSuccess = ref('');
const changePasswordData = ref({
  old_password: '',
  new_password: '',
  confirm_password: ''
});
const passwordError = ref('');
const passwordSuccess = ref('');
const users = ref([]);
const newUser = ref({ username: '', password: '' });
const userCreateError = ref('');
const userCreateSuccess = ref('');
const resetUser = ref(null);
const resetPassword = ref('');
const resetError = ref('');
const userDeleteError = ref('');
const userDeleteSuccess = ref('');

// Сохранение настроек
const saveSettings = async () => {
  if (!config.value.books_dir) {
    settingsError.value = 'Поле "Папка с книгами" обязательно';
    return;
  }
  try {
    const response = await fetch('/api/config', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${props.token}`
      },
        body: JSON.stringify({
        books_dir: config.value.books_dir,
        port: config.value.port,
        web_password: config.value.web_password,
        reader_enabled: config.value.reader_enabled,
        reader_url: config.value.reader_url,
        default_search_language: config.value.default_search_language || ''
      }),
    });
    const data = await response.json();
    if (response.ok) {
      settingsSuccess.value = 'Конфигурация успешно сохранена.';
      settingsError.value = '';
      emit('config-saved');
    } else {
      settingsError.value = data.message || 'Ошибка сохранения конфигурации';
    }
  } catch (err) {
    settingsError.value = 'Ошибка связи с сервером';
    console.error('Ошибка сохранения настроек:', err);
  }
};

// Функция для смены пароля
const changePassword = async () => {
  if (changePasswordData.value.new_password !== changePasswordData.value.confirm_password) {
    passwordError.value = 'Новые пароли не совпадают';
    return;
  }

  if (changePasswordData.value.new_password.length < 3) {
    passwordError.value = 'Пароль должен содержать минимум 3 символа';
    return;
  }

  try {
    const response = await fetch('/api/change-password', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${props.token}`
      },
      body: JSON.stringify({
        old_password: changePasswordData.value.old_password,
        new_password: changePasswordData.value.new_password
      }),
    });

    const data = await response.json();
    if (response.ok) {
      passwordSuccess.value = 'Пароль успешно изменен';
      passwordError.value = '';
      changePasswordData.value = {
        old_password: '',
        new_password: '',
        confirm_password: ''
      };
    } else {
      passwordError.value = data.message || 'Ошибка смены пароля';
    }
  } catch (err) {
    passwordError.value = 'Ошибка связи с сервером';
    console.error('Ошибка смены пароля:', err);
  }
};

// Загрузка списка пользователей
const fetchUsers = async () => {
  try {
    const r = await fetch('/api/users', {
      headers: {
        Authorization: `Bearer ${props.token}`
      }
    });
    if (r.ok) {
      const data = await r.json();
      // Защита от некорректного ответа сервера:
      // ожидаем массив пользователей, но на всякий случай нормализуем в [].
      users.value = Array.isArray(data) ? data : [];
    } else {
      console.error('Ошибка загрузки пользователей:', r.status);
    }
  } catch (e) {
    console.error('Ошибка загрузки пользователей:', e);
  }
};

// Создание нового пользователя
const createUser = async () => {
  userCreateError.value = '';
  userCreateSuccess.value = '';

  if (!newUser.value.username || !newUser.value.password) {
    userCreateError.value = 'Заполните имя и пароль';
    return;
  }

  if (newUser.value.password.length < 3) {
    userCreateError.value = 'Пароль минимум 3 символа';
    return;
  }

  try {
    const r = await fetch('/api/users/create', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${props.token}`
      },
      body: JSON.stringify({
        username: newUser.value.username,
        password: newUser.value.password
      })
    });

    const data = await r.json();
    if (r.ok) {
      userCreateSuccess.value = 'Пользователь создан';
      newUser.value = { username: '', password: '' };
      // Обновляем список пользователей после создания
      await fetchUsers();
    } else {
      userCreateError.value = data.message || 'Ошибка создания пользователя';
    }
  } catch (e) {
    userCreateError.value = 'Ошибка связи с сервером';
    console.error('Ошибка создания пользователя:', e);
  }
};

// Сброс пароля пользователя
const doResetPassword = async () => {
  resetError.value = '';

  if (!resetPassword.value || resetPassword.value.length < 3) {
    resetError.value = 'Пароль минимум 3 символа';
    return;
  }

  try {
    const r = await fetch('/api/reset-password', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${props.token}`
      },
      body: JSON.stringify({
        user_id: resetUser.value.id,
        new_password: resetPassword.value
      })
    });

    const data = await r.json();
    if (r.ok) {
      resetUser.value = null;
      resetPassword.value = '';
      userCreateSuccess.value = 'Пароль успешно сброшен';
      await fetchUsers();
    } else {
      resetError.value = data.message || 'Ошибка сброса пароля';
    }
  } catch (e) {
    resetError.value = 'Ошибка связи с сервером';
    console.error('Ошибка сброса пароля:', e);
  }
};

// Удаление пользователя
const deleteUser = async (user) => {
  userDeleteError.value = '';
  userDeleteSuccess.value = '';

  if (!confirm(`Удалить пользователя "${user.username}"?`)) {
    return;
  }

  try {
    const r = await fetch('/api/users/delete', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${props.token}`
      },
      body: JSON.stringify({
        user_id: user.id
      })
    });

    const data = await r.json();
    if (r.ok) {
      userDeleteSuccess.value = 'Пользователь удалён';
      await fetchUsers();
    } else {
      userDeleteError.value = data.message || 'Ошибка удаления пользователя';
    }
  } catch (e) {
    userDeleteError.value = 'Ошибка связи с сервером';
    console.error('Ошибка удаления пользователя:', e);
  }
};

// Закрытие окна настроек
const closeModal = () => {
  emit('close');
};

// Инициализация при монтировании
onMounted(async () => {
  try {
    // Загружаем конфигурацию
    const response = await fetch('/api/config', {
      headers: {
        'Authorization': `Bearer ${props.token}`
      }
    });

    if (response.ok) {
      const data = await response.json();
      config.value = {
        ...config.value,
        ...data,
        default_search_language: data.default_search_language || ''
      };

      // Убедимся что поля для читалки есть
      if (config.value.reader_enabled === undefined) {
        config.value.reader_enabled = false;
      }
      if (!config.value.reader_url) {
        config.value.reader_url = 'https://reader.example.com/#/read?url=';
      }
    } else {
      const errorData = await response.json();
      settingsError.value = errorData.message || 'Ошибка загрузки конфигурации';
    }

    // Загружаем список пользователей
    await fetchUsers();

    // Статистика библиотеки (админ)
    try {
      const r = await fetch('/api/library-stats', { headers: { Authorization: `Bearer ${props.token}` } });
      if (r.ok) libraryStats.value = await r.json();
    } catch (e) { console.error('Ошибка загрузки статистики', e); }

  } catch (err) {
    settingsError.value = 'Ошибка связи с сервером';
    console.error('Ошибка инициализации настроек:', err);
  }
});
</script>

<template>
  <div class="min-h-screen flex items-center justify-center p-4 bg-gradient-to-br from-gray-900 via-gray-800 to-gray-900">
    <div class="w-full max-w-2xl bg-gray-800 dark:bg-gray-900 rounded-xl shadow-2xl p-6 transition-colors duration-300 border border-gray-700 max-h-[90vh] overflow-y-auto">
      <div class="flex justify-between items-center mb-6">
        <h2 class="text-2xl font-bold text-white">Настройки</h2>
        <button @click="closeModal" class="p-1.5 rounded-full text-gray-400 hover:text-white hover:bg-gray-700">
          <X class="h-5 w-5" />
        </button>
      </div>

      <!-- Секция смены пароля -->
      <div class="mb-8 p-4 border border-gray-600 rounded-lg">
        <h3 class="text-lg font-semibold text-white mb-4">Смена пароля</h3>
        <form @submit.prevent="changePassword" class="space-y-4">
          <div>
            <label for="old_password" class="block text-sm font-medium mb-1 text-gray-300">Старый пароль</label>
            <input v-model="changePasswordData.old_password" type="password" id="old_password"
                   class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors bg-gray-700 border-gray-600 text-white focus:ring-indigo-500"
                   placeholder="Введите старый пароль" />
          </div>
          <div>
            <label for="new_password" class="block text-sm font-medium mb-1 text-gray-300">Новый пароль</label>
            <input v-model="changePasswordData.new_password" type="password" id="new_password"
                   class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors bg-gray-700 border-gray-600 text-white focus:ring-indigo-500"
                   placeholder="Введите новый пароль" />
          </div>
          <div>
            <label for="confirm_password" class="block text-sm font-medium mb-1 text-gray-300">Подтверждение пароля</label>
            <input v-model="changePasswordData.confirm_password" type="password" id="confirm_password"
                   class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors bg-gray-700 border-gray-600 text-white focus:ring-indigo-500"
                   placeholder="Повторите новый пароль" />
          </div>
          <div v-if="passwordError" class="text-red-400 text-sm">{{ passwordError }}</div>
          <div v-if="passwordSuccess" class="text-green-400 text-sm">{{ passwordSuccess }}</div>
          <div class="flex justify-end">
            <button type="submit"
                    class="inline-flex items-center gap-2 px-4 py-2.5 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white transition-colors">
              <Save class="h-4 w-4" /> Сменить пароль
            </button>
          </div>
        </form>
      </div>

      <!-- Основные настройки -->
      <form @submit.prevent="saveSettings" class="space-y-4">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label for="books_dir" class="block text-sm font-medium mb-1 text-gray-300">Папка с книгами</label>
            <input v-model="config.books_dir" type="text" id="books_dir"
                   class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors bg-gray-700 border-gray-600 text-white focus:ring-indigo-500"
                   placeholder="/path/to/books" />
          </div>
          <div class="md:col-span-1">
            <label for="port" class="block text-sm font-medium mb-1 text-gray-300">Порт приложения</label>
            <input v-model="config.port" type="text" id="port"
                   class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors bg-gray-700 border-gray-600 text-white focus:ring-indigo-500"
                   placeholder="8080" />
          </div>
          <div class="md:col-span-2">
            <label for="web_password" class="block text-sm font-medium mb-1 text-gray-300">Пароль на веб-интерфейс</label>
            <input v-model="config.web_password" type="password" id="web_password"
                   class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors bg-gray-700 border-gray-600 text-white focus:ring-indigo-500"
                   placeholder="Оставьте пустым для отключения пароля" />
            <p class="text-xs text-gray-400 mt-1">Если установлен, потребуется для доступа к веб-интерфейсу</p>
          </div>

          <!-- НАСТРОЙКИ ЧИТАЛКИ -->
          <div class="md:col-span-2 p-4 border border-gray-600 rounded-lg">
            <h3 class="text-lg font-semibold text-white mb-4">Настройки читалки</h3>

            <div class="mb-4">
              <div class="flex items-center mb-2">
                <input v-model="config.reader_enabled" type="checkbox" id="reader_enabled"
                       class="h-4 w-4 rounded border-gray-600 bg-gray-700 text-indigo-600 focus:ring-indigo-500 focus:ring-offset-gray-800" />
                <label for="reader_enabled" class="ml-2 text-sm font-medium text-gray-300">
                  Включить кнопку "Читать"
                </label>
              </div>
              <p class="text-xs text-gray-400 ml-6">Если включено, кнопка "Читать" будет отображаться в модальном окне книги</p>
            </div>

            <div>
              <label for="reader_url" class="block text-sm font-medium mb-1 text-gray-300">URL читалки</label>
              <input v-model="config.reader_url" type="text" id="reader_url"
                     class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors bg-gray-700 border-gray-600 text-white focus:ring-indigo-500"
                     placeholder="https://reader.example.com/#/read?url=" />
              <p class="text-xs text-gray-400 mt-1">
                Укажите полный URL читалки. Ссылка на скачивание книги будет автоматически подставлена в конец.<br>
                Пример: <code class="bg-gray-900 px-1 py-0.5 rounded">https://reader.example.com/#/read?url=</code>
              </p>
            </div>
            <div class="mt-4">
              <label for="default_search_language" class="block text-sm font-medium mb-1 text-gray-300">Язык книг при поиске по умолчанию</label>
              <select v-model="config.default_search_language" id="default_search_language"
                      class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors bg-gray-700 border-gray-600 text-white focus:ring-indigo-500">
                <option v-for="opt in defaultLanguageOptions" :key="opt.code" :value="opt.code">{{ opt.name }}</option>
              </select>
              <p class="text-xs text-gray-400 mt-1">Если задан, в форме поиска по умолчанию будет выбран этот язык. Пользователь может изменить его вручную.</p>
            </div>
          </div>
        </div>
        <div v-if="settingsError" class="text-red-400 text-sm">{{ settingsError }}</div>
        <div v-if="settingsSuccess" class="text-green-400 text-sm">{{ settingsSuccess }}</div>
        <div class="flex justify-end">
          <button type="submit"
                  class="inline-flex items-center gap-2 px-4 py-2.5 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white transition-colors">
            <Save class="h-4 w-4" /> Сохранить настройки
          </button>
        </div>
      </form>

      <!-- Статистика библиотеки -->
      <div v-if="libraryStats" class="mt-8 p-4 border border-gray-600 rounded-lg">
        <h3 class="text-lg font-semibold text-white mb-4 flex items-center gap-2">
          <BarChart3 class="h-5 w-5" /> Статистика библиотеки
        </h3>
        <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-6 gap-4 text-sm">
          <div class="p-3 rounded-lg bg-gray-700/50 break-words">
            <div class="text-gray-400 text-xs uppercase">Книг</div>
            <div class="text-white font-semibold mt-1">{{ libraryStats.total_books?.toLocaleString() ?? '—' }}</div>
          </div>
          <div class="p-3 rounded-lg bg-gray-700/50 break-words">
            <div class="text-gray-400 text-xs uppercase">Всего авторов (связей)</div>
            <div class="text-white font-semibold mt-1">{{ libraryStats.total_authors?.toLocaleString() ?? '—' }}</div>
          </div>
          <div class="p-3 rounded-lg bg-gray-700/50 break-words">
            <div class="text-gray-400 text-xs uppercase">Уникальных авторов</div>
            <div class="text-white font-semibold mt-1">{{ libraryStats.unique_authors?.toLocaleString() ?? '—' }}</div>
          </div>
          <div class="p-3 rounded-lg bg-gray-700/50 break-words">
            <div class="text-gray-400 text-xs uppercase">Жанров</div>
            <div class="text-white font-semibold mt-1">{{ libraryStats.genres_count?.toLocaleString() ?? '—' }}</div>
          </div>
          <div class="p-3 rounded-lg bg-gray-700/50 break-words">
            <div class="text-gray-400 text-xs uppercase">Языков</div>
            <div class="text-white font-semibold mt-1">{{ libraryStats.languages_count?.toLocaleString() ?? '—' }}</div>
          </div>
          <div class="p-3 rounded-lg bg-gray-700/50 col-span-2 sm:col-span-1 break-words">
            <div class="text-gray-400 text-xs uppercase">Последнее добавление</div>
            <div class="text-white font-semibold mt-1">{{ libraryStats.last_added_at ? new Date(libraryStats.last_added_at).toLocaleString() : '—' }}</div>
          </div>
        </div>
      </div>

      <!-- Управление пользователями -->
      <div class="mt-8 p-4 border border-gray-600 rounded-lg">
        <h3 class="text-lg font-semibold text-white mb-4">Пользователи</h3>
        <div class="space-y-4">
          <div class="flex gap-2 flex-wrap items-end">
            <div>
              <label class="block text-xs text-gray-400 mb-1">Имя</label>
              <input v-model="newUser.username" type="text" placeholder="Новый пользователь"
                     class="px-3 py-2 rounded-lg bg-gray-700 border-gray-600 text-white w-40" />
            </div>
            <div>
              <label class="block text-xs text-gray-400 mb-1">Пароль</label>
              <input v-model="newUser.password" type="password" placeholder="Пароль"
                     class="px-3 py-2 rounded-lg bg-gray-700 border-gray-600 text-white w-32" />
            </div>
            <button @click="createUser" class="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white flex items-center gap-1">
              <UserPlus class="h-4 w-4" /> Добавить
            </button>
          </div>
          <div v-if="userCreateError" class="text-red-400 text-sm">{{ userCreateError }}</div>
          <div v-if="userCreateSuccess" class="text-green-400 text-sm">{{ userCreateSuccess }}</div>
          <div v-if="userDeleteError" class="text-red-400 text-sm">{{ userDeleteError }}</div>
          <div v-if="userDeleteSuccess" class="text-green-400 text-sm">{{ userDeleteSuccess }}</div>

          <!-- Таблица пользователей -->
          <div class="border border-gray-600 rounded-lg overflow-x-auto">
            <table class="w-full text-sm">
              <thead class="bg-gray-700 text-gray-300">
                <tr>
                  <th class="text-left p-3">ID</th>
                  <th class="text-left p-3">Пользователь</th>
                  <th class="text-left p-3">Роль</th>
                  <th class="text-left p-3">Создан</th>
                  <th class="p-3">Действия</th>
                </tr>
              </thead>
              <tbody class="text-white">
                <tr v-for="u in users" :key="u.id" class="border-t border-gray-600 hover:bg-gray-700/50">
                  <td class="p-3">{{ u.id }}</td>
                  <td class="p-3">{{ u.username }}</td>
                  <td class="p-3">
                    <span :class="u.role === 'admin' ? 'text-yellow-400 font-medium' : 'text-gray-300'">
                      {{ u.role === 'admin' ? 'Администратор' : 'Пользователь' }}
                    </span>
                  </td>
                  <td class="p-3 text-gray-400">{{ u.created_at ? new Date(u.created_at).toLocaleDateString() : 'N/A' }}</td>
                  <td class="p-3 space-x-2">
                    <button v-if="u.role !== 'admin'"
                            @click="resetUser = u; resetPassword = ''"
                            class="text-indigo-400 hover:text-indigo-300 text-xs px-2 py-1 hover:bg-gray-800 rounded">
                      Сбросить пароль
                    </button>
                    <button v-if="u.role !== 'admin'"
                            @click="deleteUser(u)"
                            class="text-red-400 hover:text-red-300 text-xs px-2 py-1 hover:bg-gray-800 rounded">
                      Удалить
                    </button>
                    <span v-else class="text-gray-500 text-xs">Системный</span>
                  </td>
                </tr>
                <tr v-if="users.length === 0" class="border-t border-gray-600">
                  <td colspan="5" class="p-4 text-center text-gray-500">
                    Пользователи не найдены
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- Форма сброса пароля -->
          <div v-if="resetUser" class="p-3 bg-gray-700/50 rounded-lg flex gap-2 items-center flex-wrap">
            <span class="text-gray-300">Новый пароль для {{ resetUser.username }}:</span>
            <input v-model="resetPassword" type="password" placeholder="Пароль"
                   class="px-3 py-2 rounded-lg bg-gray-700 border-gray-600 text-white w-40" />
            <button @click="doResetPassword" class="px-3 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white text-sm">
              Сбросить
            </button>
            <button @click="resetUser = null" class="px-3 py-2 rounded-lg bg-gray-600 hover:bg-gray-500 text-white text-sm">
              Отмена
            </button>
            <span v-if="resetError" class="text-red-400 text-sm">{{ resetError }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
