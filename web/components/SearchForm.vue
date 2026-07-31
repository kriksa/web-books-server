<script setup>
import { ref } from 'vue';
import { Search, X } from 'lucide-vue-next';

const titleInputRef = ref(null);
defineExpose({
  focusSearch: () => titleInputRef.value?.focus()
});

defineProps({
  theme: { type: String, required: true },
  isSearchLocked: { type: Boolean, default: false },
  parseIsParsing: { type: Boolean, default: false },
  searchTitle: { type: String, default: '' },
  searchAuthor: { type: String, default: '' },
  searchGenre: { type: String, default: '' },
  searchSeries: { type: String, default: '' },
  selectedLanguage: { type: String, default: 'Все языки' },
  availableLanguages: { type: Array, default: () => [] },
  showGenreSuggestions: { type: Boolean, default: false },
  filteredGenreSuggestions: { type: Array, default: () => [] }
});

defineEmits([
  'submit',
  'update:searchTitle',
  'update:searchAuthor',
  'update:searchGenre',
  'update:searchSeries',
  'update:selectedLanguage',
  'language-manual',
  'update-genre-suggestions',
  'hide-genre-suggestions',
  'clear-field',
  'select-suggestion'
]);
</script>

<template>
  <section class="py-5 px-4">
    <div class="max-w-5xl mx-auto">
      <form
        class="space-y-4"
        :class="isSearchLocked ? 'opacity-60 pointer-events-none select-none' : ''"
        @submit.prevent="$emit('submit', $event)"
      >
        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div>
            <label
              for="title"
              class="block text-sm font-medium mb-1"
              :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'"
            >Название</label>
            <div class="relative">
              <input
                id="title"
                ref="titleInputRef"
                :value="searchTitle"
                :disabled="isSearchLocked"
                type="text"
                class="w-full px-3 py-2.5 pr-10 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'"
                @input="$emit('update:searchTitle', $event.target.value)"
              />
              <button
                v-if="searchTitle && !isSearchLocked"
                type="button"
                class="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded-md transition-colors"
                :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'"
                @click="$emit('clear-field', 'title')"
              >
                <X class="h-4 w-4" />
              </button>
            </div>
          </div>
          <div>
            <label
              for="author"
              class="block text-sm font-medium mb-1"
              :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'"
            >Автор</label>
            <div class="relative">
              <input
                id="author"
                :value="searchAuthor"
                :disabled="isSearchLocked"
                type="text"
                class="w-full px-3 py-2.5 pr-10 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'"
                @input="$emit('update:searchAuthor', $event.target.value)"
              />
              <button
                v-if="searchAuthor && !isSearchLocked"
                type="button"
                class="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded-md transition-colors"
                :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'"
                @click="$emit('clear-field', 'author')"
              >
                <X class="h-4 w-4" />
              </button>
            </div>
          </div>
          <div class="relative">
            <label
              for="genre"
              class="block text-sm font-medium mb-1"
              :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'"
            >Жанр</label>
            <input
              id="genre"
              :value="searchGenre"
              :disabled="isSearchLocked"
              type="text"
              class="w-full px-3 py-2.5 pr-10 rounded-lg border focus:outline-none focus:ring-2 transition-colors z-10 relative"
              :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'"
              @input="$emit('update:searchGenre', $event.target.value); $emit('update-genre-suggestions')"
              @focus="$emit('update-genre-suggestions')"
              @blur="$emit('hide-genre-suggestions')"
            />
            <button
              v-if="searchGenre && !isSearchLocked"
              type="button"
              class="absolute right-2 top-[30px] p-1 rounded-md transition-colors z-30"
              :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'"
              @click="$emit('clear-field', 'genre')"
            >
              <X class="h-4 w-4" />
            </button>
            <div
              v-show="showGenreSuggestions"
              class="absolute z-20 mt-1 w-full rounded-md shadow-lg max-h-60 overflow-auto"
              :class="theme === 'dark' ? 'bg-gray-800 border border-gray-700' : 'bg-slate-100 border border-slate-300'"
            >
              <ul class="py-1">
                <li
                  v-for="suggestion in filteredGenreSuggestions"
                  :key="suggestion.code"
                  class="px-4 py-2 text-sm cursor-pointer hover:opacity-90 transition-opacity"
                  :class="theme === 'dark' ? 'text-gray-200 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-200'"
                  @click="$emit('select-suggestion', suggestion)"
                >
                  {{ suggestion.name }}
                </li>
              </ul>
            </div>
          </div>
          <div>
            <label
              for="series"
              class="block text-sm font-medium mb-1"
              :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'"
            >Серия</label>
            <div class="relative">
              <input
                id="series"
                :value="searchSeries"
                :disabled="isSearchLocked"
                type="text"
                class="w-full px-3 py-2.5 pr-10 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'"
                @input="$emit('update:searchSeries', $event.target.value)"
              />
              <button
                v-if="searchSeries && !isSearchLocked"
                type="button"
                class="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded-md transition-colors"
                :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'"
                @click="$emit('clear-field', 'series')"
              >
                <X class="h-4 w-4" />
              </button>
            </div>
          </div>
        </div>
        <div class="flex justify-center">
          <div class="w-full max-w-md">
            <label
              class="block text-sm font-medium mb-2 text-center"
              :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'"
            >Язык книги</label>
            <select
              :value="selectedLanguage"
              :disabled="isSearchLocked"
              class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
              :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'"
              @change="$emit('update:selectedLanguage', $event.target.value); $emit('language-manual')"
            >
              <option v-for="lang in availableLanguages" :key="lang" :value="lang">
                {{ lang === 'Все языки' ? 'Все языки' : lang }}
              </option>
            </select>
          </div>
        </div>
        <div class="text-center">
          <button
            type="submit"
            :disabled="isSearchLocked"
            class="inline-flex items-center gap-2 px-6 py-2.5 rounded-lg font-medium bg-indigo-600 text-white transition-colors"
            :class="isSearchLocked ? 'opacity-70 cursor-not-allowed' : 'hover:bg-indigo-700'"
          >
            <Search class="h-4 w-4" /> {{ parseIsParsing ? 'БД обновляется…' : 'Найти' }}
          </button>
        </div>
      </form>
    </div>
  </section>
</template>
