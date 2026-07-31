<script setup>
import { X } from 'lucide-vue-next';

defineProps({
  theme: { type: String, required: true },
  authForm: { type: Object, required: true },
  authError: { type: String, default: '' }
});

defineEmits(['close', 'submit', 'update:authForm']);
</script>

<template>
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm"
    @click="$emit('close')"
  >
    <div
      class="relative w-full max-w-md rounded-xl shadow-2xl border"
      :class="theme === 'dark' ? 'bg-gray-800 border-gray-700' : 'bg-slate-100 border-slate-300'"
      @click.stop
    >
      <button
        type="button"
        class="absolute top-3 right-3 p-1.5 rounded-full transition-colors"
        :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-300'"
        @click="$emit('close')"
      >
        <X class="h-5 w-5" />
      </button>
      <div class="p-6">
        <h2 class="text-xl font-bold mb-4 text-center" :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">
          Вход
        </h2>
        <form class="space-y-4" @submit.prevent="$emit('submit')">
          <div>
            <label
              for="auth-username"
              class="block text-sm font-medium mb-1"
              :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'"
            >Имя пользователя</label>
            <input
              id="auth-username"
              :value="authForm.username"
              type="text"
              required
              class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
              :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'"
              @input="$emit('update:authForm', { ...authForm, username: $event.target.value })"
            />
          </div>
          <div>
            <label
              for="auth-password"
              class="block text-sm font-medium mb-1"
              :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'"
            >Пароль</label>
            <input
              id="auth-password"
              :value="authForm.password"
              type="password"
              required
              class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
              :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'"
              @input="$emit('update:authForm', { ...authForm, password: $event.target.value })"
            />
          </div>
          <div v-if="authError" class="text-red-500 text-sm text-center">
            {{ authError }}
          </div>
          <button type="submit" class="w-full py-2.5 px-4 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white transition-colors">
            Войти
          </button>
        </form>
      </div>
    </div>
  </div>
</template>
