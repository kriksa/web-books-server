<script setup>
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue';
import SettingsView from './Settings.vue';
import Header from './components/Header.vue';
import SearchForm from './components/SearchForm.vue';
import ParseProgress from './components/ParseProgress.vue';
import BookGrid from './components/BookGrid.vue';
import BookDetailsModal from './components/BookDetailsModal.vue';
import FavoritesView from './components/FavoritesView.vue';
import HotkeysHelp from './components/HotkeysHelp.vue';
import ScrollToTop from './components/ScrollToTop.vue';
import SetupModal from './components/Modals/SetupModal.vue';
import ProfileModal from './components/Modals/ProfileModal.vue';
import WebAuthModal from './components/Modals/WebAuthModal.vue';
import AuthModal from './components/Modals/AuthModal.vue';
import { useTheme } from './composables/useTheme.js';
import { useAuth } from './composables/useAuth.js';
import { useParseStatus } from './composables/useParseStatus.js';
import { useFavorites } from './composables/useFavorites.js';
import { useGroupedBooks } from './composables/useGroupedBooks.js';
import { useImageLazyLoad } from './composables/useImageLazyLoad.js';
import { useSearch } from './composables/useSearch.js';
import { registerKeyboardShortcuts } from './composables/useKeyboardShortcuts.js';
import { sanitizeFilename } from './utils/helpers.js';

const currentView = ref('main');
const viewMode = ref('grid');
const selectedBook = ref(null);
const showScrollTop = ref(false);
const books = ref([]);
const booksEpoch = ref(0);
const loading = ref(false);
const showHotkeysHelp = ref(false);
const searchFormRef = ref(null);
const APP_VERSION = __APP_VERSION__;
const DISPLAY_VERSION = typeof APP_VERSION === 'string' && APP_VERSION.toLowerCase().startsWith('v')
  ? APP_VERSION
  : `v${APP_VERSION}`;

const bookDetails = ref(null);
const loadingDetails = ref(false);

const { theme, toggleTheme } = useTheme();

const auth = useAuth();
const {
  showAuthModal,
  showSetupModal,
  showProfileModal,
  profileForm,
  profileError,
  profileSuccess,
  authForm,
  authError,
  currentUser,
  token,
  fetchWithAuth,
  handleLogout,
  openAuthModal,
  closeAuthModal,
  handleAuth,
  handleSetup,
  checkSetupRequired,
  openProfileModal,
  handleUpdateProfile,
  showWebAuthModal,
  webAuthPassword,
  webAuthError,
  webPasswordRequired,
  checkWebAuthStatus,
  handleWebAuth,
  showUserMenu,
  closeUserMenu,
  userInitial
} = auth;

const { parseStatus, parseProgressPercent, parseEstimatedTime, fetchParseStatus } = useParseStatus();

const { favoriteIds, favoritesBooks, fetchFavorites, toggleFavorite } = useFavorites({
  token,
  fetchWithAuth,
  currentView
});

const {
  expandedAuthors,
  expandedSeries,
  groupedBooksCache,
  filteredBooks,
  groupedBooks,
  flatBooks,
  toggleAuthor: toggleAuthorBase,
  toggleSeries: toggleSeriesBase
} = useGroupedBooks(books, booksEpoch);

const {
  visibleImages,
  imageErrors,
  refreshObservers,
  disconnect: disconnectImageObserver,
  getImageId,
  shouldLoadImage,
  hasImageError,
  handleImageError,
  handleImageLoad
} = useImageLazyLoad();

const searchApi = useSearch({
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
});

const {
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
  disposeGenreTimeout
} = searchApi;

const showSettingsButton = computed(() => currentUser.value && currentUser.value.role === 'admin');

/** Оверлей парсинга — только не на экране настроек (после закрытия X). */
const showParseOverlay = computed(
  () => parseStatus.value.isParsing && currentView.value !== 'settings'
);
const READER_PREFS_KEY = 'reader-launch-preferences-v1';
const readerLaunchPrefs = ref({
  mode: 'embedded',
});

// Простой "роутинг" без vue-router: /reader открывает Liberama suite.
const locationPath = ref(typeof window !== 'undefined' ? window.location.pathname : '/');
const locationSearch = ref(typeof window !== 'undefined' ? window.location.search : '');

function refreshLocation() {
  locationPath.value = window.location.pathname;
  locationSearch.value = window.location.search;
}

const isReaderRoute = computed(() => locationPath.value === '/reader');

const readerIframeRef = ref(null);

function focusEmbeddedReaderIframe() {
  nextTick(() => {
    try {
      readerIframeRef.value?.focus({ preventScroll: true });
    } catch {
      try {
        readerIframeRef.value?.focus();
      } catch {}
    }
  });
}

watch(
  isReaderRoute,
  (on) => {
    if (on) focusEmbeddedReaderIframe();
    const link = typeof document !== 'undefined' ? document.getElementById('app-favicon') : null;
    if (link) {
      link.href = on ? '/reader-favicon.svg' : '/favicon.svg';
    }
  },
  { immediate: true }
);

const readerQueryUrl = computed(() => {
  const s = locationSearch.value || '';
  const p = new URLSearchParams(s.startsWith('?') ? s.slice(1) : s);
  return (p.get('url') || '').trim();
});

const liberamaIframeSrc = computed(() => {
  // Важно: используем index.html в /liberama/ и передаём url через query (?url=...),
  // потому что Liberama читает window.location.search в redirectIfNeeded().
  // Это также гарантирует, что opts.url не станет "undefined".
  const u = readerQueryUrl.value;
  if (!u) return '/liberama/index.html';
  return `/liberama/index.html?url=${encodeURIComponent(u)}`;
});

const locationHash = ref(typeof window !== 'undefined' ? window.location.hash || '' : '');

function refreshHash() {
  locationHash.value = typeof window !== 'undefined' ? window.location.hash || '' : '';
}

function parseBookIdFromReadHash(hash) {
  const h = (hash || '').trim();
  if (!h.toLowerCase().startsWith('#/read?')) return null;
  const q = h.slice('#/read?'.length);
  const id = Number(new URLSearchParams(q).get('book') || new URLSearchParams(q).get('id') || 0);
  return id > 0 ? id : null;
}

function parseBookIdFromReadPath(path, search) {
  const p = (path || '').replace(/\/+$/, '') || '/';
  if (p !== '/read') return null;
  const s = search || '';
  const id = Number(new URLSearchParams(s.startsWith('?') ? s.slice(1) : s).get('book') || 0);
  return id > 0 ? id : null;
}

/** Встроенная foliate: хэш #/read?book=… (как Liberama #/read?url=…) или путь /read?book=… */
const embeddedFoliateBookId = computed(() => {
  const fromHash = parseBookIdFromReadHash(locationHash.value);
  if (fromHash) return fromHash;
  return parseBookIdFromReadPath(locationPath.value, locationSearch.value);
});

const embeddedFoliateIframeSrc = computed(() => {
  const id = embeddedFoliateBookId.value;
  if (!id) return '';
  try {
    return new URL(`/reader.html?book=${encodeURIComponent(String(id))}`, window.location.origin).href;
  } catch {
    return `/reader.html?book=${encodeURIComponent(String(id))}`;
  }
});

function closeEmbeddedFoliate() {
  if (typeof window === 'undefined') return;
  const hadHash = /^#\/read\?/i.test(locationHash.value);
  const onReadPath = (locationPath.value || '').replace(/\/+$/, '') === '/read';
  if (hadHash) {
    history.replaceState(null, document.title, window.location.pathname + window.location.search);
    refreshHash();
  } else if (onReadPath) {
    history.replaceState(null, document.title, `${window.location.origin}/`);
    refreshLocation();
  }
}

function onEmbeddedReaderMessage(ev) {
  if (ev.origin !== window.location.origin) return;
  if (ev.data?.type === 'books-embedded-reader-close') closeEmbeddedFoliate();
}

if (typeof window !== 'undefined') {
  window.addEventListener('popstate', () => {
    refreshLocation();
    refreshHash();
  });
  window.addEventListener('hashchange', refreshHash);
}

function loadReaderLaunchPrefs() {
  try {
    const raw = localStorage.getItem(READER_PREFS_KEY);
    if (!raw) return;
    const parsed = JSON.parse(raw);
    // Старый формат: auto + embeddedEnabled=false трактовался как внешняя читалка.
    if (parsed && parsed.mode === 'auto' && parsed.embeddedEnabled === false) {
      readerLaunchPrefs.value.mode = 'external';
      return;
    }
    if (parsed && (parsed.mode === 'auto' || parsed.mode === 'external' || parsed.mode === 'embedded')) {
      readerLaunchPrefs.value.mode = parsed.mode;
      return;
    }
    if (parsed && parsed.mode === 'external') {
      readerLaunchPrefs.value.mode = 'external';
      return;
    }
    if (parsed && parsed.embeddedEnabled === true) {
      readerLaunchPrefs.value.mode = 'embedded';
    }
  } catch {
    /* ignore */
  }
}

function saveReaderLaunchPrefs() {
  try {
    localStorage.setItem(READER_PREFS_KEY, JSON.stringify(readerLaunchPrefs.value));
  } catch {
    /* ignore */
  }
}

const readBookUrl = computed(() => {
  if (!selectedBook.value || !readerSettings.value.url) return '#';

  const safeTitle = sanitizeFilename(selectedBook.value.title);
  const safeAuthor = sanitizeFilename(selectedBook.value.author);
  const fileName = `${safeTitle} - ${safeAuthor}.${selectedBook.value.format}`;
  const downloadUri = `/download/${selectedBook.value.id}/${fileName}`;
  const baseUrl = window.location.origin || '';
  const fullDownloadUrl = `${baseUrl}${downloadUri}`;

  return `${readerSettings.value.url}${encodeURIComponent(fullDownloadUrl)}`;
});

const hasExternalReader = computed(
  () => readerSettings.value.enabled && readerSettings.value.url && readBookUrl.value !== '#'
);

/** PDF — только внешняя читалка (reader_url), если он настроен; иначе запасной вариант /reader. */
function bookUsesExternalLiberamaUrl(book) {
  if (!book || book.format == null) return false;
  return String(book.format).toLowerCase().trim() === 'pdf';
}

/** В /reader (Liberama) в браузере часто «вешается» разбор или не реализован нормально — не открываем там. */
function skipInBrowserLiberama(book) {
  if (!book || book.format == null) return false;
  const f = String(book.format).toLowerCase().trim();
  return f === 'djvu';
}

const canReadBook = computed(() => readerSettings.value.enabled);

function openInNewTab(url) {
  if (typeof window === 'undefined') return;
  if (!url) return;
  // В ряде окружений (особенно с noopener/noreferrer) window.open может вернуть null,
  // но вкладку всё равно открыть — из-за этого легко получить "две вкладки" при fallback.
  // Поэтому сначала используем "настоящий" клик по ссылке, и только если он не сработал —
  // fallback на window.open().
  try {
    const a = document.createElement('a');
    a.href = url;
    a.target = '_blank';
    a.rel = 'noopener noreferrer';
    a.style.display = 'none';
    document.body.appendChild(a);
    a.click();
    a.remove();
    return;
  } catch {
    //
  }

  let w = null;
  try {
    w = window.open(url, '_blank');
  } catch {
    w = null;
  }
  // Last resort (may navigate same tab)
  if (!w) {
    try {
      window.location.href = url;
    } catch {
      //
    }
  }
}

const openReadBook = () => {
  if (!selectedBook.value) return;
  const id = selectedBook.value.id;
  const safeTitle = sanitizeFilename(selectedBook.value.title);
  const safeAuthor = sanitizeFilename(selectedBook.value.author);
  const fileName = `${safeTitle} - ${safeAuthor}.${selectedBook.value.format}`;
  const downloadUri = `/download/${id}/${fileName}`;
  const origin = window.location.origin || '';
  const fullDownloadUrl = `${origin}${downloadUri}`;

  const liberamaPage = `${origin}/reader?url=${encodeURIComponent(fullDownloadUrl)}`;
  const fallback = hasExternalReader.value ? readBookUrl.value : null;
  const pdf = bookUsesExternalLiberamaUrl(selectedBook.value);

  // DJVU: не открываем в Liberama (зависание); внешняя читалка или встроенная foliate через /#/read?book=…
  if (skipInBrowserLiberama(selectedBook.value)) {
    if (fallback) openInNewTab(fallback);
    else openInNewTab(`${origin}/#/read?book=${id}`);
    selectedBook.value = null;
    return;
  }

  // Требование: читалка открывается в НОВОЙ вкладке всегда.
  // PDF: предпочитаем внешний reader_url (если настроен), иначе Liberama.
  if (pdf && fallback) openInNewTab(fallback);
  else openInNewTab(liberamaPage);

  selectedBook.value = null;
};

const selectedBookIndex = computed(() => {
  if (!selectedBook.value || flatBooks.value.length === 0) return -1;
  return flatBooks.value.findIndex((b) => b.id === selectedBook.value.id);
});

const navigateBook = (delta) => {
  const list = flatBooks.value;
  if (list.length === 0) return;
  let idx = selectedBookIndex.value;
  if (idx < 0) idx = delta > 0 ? 0 : list.length - 1;
  else idx = (idx + delta + list.length) % list.length;
  selectedBook.value = list[idx];
};

const focusSearch = () => {
  searchFormRef.value?.focusSearch?.();
};

const toggleAuthor = (authorName) => {
  toggleAuthorBase(authorName);
  nextTick(() => refreshObservers());
};

const toggleSeries = (authorName, seriesName) => {
  toggleSeriesBase(authorName, seriesName);
  nextTick(() => refreshObservers());
};

const closeModal = () => {
  selectedBook.value = null;
  bookDetails.value = null;
};

const openFavorites = async () => {
  currentView.value = 'favorites';
  visibleImages.value.clear();
  if (!token.value) return;
  try {
    const res = await fetchWithAuth('/api/favorites/books');
    if (res.ok) {
      const data = await res.json();
      favoritesBooks.value = data.books || [];
      nextTick(() => refreshObservers());
    }
  } catch (e) {
    favoritesBooks.value = [];
  }
};

const closeFavorites = () => {
  currentView.value = 'main';
  visibleImages.value.clear();
  nextTick(() => refreshObservers());
};

const scrollToTop = () => {
  window.scrollTo({ top: 0, behavior: 'smooth' });
};

function onSettingsConfigSaved() {
  loadReaderLaunchPrefs();
  fetchReaderSettings();
  // Парсинг мог уже стартовать на сервере; оверлей покажем после закрытия настроек.
  fetchParseStatus();
}

const openSettings = () => {
  if (currentUser.value && currentUser.value.role === 'admin') currentView.value = 'settings';
};

const closeSettings = () => {
  currentView.value = 'main';
  loadReaderLaunchPrefs();
  fetchReaderSettings();
  fetchParseStatus();
  nextTick(() => refreshObservers());
};

const openProfileFromMenu = () => {
  closeUserMenu();
  openProfileModal();
};

const openSettingsFromMenu = () => {
  closeUserMenu();
  openSettings();
};

const openFavoritesFromMenu = async () => {
  closeUserMenu();
  await openFavorites();
};

const logoutFromMenu = () => {
  closeUserMenu();
  handleLogout();
};

watch(selectedBook, async (newBook) => {
  if (!newBook) {
    bookDetails.value = null;
    return;
  }

  bookDetails.value = null;
  if (newBook.format === 'fb2' || newBook.format === 'epub') {
    loadingDetails.value = true;
    try {
      const res = await fetchWithAuth(`/api/book/details?id=${newBook.id}`);
      if (res.ok) {
        bookDetails.value = await res.json();
      }
    } catch (e) {
      console.error('Ошибка загрузки деталей:', e);
    } finally {
      loadingDetails.value = false;
    }
  }
});

watch(selectedBook, (newVal) => {
  if (newVal) {
    document.body.style.overflow = 'hidden';
    document.body.style.touchAction = 'none';
  } else {
    document.body.style.overflow = '';
    document.body.style.touchAction = '';
  }
});

const mountCleanup = {
  removeKbd: null,
  parseIntervalId: null,
  handleScrollFn: null
};
let appDisposed = false;

function onGlobalKeydownEmbeddedClose(e) {
  if (!embeddedFoliateBookId.value) return;
  if (e.key === 'Escape') {
    e.preventDefault();
    closeEmbeddedFoliate();
  }
}

watch(embeddedFoliateBookId, (id) => {
  if (typeof document === 'undefined') return;
  if (id) {
    document.body.style.overflow = 'hidden';
    document.body.style.touchAction = 'none';
    window.addEventListener('keydown', onGlobalKeydownEmbeddedClose, true);
  } else {
    document.body.style.overflow = '';
    document.body.style.touchAction = '';
    window.removeEventListener('keydown', onGlobalKeydownEmbeddedClose, true);
  }
});

onMounted(() => {
  loadReaderLaunchPrefs();
  refreshHash();
  window.addEventListener('message', onEmbeddedReaderMessage);
  (async () => {
    await Promise.all([
      checkWebAuthStatus(),
      checkSetupRequired(),
      fetchReaderSettings(),
      fetchParseStatus()
    ]);

    if (appDisposed) return;

    if (showSetupModal.value) return;

    if (token.value) {
      try {
        const res = await fetchWithAuth('/api/user/status');
        if (res.ok) {
          currentUser.value = await res.json();
          await fetchFavorites();
        } else {
          handleLogout();
        }
      } catch (e) {
        currentUser.value = null;
      }
    }

    if (appDisposed) return;

    mountCleanup.removeKbd = registerKeyboardShortcuts({
      currentView,
      focusSearch,
      showUserMenu,
      selectedBook,
      closeModal,
      showHotkeysHelp,
      isSearchLocked,
      navigateBook,
      toggleFavorite,
      readerSettings,
      openReadBook,
      flatBooks
    });

    mountCleanup.parseIntervalId = setInterval(fetchParseStatus, 2000);

    mountCleanup.handleScrollFn = () => {
      showScrollTop.value = window.scrollY > 300;
    };
    window.addEventListener('scroll', mountCleanup.handleScrollFn);
  })();
});

onUnmounted(() => {
  appDisposed = true;
  mountCleanup.removeKbd?.();
  if (mountCleanup.parseIntervalId) clearInterval(mountCleanup.parseIntervalId);
  if (mountCleanup.handleScrollFn) {
    window.removeEventListener('scroll', mountCleanup.handleScrollFn);
  }
  if (typeof window !== 'undefined') {
    window.removeEventListener('hashchange', refreshHash);
  }
  window.removeEventListener('message', onEmbeddedReaderMessage);
  window.removeEventListener('keydown', onGlobalKeydownEmbeddedClose, true);
  if (typeof document !== 'undefined') {
    document.body.style.overflow = '';
    document.body.style.touchAction = '';
  }
  disposeGenreTimeout();
  disconnectImageObserver();
});
</script>

<template>
  <div
    class="min-h-screen transition-colors duration-300 overflow-x-hidden"
    :class="theme === 'dark' ? 'bg-gray-900 text-gray-100' : 'bg-gradient-to-br from-slate-100 to-slate-200 text-slate-800'"
  >
    <div v-if="currentView === 'main'">
      <Header
        :theme="theme"
        :current-user="currentUser"
        :show-settings-button="showSettingsButton"
        :user-initial="userInitial"
        :show-user-menu="showUserMenu"
        @toggle-theme="toggleTheme"
        @open-settings="openSettings"
        @open-favorites="openFavorites"
        @toggle-hotkeys="showHotkeysHelp = !showHotkeysHelp"
        @open-auth="openAuthModal"
        @open-profile="openProfileModal"
        @logout="handleLogout"
        @toggle-user-menu="showUserMenu = !showUserMenu"
        @close-user-menu="closeUserMenu"
        @open-profile-from-menu="openProfileFromMenu"
        @open-settings-from-menu="openSettingsFromMenu"
        @open-favorites-from-menu="openFavoritesFromMenu"
        @logout-from-menu="logoutFromMenu"
      />

      <SearchForm
        ref="searchFormRef"
        :theme="theme"
        :is-search-locked="isSearchLocked"
        :parse-is-parsing="parseStatus.isParsing"
        :search-title="searchTitle"
        :search-author="searchAuthor"
        :search-genre="searchGenre"
        :search-series="searchSeries"
        :selected-language="selectedLanguage"
        :available-languages="availableLanguages"
        :show-genre-suggestions="showGenreSuggestions"
        :filtered-genre-suggestions="filteredGenreSuggestions"
        @submit="handleSearch"
        @update:search-title="searchTitle = $event"
        @update:search-author="searchAuthor = $event"
        @update:search-genre="searchGenre = $event"
        @update:search-series="searchSeries = $event"
        @update:selected-language="selectedLanguage = $event"
        @language-manual="languageManuallyChanged = true"
        @update-genre-suggestions="updateGenreSuggestions"
        @hide-genre-suggestions="hideGenreSuggestions"
        @clear-field="clearSearchField"
        @select-suggestion="selectGenreSuggestion"
      />

      <BookGrid
        :theme="theme"
        :view-mode="viewMode"
        :loading="loading"
        :filtered-books-count="filteredBooks.length"
        :grouped-books="groupedBooks"
        :expanded-authors="expandedAuthors"
        :expanded-series="expandedSeries"
        :current-user="currentUser"
        :favorite-ids="favoriteIds"
        :should-load-image="shouldLoadImage"
        :has-image-error="hasImageError"
        :get-image-id="getImageId"
        @toggle-author="toggleAuthor"
        @toggle-series="toggleSeries"
        @select-book="selectedBook = $event"
        @toggle-favorite="toggleFavorite"
        @image-error="handleImageError"
        @image-load="handleImageLoad"
      />

      <HotkeysHelp v-if="showHotkeysHelp" :theme="theme" @close="showHotkeysHelp = false" />

      <ScrollToTop v-if="showScrollTop" @click="scrollToTop" />

    </div>

    <FavoritesView
      v-else-if="currentView === 'favorites'"
      :theme="theme"
      :favorites-books="favoritesBooks"
      :favorite-ids="favoriteIds"
      :should-load-image="shouldLoadImage"
      :has-image-error="hasImageError"
      :get-image-id="getImageId"
      @close="closeFavorites"
      @select-book="selectedBook = $event"
      @toggle-favorite="toggleFavorite"
      @image-error="handleImageError"
      @image-load="handleImageLoad"
    />

    <div v-else-if="currentView === 'settings'" class="overflow-x-hidden">
      <SettingsView :token="token" @close="closeSettings" @config-saved="onSettingsConfigSaved" />
    </div>

    <Teleport to="body">
      <BookDetailsModal
        v-if="selectedBook"
        :theme="theme"
        :book="selectedBook"
        :current-user="currentUser"
        :favorite-ids="favoriteIds"
        :reader-enabled="canReadBook"
        :book-details="bookDetails"
        :loading-details="loadingDetails"
        :has-image-error="hasImageError"
        @close="closeModal"
        @read="openReadBook"
        @toggle-favorite="toggleFavorite"
        @image-error="handleImageError"
        @image-load="handleImageLoad"
      />
    </Teleport>

    <Teleport to="body">
      <div v-if="isReaderRoute" class="fixed inset-0 z-[200] bg-black">
        <iframe
          ref="readerIframeRef"
          tabindex="0"
          title="Читалка Liberama"
          :src="liberamaIframeSrc"
          class="absolute inset-0 w-full h-full border-0 outline-none"
          allow="fullscreen; clipboard-read; clipboard-write"
          referrerpolicy="no-referrer"
          @load="focusEmbeddedReaderIframe"
        />
      </div>
    </Teleport>

    <Teleport to="body">
      <div
        v-if="embeddedFoliateBookId"
        class="fixed inset-0 z-[210] bg-black"
        role="dialog"
        aria-modal="true"
        aria-label="Читалка"
      >
        <iframe
          v-if="embeddedFoliateIframeSrc"
          :key="embeddedFoliateBookId"
          title="Читалка"
          :src="embeddedFoliateIframeSrc"
          class="h-full w-full border-0 outline-none"
          allow="fullscreen; clipboard-read; clipboard-write"
        />
      </div>
    </Teleport>

    <Teleport to="body">
      <SetupModal
        v-if="showSetupModal"
        :auth-form="authForm"
        :auth-error="authError"
        @submit="handleSetup"
        @update:auth-form="Object.assign(authForm, $event)"
      />

      <ProfileModal
        v-if="showProfileModal"
        :theme="theme"
        :profile-form="profileForm"
        :profile-error="profileError"
        :profile-success="profileSuccess"
        @submit="handleUpdateProfile"
        @close="showProfileModal = false"
        @update:profile-form="Object.assign(profileForm, $event)"
      />

      <WebAuthModal
        v-if="showWebAuthModal"
        :web-auth-password="webAuthPassword"
        :web-auth-error="webAuthError"
        @submit="handleWebAuth"
        @update:web-auth-password="webAuthPassword = $event"
      />

      <AuthModal
        v-if="showAuthModal"
        :theme="theme"
        :auth-form="authForm"
        :auth-error="authError"
        @close="closeAuthModal"
        @submit="handleAuth"
        @update:auth-form="Object.assign(authForm, $event)"
      />
    </Teleport>

    <ParseProgress
      :show="showParseOverlay"
      :theme="theme"
      :parse-status="parseStatus"
      :parse-progress-percent="parseProgressPercent"
      :parse-estimated-time="parseEstimatedTime"
    />

    <div
      class="fixed left-4 bottom-4 z-50 text-xs opacity-70 pointer-events-none select-none"
      :class="theme === 'dark' ? 'text-gray-400' : 'text-slate-600'"
      style="padding-left: env(safe-area-inset-left, 0px); padding-bottom: env(safe-area-inset-bottom, 0px);"
    >
      {{ DISPLAY_VERSION }}
    </div>
  </div>
</template>
