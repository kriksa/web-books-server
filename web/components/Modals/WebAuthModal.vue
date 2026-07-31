<script setup>
defineProps({
  webAuthPassword: { type: String, default: '' },
  webAuthError: { type: String, default: '' }
});

defineEmits(['submit', 'update:webAuthPassword']);
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
    <div class="relative w-full max-w-md rounded-xl shadow-2xl border bg-gray-800 border-gray-700" @click.stop>
      <div class="p-6">
        <h2 class="text-xl font-bold mb-4 text-center text-white">
          Вход в систему
        </h2>
        <form class="space-y-4" @submit.prevent="$emit('submit')">
          <div>
            <label for="web-auth-password" class="block text-sm font-medium mb-1 text-gray-300">Пароль</label>
            <input
              id="web-auth-password"
              :value="webAuthPassword"
              type="password"
              required
              class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors bg-gray-700 border-gray-600 text-white focus:ring-indigo-500"
              placeholder="Введите пароль для доступа"
              @input="$emit('update:webAuthPassword', $event.target.value)"
            />
          </div>
          <div v-if="webAuthError" class="text-red-400 text-sm text-center">
            {{ webAuthError }}
          </div>
          <button type="submit" class="w-full py-2.5 px-4 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white transition-colors">
            Войти
          </button>
        </form>
      </div>
    </div>
  </div>
</template>
