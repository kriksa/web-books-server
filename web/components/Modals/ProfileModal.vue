<script setup>
defineProps({
  theme: { type: String, required: true },
  profileForm: { type: Object, required: true },
  profileError: { type: String, default: '' },
  profileSuccess: { type: String, default: '' }
});

defineEmits(['submit', 'close', 'update:profileForm']);
</script>

<template>
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm"
    @click="$emit('close')"
  >
    <div
      class="relative w-full max-w-md rounded-xl shadow-2xl border p-6"
      :class="theme === 'dark' ? 'bg-gray-800 border-gray-700' : 'bg-slate-100 border-slate-300'"
      @click.stop
    >
      <h2 class="text-xl font-bold mb-4" :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">Мой профиль</h2>
      <form class="space-y-4" @submit.prevent="$emit('submit')">
        <div>
          <label class="block text-sm font-medium mb-1" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Имя пользователя</label>
          <input
            :value="profileForm.newUsername"
            type="text"
            class="w-full px-3 py-2.5 rounded-lg border"
            :class="theme === 'dark' ? 'bg-gray-700 border-gray-600 text-white' : 'bg-slate-100 border-slate-300 text-slate-800'"
            @input="$emit('update:profileForm', { ...profileForm, newUsername: $event.target.value })"
          />
        </div>
        <div>
          <label class="block text-sm font-medium mb-1" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Новый пароль (оставьте пустым, если не меняете)</label>
          <input
            :value="profileForm.newPassword"
            type="password"
            class="w-full px-3 py-2.5 rounded-lg border"
            :class="theme === 'dark' ? 'bg-gray-700 border-gray-600 text-white' : 'bg-slate-100 border-slate-300 text-slate-800'"
            @input="$emit('update:profileForm', { ...profileForm, newPassword: $event.target.value })"
          />
        </div>
        <div v-if="profileForm.newPassword">
          <label class="block text-sm font-medium mb-1" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Текущий пароль</label>
          <input
            :value="profileForm.oldPassword"
            type="password"
            class="w-full px-3 py-2.5 rounded-lg border"
            :class="theme === 'dark' ? 'bg-gray-700 border-gray-600 text-white' : 'bg-slate-100 border-slate-300 text-slate-800'"
            @input="$emit('update:profileForm', { ...profileForm, oldPassword: $event.target.value })"
          />
        </div>
        <div v-if="profileForm.newPassword">
          <label class="block text-sm font-medium mb-1" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Подтверждение пароля</label>
          <input
            :value="profileForm.confirmPassword"
            type="password"
            class="w-full px-3 py-2.5 rounded-lg border"
            :class="theme === 'dark' ? 'bg-gray-700 border-gray-600 text-white' : 'bg-slate-100 border-slate-300 text-slate-800'"
            @input="$emit('update:profileForm', { ...profileForm, confirmPassword: $event.target.value })"
          />
        </div>
        <div v-if="profileError" class="text-red-500 text-sm">{{ profileError }}</div>
        <div v-if="profileSuccess" class="text-green-500 text-sm">{{ profileSuccess }}</div>
        <div class="flex gap-2">
          <button type="submit" class="px-4 py-2.5 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white">
            Сохранить
          </button>
          <button
            type="button"
            class="px-4 py-2.5 rounded-lg font-medium border"
            :class="theme === 'dark' ? 'border-gray-600 text-gray-300 hover:bg-gray-700' : 'border-slate-300 text-slate-700 hover:bg-slate-200'"
            @click="$emit('close')"
          >
            Закрыть
          </button>
        </div>
      </form>
    </div>
  </div>
</template>
