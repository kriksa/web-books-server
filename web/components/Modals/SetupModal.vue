<script setup>
defineProps({
  authForm: { type: Object, required: true },
  authError: { type: String, default: '' }
});

defineEmits(['submit', 'update:authForm']);
</script>

<template>
  <div class="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
    <div class="relative w-full max-w-md rounded-xl shadow-2xl border bg-gray-800 border-gray-700 p-6" @click.stop>
      <h2 class="text-xl font-bold mb-4 text-center text-white">Первоначальная настройка</h2>
      <p class="text-gray-300 text-sm mb-4 text-center">Создайте учётную запись администратора</p>
      <form class="space-y-4" @submit.prevent="$emit('submit')">
        <div>
          <label class="block text-sm font-medium mb-1 text-gray-300">Имя пользователя</label>
          <input
            :value="authForm.username"
            type="text"
            required
            class="w-full px-3 py-2.5 rounded-lg border bg-gray-700 border-gray-600 text-white focus:ring-indigo-500"
            @input="$emit('update:authForm', { ...authForm, username: $event.target.value })"
          />
        </div>
        <div>
          <label class="block text-sm font-medium mb-1 text-gray-300">Пароль (мин. 3 символа)</label>
          <input
            :value="authForm.password"
            type="password"
            required
            class="w-full px-3 py-2.5 rounded-lg border bg-gray-700 border-gray-600 text-white focus:ring-indigo-500"
            @input="$emit('update:authForm', { ...authForm, password: $event.target.value })"
          />
        </div>
        <div v-if="authError" class="text-red-400 text-sm text-center">{{ authError }}</div>
        <button type="submit" class="w-full py-2.5 px-4 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white">
          Создать администратора
        </button>
      </form>
    </div>
  </div>
</template>
