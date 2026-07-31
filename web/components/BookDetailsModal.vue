<script setup>
import { BookOpen, Download, Heart, X } from 'lucide-vue-next';
import { getCoverUrl, getGenreNames, getLanguageName, formatFileSize } from '../utils/helpers.js';

defineProps({
  theme: { type: String, required: true },
  book: { type: Object, required: true },
  currentUser: { type: Object, default: null },
  favoriteIds: { type: Object, required: true },
  readerEnabled: { type: Boolean, default: false },
  bookDetails: { type: Object, default: null },
  loadingDetails: { type: Boolean, default: false },
  hasImageError: { type: Function, required: true }
});

defineEmits(['close', 'toggle-favorite', 'image-error', 'image-load', 'read']);
</script>

<template>
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-2 md:p-4 bg-black/60 backdrop-blur-sm overflow-hidden"
    @click="$emit('close')"
  >
    <div
      class="relative w-full max-w-4xl max-h-[90vh] overflow-hidden rounded-xl shadow-2xl border flex flex-col"
      :class="theme === 'dark' ? 'bg-gray-800 border-gray-700' : 'bg-slate-50 border-slate-300'"
      @click.stop
    >
      <div
        class="flex justify-between items-start p-3 sm:p-4 border-b"
        :class="theme === 'dark' ? 'border-gray-700 bg-gray-800' : 'border-slate-200 bg-white'"
      >
        <div class="pr-4">
          <h2 class="text-lg sm:text-xl font-bold leading-tight break-words" :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">
            {{ book.title }}
          </h2>
          <p class="text-sm mt-1 break-words" :class="theme === 'dark' ? 'text-indigo-400' : 'text-indigo-600'">
            {{ book.author }}
          </p>
        </div>
        <div class="flex items-center gap-1">
          <button
            v-if="currentUser"
            type="button"
            class="p-1.5 sm:p-2 rounded-full transition-colors"
            :class="favoriteIds.has(book.id) ? 'text-red-500' : (theme === 'dark' ? 'text-gray-400 hover:bg-gray-700' : 'text-slate-500 hover:bg-slate-200')"
            @click="$emit('toggle-favorite', book)"
          >
            <Heart class="h-4 sm:h-5 w-4 sm:w-5" :fill="favoriteIds.has(book.id) ? 'currentColor' : 'none'" />
          </button>
          <button
            type="button"
            class="p-1.5 sm:p-2 rounded-full transition-colors flex-shrink-0"
            :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'"
            @click="$emit('close')"
          >
            <X class="h-5 sm:h-6 w-5 sm:w-6" />
          </button>
        </div>
      </div>
      <div class="overflow-y-auto overflow-x-hidden p-0 flex-grow">
        <div class="flex flex-col md:flex-row">
          <div
            class="md:w-1/3 p-3 sm:p-5 flex flex-col items-center border-b md:border-b-0 md:border-r"
            :class="theme === 'dark' ? 'border-gray-700 bg-gray-800/50' : 'border-slate-200 bg-white'"
          >
            <div class="w-full max-w-[200px] mb-4 shadow-lg rounded-lg overflow-hidden relative group">
              <img
                v-if="getCoverUrl(book) && !hasImageError(book)"
                :src="getCoverUrl(book)"
                :alt="book.title"
                class="w-full h-auto object-cover"
                @error="$emit('image-error', $event, book)"
                @load="$emit('image-load', $event, book)"
              />
              <div
                v-else
                class="w-full aspect-[2/3] flex items-center justify-center"
                :class="theme === 'dark' ? 'bg-gray-700' : 'bg-slate-200'"
              >
                <BookOpen class="h-12 w-12" :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'" />
              </div>
            </div>
            <a
              :href="`/download/${book.id}`"
              class="w-full flex items-center justify-center gap-2 py-3 px-4 rounded-lg font-bold shadow-lg transform transition hover:scale-105 mb-3"
              :class="theme === 'dark' ? 'bg-indigo-600 hover:bg-indigo-500 text-white' : 'bg-indigo-600 hover:bg-indigo-700 text-white'"
            >
              <Download class="h-5 w-5" /> Скачать ({{ book.format.toUpperCase() }})
            </a>
            <button
              v-if="readerEnabled"
              type="button"
              class="w-full flex items-center justify-center gap-2 py-2 px-4 rounded-lg font-bold shadow-md transform transition hover:scale-105 mb-6 border"
              :class="theme === 'dark' ? 'border-indigo-500 text-indigo-400 hover:bg-gray-700' : 'border-indigo-600 text-indigo-700 hover:bg-indigo-50'"
              @click="$emit('read')"
            >
              <BookOpen class="h-5 w-5" /> Читать
            </button>
            <div class="w-full space-y-3 text-sm" :class="theme === 'dark' ? 'text-gray-200' : 'text-slate-800'">
              <div class="flex justify-between border-b pb-2" :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-200'">
                <span class="opacity-70 flex-shrink-0">Серия:</span>
                <span class="font-medium text-right break-words ml-2">
                  {{ book.series || '—' }}
                  <span v-if="book.seriesNo" class="bg-indigo-100 text-indigo-800 text-xs px-1.5 py-0.5 rounded ml-1">#{{ book.seriesNo }}</span>
                </span>
              </div>
              <div class="flex justify-between border-b pb-2" :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-200'">
                <span class="opacity-70 flex-shrink-0">Жанр:</span>
                <span class="font-medium text-right break-words ml-2">{{ getGenreNames(book.genre) }}</span>
              </div>
              <div class="flex justify-between border-b pb-2" :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-200'">
                <span class="opacity-70 flex-shrink-0">Язык:</span>
                <span class="font-medium text-right break-words ml-2">{{ getLanguageName(book.language) }}</span>
              </div>
              <div class="flex justify-between border-b pb-2" :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-200'">
                <span class="opacity-70 flex-shrink-0">Размер:</span>
                <span class="font-medium text-right break-words ml-2">{{ formatFileSize(book.fileSize) }}</span>
              </div>
              <div class="flex justify-between">
                <span class="opacity-70 flex-shrink-0">Добавлено:</span>
                <span class="font-medium text-right break-words ml-2">{{ new Date(book.addedAt).toLocaleDateString() }}</span>
              </div>
            </div>
          </div>
          <div class="md:w-2/3 p-3 sm:p-5 md:p-6" :class="theme === 'dark' ? 'bg-gray-900/50' : 'bg-slate-50'">
            <div v-if="loadingDetails" class="flex flex-col items-center justify-center h-40 space-y-3">
              <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-500" />
              <span class="text-sm opacity-70">Распаковка описания...</span>
            </div>
            <div v-else>
              <div v-if="bookDetails && bookDetails.titleInfo && bookDetails.titleInfo.annotationHtml" class="mb-8">
                <h3 class="text-lg font-bold mb-3 flex items-center gap-2" :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">
                  <span class="w-1 h-6 bg-indigo-500 rounded-full" />
                  Аннотация
                </h3>
                <div
                  class="prose max-w-none text-sm leading-relaxed book-annotation"
                  :class="theme === 'dark' ? 'prose-invert text-gray-300' : 'text-slate-700'"
                  v-html="bookDetails.titleInfo.annotationHtml"
                />
              </div>
              <div
                v-else-if="(book.format === 'fb2' || book.format === 'epub') && !loadingDetails"
                class="mb-8 text-center py-4 opacity-50 italic"
              >
                Описание отсутствует
              </div>
              <div
                v-else-if="book.format !== 'fb2' && book.format !== 'epub'"
                class="mb-8 p-4 rounded-lg border border-dashed text-center text-sm opacity-70"
                :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-300'"
              >
                Детальное описание доступно только для FB2 и EPUB файлов
              </div>
              <div v-if="bookDetails" class="space-y-6">
                <div v-if="bookDetails.publishInfo && (bookDetails.publishInfo.publisher || bookDetails.publishInfo.year)">
                  <h3 class="text-md font-bold mb-2 opacity-80 uppercase text-xs tracking-wider">Информация об издании</h3>
                  <div
                    class="grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-2 text-sm rounded-lg p-3"
                    :class="theme === 'dark' ? 'bg-gray-800' : 'bg-white border'"
                  >
                    <div v-if="bookDetails.publishInfo.publisher" class="break-words">
                      <span class="opacity-60 block text-xs">Издательство</span>
                      {{ bookDetails.publishInfo.publisher }}
                    </div>
                    <div v-if="bookDetails.publishInfo.city" class="break-words">
                      <span class="opacity-60 block text-xs">Город</span>
                      {{ bookDetails.publishInfo.city }}
                    </div>
                    <div v-if="bookDetails.publishInfo.year">
                      <span class="opacity-60 block text-xs">Год</span>
                      {{ bookDetails.publishInfo.year }}
                    </div>
                    <div v-if="bookDetails.publishInfo.isbn" class="break-words">
                      <span class="opacity-60 block text-xs">ISBN</span>
                      {{ bookDetails.publishInfo.isbn }}
                    </div>
                  </div>
                </div>
                <div v-if="bookDetails.titleInfo && bookDetails.titleInfo.translator && bookDetails.titleInfo.translator.length">
                  <h3 class="text-md font-bold mb-2 opacity-80 uppercase text-xs tracking-wider">Перевод</h3>
                  <div class="text-sm rounded-lg p-3" :class="theme === 'dark' ? 'bg-gray-800' : 'bg-white border'">
                    <div class="mb-2 break-words">
                      <span class="opacity-60 block text-xs">Переводчики</span>
                      {{ bookDetails.titleInfo.translator.join(', ') }}
                    </div>
                    <div v-if="bookDetails.srcTitleInfo && bookDetails.srcTitleInfo.bookTitle" class="break-words">
                      <span class="opacity-60 block text-xs">Оригинальное название</span>
                      {{ bookDetails.srcTitleInfo.bookTitle }}
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.book-annotation {
  overflow-wrap: break-word;
  hyphens: auto;
}
.book-annotation :deep(p) {
  margin-bottom: 0.75em;
  text-align: justify;
}
.book-annotation :deep(strong),
.book-annotation :deep(b) {
  font-weight: 700;
  color: inherit;
}
.book-annotation :deep(i),
.book-annotation :deep(em) {
  font-style: italic;
}
</style>
