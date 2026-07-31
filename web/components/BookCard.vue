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
    class="group cursor-pointer rounded-xl transition-all duration-200 hover:scale-[1.02] flex flex-col border"
    :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 shadow-md hover:shadow-lg' : 'bg-slate-100 border-slate-300 shadow hover:shadow-md'"
    style="height: 260px"
    @click="$emit('select', book)"
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
        v-if="currentUser"
        type="button"
        class="absolute top-1.5 right-1.5 p-1 rounded-full transition-colors"
        :class="favoriteIds.has(book.id) ? 'text-red-500 bg-white/90' : 'text-gray-400 bg-white/70 hover:bg-white/90'"
        @click.stop="$emit('toggle-favorite', book)"
      >
        <Heart class="h-3.5 w-3.5" :fill="favoriteIds.has(book.id) ? 'currentColor' : 'none'" />
      </button>
    </div>
    <div class="p-3 flex flex-col flex-grow">
      <h3
        class="font-semibold text-sm mb-1 line-clamp-2 leading-tight break-words"
        :class="theme === 'dark' ? 'text-gray-100' : 'text-slate-800'"
      >
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
</template>
