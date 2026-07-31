<script setup>
import { BookOpen, Heart } from 'lucide-vue-next';
import { getCoverUrl, getGenreNames, getLanguageName } from '../utils/helpers.js';

defineProps({
  theme: { type: String, required: true },
  favoritesBooks: { type: Array, required: true },
  favoriteIds: { type: Object, required: true },
  shouldLoadImage: { type: Function, required: true },
  hasImageError: { type: Function, required: true },
  getImageId: { type: Function, required: true }
});

defineEmits(['close', 'select-book', 'toggle-favorite', 'image-error', 'image-load']);
</script>

<template>
  <div
    class="min-h-screen p-4 overflow-x-hidden"
    :class="theme === 'dark' ? 'bg-gray-900' : 'bg-gradient-to-br from-slate-100 to-slate-200'"
  >
    <header class="max-w-7xl mx-auto flex items-center justify-between mb-6">
      <h2 class="text-xl font-bold break-words" :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">Книжная полка</h2>
      <button
        type="button"
        class="px-4 py-2 rounded-lg font-medium transition-colors"
        :class="theme === 'dark' ? 'bg-gray-700 hover:bg-gray-600 text-white' : 'bg-slate-200 hover:bg-slate-300 text-slate-800'"
        @click="$emit('close')"
      >
        Назад
      </button>
    </header>
    <div class="max-w-7xl mx-auto">
      <div v-if="favoritesBooks.length === 0" class="text-center py-12">
        <Heart class="mx-auto h-12 w-12 mb-3" :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-400'" />
        <p :class="theme === 'dark' ? 'text-gray-400' : 'text-slate-600'">В избранном пока ничего нет</p>
        <p class="text-sm mt-1" :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'">Нажимайте F на книге или значок ♥</p>
      </div>
      <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-4">
        <div
          v-for="book in favoritesBooks"
          :key="book.id"
          class="group cursor-pointer rounded-xl transition-all duration-200 hover:scale-[1.02] flex flex-col border"
          :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 shadow-md hover:shadow-lg' : 'bg-slate-100 border-slate-300 shadow hover:shadow-md'"
          style="height: 260px"
          @click="$emit('select-book', book)"
        >
          <div
            class="h-32 w-full overflow-hidden rounded-t-xl relative book-cover-placeholder"
            :data-img-id="getImageId(book)"
          >
            <div
              v-if="!shouldLoadImage(book) || hasImageError(book) || !getCoverUrl(book)"
              class="w-full h-full flex items-center justify-center rounded-t-xl"
              :class="theme === 'dark' ? 'bg-gray-700' : 'bg-slate-200'"
            >
              <BookOpen class="h-6 w-6" :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'" />
            </div>
            <img
              v-else
              :src="getCoverUrl(book)"
              :alt="book.title"
              class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-200"
              loading="lazy"
              @error="$emit('image-error', $event, book)"
              @load="$emit('image-load', $event, book)"
            />
            <button
              type="button"
              class="absolute top-2 right-2 p-1.5 rounded-full transition-colors"
              :class="favoriteIds.has(book.id) ? 'text-red-500 bg-white/90' : 'text-gray-400 bg-white/70 hover:bg-white/90'"
              @click.stop="$emit('toggle-favorite', book)"
            >
              <Heart class="h-4 w-4" :fill="favoriteIds.has(book.id) ? 'currentColor' : 'none'" />
            </button>
          </div>
          <div class="p-3 flex flex-col flex-grow">
            <h3 class="font-semibold text-sm mb-1 line-clamp-2 leading-tight break-words" :class="theme === 'dark' ? 'text-gray-100' : 'text-slate-800'">
              {{ book.title }}
            </h3>
            <p class="text-xs mb-2 line-clamp-1 break-words" :class="theme === 'dark' ? 'text-gray-400' : 'text-slate-600'">
              {{ getGenreNames(book.genre) }}
            </p>
            <div class="mt-auto space-y-1">
              <div class="flex justify-between text-xs">
                <span :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'">Формат:</span>
                <span class="font-medium uppercase" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">{{ book.format }}</span>
              </div>
              <div class="flex justify-between text-xs">
                <span :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'">Язык:</span>
                <span class="font-medium" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">{{ getLanguageName(book.language) }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
