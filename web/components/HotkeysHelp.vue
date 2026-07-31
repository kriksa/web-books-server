<script setup>
import { X } from 'lucide-vue-next';

defineProps({
  theme: { type: String, required: true }
});

defineEmits(['close']);
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-[70] flex items-start justify-end p-4 pt-20" @click.self="$emit('close')">
      <div
        class="rounded-xl shadow-xl border p-4 max-w-xs text-sm"
        :class="theme === 'dark' ? 'bg-gray-800 border-gray-600' : 'bg-white border-slate-200'"
      >
        <div class="flex justify-between items-center mb-3">
          <span class="font-semibold" :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">Горячие клавиши</span>
          <button
            type="button"
            class="p-1 rounded"
            :class="theme === 'dark' ? 'hover:bg-gray-700' : 'hover:bg-slate-100'"
            @click="$emit('close')"
          >
            <X class="h-4 w-4" />
          </button>
        </div>
        <ul class="space-y-2" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-600'">
          <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">/</kbd> — фокус на поиск</li>
          <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">Esc</kbd> — закрыть модалку</li>
          <li>
            <kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">←</kbd><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs ml-0.5">→</kbd> — листать книги
          </li>
          <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">F</kbd> — в избранное</li>
          <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">D</kbd> — скачать</li>
          <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">R</kbd> — открыть читалку</li>
          <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">Space</kbd> — быстрый просмотр</li>
        </ul>
      </div>
    </div>
  </Teleport>
</template>
