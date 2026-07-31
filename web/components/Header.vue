<script setup>
import {
  BookOpen,
  Sun,
  Moon,
  Settings,
  User,
  LogOut,
  Heart,
  Keyboard
} from 'lucide-vue-next';

defineProps({
  theme: { type: String, required: true },
  currentUser: { type: Object, default: null },
  showSettingsButton: { type: Boolean, default: false },
  userInitial: { type: String, default: '?' },
  showUserMenu: { type: Boolean, default: false }
});

defineEmits([
  'toggle-theme',
  'open-settings',
  'open-favorites',
  'toggle-hotkeys',
  'open-auth',
  'open-profile',
  'logout',
  'toggle-user-menu',
  'close-user-menu',
  'open-profile-from-menu',
  'open-settings-from-menu',
  'open-favorites-from-menu',
  'logout-from-menu'
]);
</script>

<template>
  <header
    class="sticky top-0 z-40 backdrop-blur-md border-b transition-colors"
    :class="theme === 'dark' ? 'bg-gray-800/80 border-gray-700' : 'bg-slate-100/80 border-slate-300'"
  >
    <div class="max-w-7xl mx-auto px-4 py-3 flex items-center justify-between">
      <div class="flex items-center space-x-2">
        <div class="p-2 rounded-lg" :class="theme === 'dark' ? 'bg-indigo-600' : 'bg-slate-300'">
          <BookOpen class="h-5 w-5" :class="theme === 'dark' ? 'text-white' : 'text-slate-700'" />
        </div>
        <h1 class="text-xl font-bold bg-gradient-to-r from-indigo-600 to-purple-600 bg-clip-text text-transparent">
          Моя Библиотека
        </h1>
      </div>
      <div class="flex items-center space-x-2">
        <button
          type="button"
          class="p-2 rounded-md transition-colors"
          :class="theme === 'dark' ? 'bg-gray-700 text-yellow-400 hover:bg-gray-600' : 'bg-slate-300 text-slate-700 hover:bg-slate-400'"
          @click="$emit('toggle-theme')"
        >
          <Sun v-if="theme === 'dark'" class="h-4 w-4" />
          <Moon v-else class="h-4 w-4" />
        </button>
        <button
          v-if="showSettingsButton"
          type="button"
          class="hidden sm:flex p-2 rounded-md transition-colors"
          :class="theme === 'dark' ? 'text-gray-300 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-300'"
          @click="$emit('open-settings')"
        >
          <Settings class="h-5 w-5" />
        </button>
        <button
          v-if="currentUser"
          type="button"
          class="flex items-center gap-1.5 px-2 py-2 rounded-md text-sm transition-colors"
          :class="theme === 'dark' ? 'text-gray-300 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-300'"
          @click="$emit('open-favorites')"
        >
          <Heart class="h-4 w-4" />
          <span class="hidden sm:inline">Книжная полка</span>
        </button>
        <button
          type="button"
          class="hidden sm:flex p-2 rounded-md transition-colors"
          :class="theme === 'dark' ? 'text-gray-400 hover:bg-gray-700' : 'text-slate-500 hover:bg-slate-300'"
          @click="$emit('toggle-hotkeys')"
        >
          <Keyboard class="h-5 w-5" />
        </button>
        <button
          v-if="!currentUser"
          type="button"
          class="p-2 rounded-md transition-colors"
          :class="theme === 'dark' ? 'text-gray-300 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-300'"
          @click="$emit('open-auth', 'login')"
        >
          <User class="h-5 w-5" />
        </button>
        <template v-else>
          <div class="hidden sm:flex items-center gap-1">
            <button
              type="button"
              class="px-3 py-2 rounded-md text-sm font-medium transition-colors"
              :class="theme === 'dark' ? 'text-gray-300 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-300'"
              @click="$emit('open-profile')"
            >
              {{ currentUser.username }}
            </button>
            <button
              type="button"
              class="p-2 rounded-md transition-colors"
              :class="theme === 'dark' ? 'text-red-400 hover:bg-gray-700' : 'text-red-600 hover:bg-slate-300'"
              @click="$emit('logout')"
            >
              <LogOut class="h-5 w-5" />
            </button>
          </div>
          <div class="relative sm:hidden">
            <button
              type="button"
              class="flex h-9 w-9 items-center justify-center rounded-full text-sm font-semibold transition-colors shrink-0 border"
              :class="theme === 'dark' ? 'bg-indigo-600 border-indigo-500 text-white hover:bg-indigo-500' : 'bg-indigo-600 border-indigo-500 text-white hover:bg-indigo-700'"
              :aria-expanded="showUserMenu"
              aria-haspopup="true"
              aria-label="Меню пользователя"
              @click="$emit('toggle-user-menu')"
            >
              <span class="select-none">{{ userInitial }}</span>
            </button>
            <Teleport to="body">
              <template v-if="showUserMenu">
                <div
                  class="fixed inset-0 z-[100] bg-black/30 sm:hidden"
                  aria-hidden="true"
                  @click="$emit('close-user-menu')"
                />
                <div
                  class="fixed z-[110] w-56 max-w-[calc(100vw-1.5rem)] rounded-lg border shadow-xl py-1 sm:hidden right-3 top-16"
                  :class="theme === 'dark' ? 'bg-gray-800 border-gray-700' : 'bg-white border-slate-200'"
                  role="menu"
                  @click.stop
                >
                  <div
                    class="px-3 py-2 border-b text-xs truncate"
                    :class="theme === 'dark' ? 'border-gray-700 text-gray-400' : 'border-slate-200 text-slate-500'"
                  >
                    {{ currentUser.username }}
                  </div>
                  <button
                    type="button"
                    role="menuitem"
                    class="w-full text-left px-3 py-2.5 text-sm transition-colors flex items-center gap-2"
                    :class="theme === 'dark' ? 'text-gray-200 hover:bg-gray-700' : 'text-slate-800 hover:bg-slate-100'"
                    @click="$emit('open-profile-from-menu')"
                  >
                    <User class="h-4 w-4 shrink-0" /> Профиль и пароль
                  </button>
                  <button
                    v-if="showSettingsButton"
                    type="button"
                    role="menuitem"
                    class="w-full text-left px-3 py-2.5 text-sm transition-colors flex items-center gap-2"
                    :class="theme === 'dark' ? 'text-gray-200 hover:bg-gray-700' : 'text-slate-800 hover:bg-slate-100'"
                    @click="$emit('open-settings-from-menu')"
                  >
                    <Settings class="h-4 w-4 shrink-0" /> Настройки сервера
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    class="w-full text-left px-3 py-2.5 text-sm transition-colors flex items-center gap-2"
                    :class="theme === 'dark' ? 'text-gray-200 hover:bg-gray-700' : 'text-slate-800 hover:bg-slate-100'"
                    @click="$emit('open-favorites-from-menu')"
                  >
                    <Heart class="h-4 w-4 shrink-0" /> Книжная полка
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    class="w-full text-left px-3 py-2.5 text-sm transition-colors flex items-center gap-2"
                    :class="theme === 'dark' ? 'text-red-400 hover:bg-gray-700' : 'text-red-600 hover:bg-slate-100'"
                    @click="$emit('logout-from-menu')"
                  >
                    <LogOut class="h-4 w-4 shrink-0" /> Выйти
                  </button>
                </div>
              </template>
            </Teleport>
          </div>
        </template>
      </div>
    </div>
  </header>
</template>
