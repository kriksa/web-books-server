<script setup>
import { BookOpen, ChevronRight, ChevronDown } from 'lucide-vue-next';
import BookCard from './BookCard.vue';
import BookListItem from './BookListItem.vue';

defineProps({
  theme: { type: String, required: true },
  viewMode: { type: String, default: 'grid' },
  loading: { type: Boolean, default: false },
  filteredBooksCount: { type: Number, required: true },
  groupedBooks: { type: Array, required: true },
  expandedAuthors: { type: Object, required: true },
  expandedSeries: { type: Object, required: true },
  currentUser: { type: Object, default: null },
  favoriteIds: { type: Object, required: true },
  shouldLoadImage: { type: Function, required: true },
  hasImageError: { type: Function, required: true },
  getImageId: { type: Function, required: true }
});

defineEmits([
  'toggle-author',
  'toggle-series',
  'select-book',
  'toggle-favorite',
  'image-error',
  'image-load'
]);

const booksWord = (n) =>
  n === 1 ? 'книга' : n % 10 > 1 && n % 10 < 5 && (n < 10 || n > 20) ? 'книги' : 'книг';
</script>

<template>
  <main class="px-4 pb-24">
    <div class="max-w-7xl mx-auto">
      <div class="flex items-center justify-between mb-4">
        <h2 class="text-lg font-semibold" :class="theme === 'dark' ? 'text-gray-200' : 'text-slate-700'">
          Найдено: {{ filteredBooksCount }}
          {{ booksWord(filteredBooksCount) }}
        </h2>
      </div>

      <div v-if="loading" class="text-center py-12">
        <div class="inline-flex flex-col items-center gap-3">
          <div
            class="animate-spin rounded-full h-8 w-8 border-2"
            :class="theme === 'dark' ? 'border-indigo-500 border-t-transparent' : 'border-indigo-600 border-t-transparent'"
          />
          <span :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-600'">Поиск книг...</span>
        </div>
      </div>

      <div v-else-if="filteredBooksCount === 0" class="text-center py-12">
        <BookOpen class="mx-auto h-12 w-12 mb-3" :class="theme === 'dark' ? 'text-gray-600' : 'text-slate-500'" />
        <p :class="theme === 'dark' ? 'text-gray-400' : 'text-slate-600'">Книги не найдены</p>
      </div>

      <div v-else class="space-y-4">
        <div
          v-for="authorGroup in groupedBooks"
          :key="authorGroup.author"
          class="rounded-xl border overflow-hidden transition-all duration-300"
          :class="theme === 'dark' ? 'bg-gray-800/50 border-gray-700' : 'bg-white border-slate-200'"
        >
          <button
            type="button"
            class="w-full flex items-center justify-between p-4 text-left transition-colors hover:bg-opacity-80"
            :class="theme === 'dark' ? 'hover:bg-gray-700' : 'hover:bg-slate-50'"
            @click="$emit('toggle-author', authorGroup.author)"
          >
            <div class="flex items-center gap-3 w-full">
              <component
                :is="expandedAuthors.has(authorGroup.author) ? ChevronDown : ChevronRight"
                class="h-5 w-5 flex-shrink-0"
                :class="theme === 'dark' ? 'text-indigo-400' : 'text-indigo-600'"
              />
              <div class="flex-grow min-w-0">
                <h3 class="font-bold text-lg break-words" :class="theme === 'dark' ? 'text-gray-100' : 'text-slate-800'">
                  {{ authorGroup.author }}
                </h3>
                <span
                  class="text-xs font-medium px-2 py-0.5 rounded-full"
                  :class="theme === 'dark' ? 'bg-gray-700 text-gray-300' : 'bg-slate-100 text-slate-600'"
                >
                  Книг: {{ authorGroup.seriesGroups.reduce((acc, sg) => acc + sg.books.length, 0) }}
                </span>
              </div>
            </div>
          </button>

          <div
            v-show="expandedAuthors.has(authorGroup.author)"
            class="p-0 border-t transition-all duration-300"
            :class="theme === 'dark' ? 'border-gray-700 bg-gray-900/30' : 'border-slate-100 bg-slate-50/50'"
          >
            <div
              v-for="seriesGroup in authorGroup.seriesGroups"
              :key="`${authorGroup.author}_${seriesGroup.series}`"
              class="border-b last:border-b-0"
              :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-100'"
            >
              <button
                type="button"
                class="w-full flex items-center justify-between p-4 pl-8 text-left transition-colors hover:bg-opacity-80"
                :class="theme === 'dark' ? 'hover:bg-gray-700' : 'hover:bg-slate-50'"
                @click="$emit('toggle-series', authorGroup.author, seriesGroup.series)"
              >
                <div class="flex items-center gap-3 w-full">
                  <component
                    :is="expandedSeries.has(`${authorGroup.author}_${seriesGroup.series}`) ? ChevronDown : ChevronRight"
                    class="h-5 w-5 flex-shrink-0"
                    :class="theme === 'dark' ? 'text-indigo-400' : 'text-indigo-600'"
                  />
                  <div class="flex-grow min-w-0">
                    <h4 class="font-semibold break-words" :class="theme === 'dark' ? 'text-gray-200' : 'text-slate-700'">
                      {{ seriesGroup.series }}
                    </h4>
                    <span
                      class="text-xs font-medium px-2 py-0.5 rounded-full"
                      :class="theme === 'dark' ? 'bg-gray-600 text-gray-300' : 'bg-slate-200 text-slate-600'"
                    >
                      Книг: {{ seriesGroup.books.length }}
                    </span>
                  </div>
                </div>
              </button>

              <div v-show="expandedSeries.has(`${authorGroup.author}_${seriesGroup.series}`)" class="p-4 pl-12">
                <div
                  v-if="viewMode === 'grid'"
                  class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4"
                >
                  <BookCard
                    v-for="book in seriesGroup.books"
                    :key="book.id"
                    :theme="theme"
                    :book="book"
                    :current-user="currentUser"
                    :favorite-ids="favoriteIds"
                    :should-load-image="shouldLoadImage"
                    :has-image-error="hasImageError"
                    :get-image-id="getImageId"
                    @select="$emit('select-book', $event)"
                    @toggle-favorite="$emit('toggle-favorite', $event)"
                    @image-error="(e, b) => $emit('image-error', e, b)"
                    @image-load="(e, b) => $emit('image-load', e, b)"
                  />
                </div>
                <div v-else class="space-y-3">
                  <BookListItem
                    v-for="book in seriesGroup.books"
                    :key="book.id"
                    :theme="theme"
                    :book="book"
                    :current-user="currentUser"
                    :favorite-ids="favoriteIds"
                    :should-load-image="shouldLoadImage"
                    :has-image-error="hasImageError"
                    :get-image-id="getImageId"
                    @select="$emit('select-book', $event)"
                    @toggle-favorite="$emit('toggle-favorite', $event)"
                    @image-error="(e, b) => $emit('image-error', e, b)"
                    @image-load="(e, b) => $emit('image-load', e, b)"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>
