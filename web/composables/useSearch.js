import { ref, computed, nextTick } from 'vue';
import { LANGUAGES_MAP } from '../data/languages.js';
import { GENRES_MAP } from '../data/genres.js';

const defaultReaderUrl = 'https://reader.example.com/#/read?url=';

export function useSearch(deps) {
  const {
    books,
    booksEpoch,
    loading,
    parseStatus,
    expandedAuthors,
    expandedSeries,
    groupedBooksCache,
    refreshObservers,
    visibleImages,
    imageErrors
  } = deps;

  const bumpBooksEpoch = () => {
    if (booksEpoch) booksEpoch.value += 1;
  };

  const searchTitle = ref('');
  const searchAuthor = ref('');
  const searchGenre = ref('');
  const searchSeries = ref('');
  const selectedLanguage = ref('Все языки');
  const languageManuallyChanged = ref(false);
  const availableLanguages = ref([
    'Все языки',
    ...Object.values(LANGUAGES_MAP).filter((lang) => lang !== 'Все языки').sort()
  ]);
  const readerSettings = ref({
    enabled: false,
    url: defaultReaderUrl,
    defaultSearchLanguage: ''
  });

  const showGenreSuggestions = ref(false);
  const filteredGenreSuggestions = ref([]);
  let genreTimeout = null;

  const isSearchLocked = computed(() => loading.value || parseStatus.value.isParsing);

  const fetchReaderSettings = async () => {
    try {
      const res = await fetch('/api/reader-config');
      if (res.ok) {
        const config = await res.json();
        readerSettings.value = {
          enabled: Boolean(config.reader_enabled),
          url: config.reader_url || defaultReaderUrl,
          defaultSearchLanguage: config.default_search_language || ''
        };
        const code = config.default_search_language || '';
        if (code && LANGUAGES_MAP[code]) {
          selectedLanguage.value = LANGUAGES_MAP[code];
          languageManuallyChanged.value = false;
        }
      }
    } catch (e) {
      console.error('Не удалось загрузить настройки читалки', e);
    }
  };

  const updateGenreSuggestions = () => {
    if (genreTimeout) clearTimeout(genreTimeout);

    genreTimeout = setTimeout(() => {
      if (searchGenre.value.trim() === '') {
        showGenreSuggestions.value = false;
        filteredGenreSuggestions.value = [];
        return;
      }

      const inputValue = searchGenre.value.toLowerCase();
      const allSuggestions = Object.entries(GENRES_MAP)
        .map(([code, name]) => ({ code, name: name.trim() }))
        .filter((suggestion) => suggestion.name.toLowerCase().includes(inputValue))
        .sort((a, b) => {
          const posA = a.name.toLowerCase().indexOf(inputValue);
          const posB = b.name.toLowerCase().indexOf(inputValue);
          if (posA !== posB) return posA - posB;
          return a.name.localeCompare(b.name);
        });

      const uniqueSuggestionsMap = new Map();
      allSuggestions.forEach((s) => {
        if (!uniqueSuggestionsMap.has(s.name)) uniqueSuggestionsMap.set(s.name, s);
      });

      filteredGenreSuggestions.value = Array.from(uniqueSuggestionsMap.values()).slice(0, 5);
      showGenreSuggestions.value = filteredGenreSuggestions.value.length > 0;
    }, 150);
  };

  const selectGenreSuggestion = (suggestion) => {
    searchGenre.value = suggestion.name;
    showGenreSuggestions.value = false;
  };

  const hideGenreSuggestions = () => {
    setTimeout(() => {
      showGenreSuggestions.value = false;
    }, 200);
  };

  const clearSearchField = (field) => {
    if (field === 'title') searchTitle.value = '';
    if (field === 'author') searchAuthor.value = '';
    if (field === 'series') searchSeries.value = '';
    if (field === 'genre') {
      searchGenre.value = '';
      showGenreSuggestions.value = false;
      filteredGenreSuggestions.value = [];
    }
  };

  const handleSearch = async (e) => {
    e.preventDefault();
    loading.value = true;
    imageErrors.value.clear();
    visibleImages.value.clear();
    expandedAuthors.value.clear();
    expandedSeries.value.clear();
    groupedBooksCache.value = { key: '', data: null };

    try {
      const params = new URLSearchParams();
      if (searchTitle.value) params.append('title', searchTitle.value);
      if (searchAuthor.value) params.append('author', searchAuthor.value);
      if (searchSeries.value) params.append('series', searchSeries.value);

      let langCodeToUse = null;

      if (languageManuallyChanged.value) {
        if (selectedLanguage.value !== 'Все языки') {
          const byName = Object.keys(LANGUAGES_MAP).find(
            (key) => LANGUAGES_MAP[key] === selectedLanguage.value
          );
          if (byName) langCodeToUse = byName;
        }
      } else {
        if (readerSettings.value.defaultSearchLanguage) {
          langCodeToUse = readerSettings.value.defaultSearchLanguage;
        } else if (selectedLanguage.value !== 'Все языки') {
          const byName = Object.keys(LANGUAGES_MAP).find(
            (key) => LANGUAGES_MAP[key] === selectedLanguage.value
          );
          if (byName) langCodeToUse = byName;
        }
      }

      if (langCodeToUse) {
        params.append('language', langCodeToUse);
      }
      if (searchGenre.value) {
        const input = searchGenre.value.trim();
        const inputLower = input.toLowerCase();

        let genreCodes = Object.entries(GENRES_MAP)
          .filter(([, name]) => name.trim().toLowerCase() === inputLower)
          .map(([code]) => code);

        if (genreCodes.length === 0) {
          const codeKey = Object.keys(GENRES_MAP).find(
            (code) => code.toLowerCase().replace(/\s+/g, '_') === inputLower.replace(/\s+/g, '_')
          );
          if (codeKey) genreCodes = [codeKey];
        }

        if (genreCodes.length > 0) params.append('genre', genreCodes.join(' '));
      }

      const res = await fetch(`/api/search?${params}`);
      if (!res.ok) {
        books.value = [];
        bumpBooksEpoch();
        return;
      }

      const data = await res.json();
      books.value = data.books || [];
      bumpBooksEpoch();

      nextTick(() => refreshObservers());
    } catch (e) {
      console.error('Ошибка поиска', e);
      books.value = [];
      bumpBooksEpoch();
    } finally {
      loading.value = false;
    }
  };

  return {
    searchTitle,
    searchAuthor,
    searchGenre,
    searchSeries,
    selectedLanguage,
    languageManuallyChanged,
    availableLanguages,
    readerSettings,
    showGenreSuggestions,
    filteredGenreSuggestions,
    isSearchLocked,
    fetchReaderSettings,
    updateGenreSuggestions,
    selectGenreSuggestion,
    hideGenreSuggestions,
    clearSearchField,
    handleSearch,
    defaultReaderUrl,
    disposeGenreTimeout: () => {
      if (genreTimeout) clearTimeout(genreTimeout);
    }
  };
}
