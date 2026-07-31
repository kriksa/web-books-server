<script setup>
import { BookOpen, Heart } from 'lucide-vue-next';
import { getCoverUrl, getGenreNames, getLanguageName } from '../utils/helpers.js';

defineProps({
  theme: { type: String, required: true },
  book: { type: Object, required: true },
  currentUser: { type: Object, default: null },
  favoriteIds: { type: Object, required: true },
  shouldLoadImage: { type: Function, required: true },
  hasImageError: { type: Function, required: true },
  getImageId: { type: Function, required: true }
});

defineEmits(['select', 'toggle-favorite', 'image-error', 'image-load']);
</script>

<template>
  <div
    class="group cursor-pointer rounded-xl p-4 transition-all duration-200 flex items-start gap-4 border"
    :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 hover:bg-gray-750 shadow' : 'bg-slate-100 border-slate-300 hover:bg-slate-200 shadow'"
    @click="$emit('select', book)"
  >
    <div
      class="flex-shrink-0 w-16 h-24 relative book-cover-placeholder"
      :data-img-id="getImageId(book)"
    >
      <div
        v-if="!shouldLoadImage(book) || hasImageError(book) || !getCoverUrl(book)"
        class="w-full h-full flex items-center justify-center rounded-lg"
        :class="theme === 'dark' ? 'bg-gray-700' : 'bg-slate-200'"
      >
        <BookOpen class="h-5 w-5" :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'" />
      </div>
      <img
        v-else
        :src="getCoverUrl(book)"
        :alt="book.title"
        class="w-full h-full object-cover rounded-lg"
        loading="lazy"
        @error="$emit('image-error', $event, book)"
        @load="$emit('image-load', $event, book)"
      />
      <button
        v-if="currentUser"
        type="button"
        class="absolute top-0.5 right-0.5 p-0.5 rounded"
        :class="favoriteIds.has(book.id) ? 'text-red-500' : (theme === 'dark' ? 'text-gray-400' : 'text-slate-500')"
        @click.stop="$emit('toggle-favorite', book)"
      >
        <Heart class="h-3.5 w-3.5" :fill="favoriteIds.has(book.id) ? 'currentColor' : 'none'" />
      </button>
    </div>
    <div class="flex-grow min-w-0">
      <h3 class="font-semibold text-base mb-1 line-clamp-2 break-words" :class="theme === 'dark' ? 'text-gray-100' : 'text-slate-800'">
        {{ book.title }}
      </h3>
      <div class="text-sm space-y-1" :class="theme === 'dark' ? 'text-gray-400' : 'text-slate-600'">
        <div v-if="book.series" class="break-words">
          <span class="font-medium">Серия:</span>
          {{ book.series }}{{ book.seriesNo > 0 ? ` #${book.seriesNo}` : '' }}
        </div>
        <div class="break-words">
          <span class="font-medium">Жанр:</span>
          {{ getGenreNames(book.genre) }}
        </div>
        <div>
          <span class="font-medium">Формат:</span>
          {{ book.format.toUpperCase() }}
        </div>
        <div>
          <span class="font-medium">Язык:</span>
          {{ getLanguageName(book.language) }}
        </div>
      </div>
    </div>
  </div>
</template>
