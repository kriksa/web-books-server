<script setup>
import { ref, computed, onMounted, watch, nextTick } from 'vue';
import { Search, BookOpen, Download, X, Sun, Moon, ArrowUp, Settings, User, LogOut, ChevronRight, ChevronDown, Heart, Keyboard } from 'lucide-vue-next';
import SettingsView from './Settings.vue';
import { LANGUAGES_MAP } from './data/languages.js';
import { GENRES_MAP } from './data/genres.js';

// ===== STATE =====
const currentView = ref('main');
const searchTitle = ref("");
const searchAuthor = ref("");
const searchGenre = ref("");
const searchSeries = ref("");
const selectedLanguage = ref("Все языки");
const languageManuallyChanged = ref(false);
const availableLanguages = ref(["Все языки", ...Object.values(LANGUAGES_MAP).filter(lang => lang !== "Все языки").sort()]);
const theme = ref("light");
const viewMode = ref("grid");
const selectedBook = ref(null);
const showScrollTop = ref(false);
const books = ref([]);
const loading = ref(false);
const imageErrors = ref(new Set());
const visibleImages = ref(new Set()); // Для отслеживания видимых изображений

// ===== STATE ДЛЯ СТАТУСА ПАРСИНГА =====
const parseStatus = ref({
  isParsing: false,
  progress: 0,
  total: 0,
  message: "",
  estimatedRemainingSec: 0
});
const parseStatusInFlight = ref(false);

// ===== STATE ДЛЯ ДЕТАЛЕЙ КНИГИ =====
const bookDetails = ref(null);
const loadingDetails = ref(false);

// ===== STATE ДЛЯ АВТОДОПОЛНЕНИЯ ЖАНРА =====
const showGenreSuggestions = ref(false);
const filteredGenreSuggestions = ref([]);
let genreTimeout = null;

// ===== STATE ДЛЯ АВТОРИЗАЦИИ =====
const showAuthModal = ref(false);
const showSetupModal = ref(false);
const showProfileModal = ref(false);
const profileForm = ref({ oldPassword: '', newPassword: '', confirmPassword: '', newUsername: '' });
const profileError = ref('');
const profileSuccess = ref('');
const authMode = ref('login');
const authForm = ref({ username: '', password: '', confirmPassword: '' });
const authError = ref('');
const currentUser = ref(null);
const token = ref(localStorage.getItem('token') || null);

// ===== STATE ДЛЯ ВЕБ-АУТЕНТИФИКАЦИИ =====
const showWebAuthModal = ref(false);
const webAuthPassword = ref('');
const webAuthError = ref('');
const webPasswordRequired = ref(false);

// ===== STATE ДЛЯ ГРУППИРОВКИ АВТОРОВ =====
const expandedAuthors = ref(new Set());
const expandedSeries = ref(new Set());

// ===== STATE ДЛЯ ЧИТАЛКИ =====
const defaultReaderUrl = 'https://reader.example.com/#/read?url=';
const readerSettings = ref({
  enabled: false,
  url: defaultReaderUrl,
  defaultSearchLanguage: ''
});

// ===== ИЗБРАННОЕ =====
const favoriteIds = ref(new Set());
const favoritesBooks = ref([]);
const showHotkeysHelp = ref(false);
const searchInputRef = ref(null);

// ===== КЭШИРОВАНИЕ =====
const groupedBooksCache = ref({
  key: '',
  data: null
});

// ===== INTERSECTION OBSERVER =====
let imageObserver = null;

const setupImageObserver = () => {
  if (imageObserver) {
    imageObserver.disconnect();
  }
  
  imageObserver = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
      if (entry.isIntersecting) {
        const imgId = entry.target.dataset.imgId;
        if (imgId) {
          visibleImages.value.add(imgId);
          // Прекращаем наблюдать за этим элементом после загрузки
          imageObserver.unobserve(entry.target);
        }
      }
    });
  }, {
    root: null,
    rootMargin: '50px', // Начинаем загружать чуть раньше, чем элемент появится
    threshold: 0.1
  });
  
  // Наблюдаем за всеми placeholder'ами изображений
  nextTick(() => {
    const placeholders = document.querySelectorAll('.book-cover-placeholder');
    placeholders.forEach(placeholder => {
      imageObserver.observe(placeholder);
    });
  });
};

// ===== COMPUTED =====
const filteredBooks = computed(() => books.value);

const parseProgressPercent = computed(() => {
  const s = parseStatus.value;
  if (!s.total || s.total <= 0) return null;
  return Math.round((s.progress / s.total) * 100);
});

const parseEstimatedTime = computed(() => {
  const sec = parseStatus.value.estimatedRemainingSec;
  if (!sec || sec <= 0) return null;
  if (sec > 60 * 60 * 24) return null;
  if (sec < 60) return `~${sec} сек`;
  const min = Math.ceil(sec / 60);
  if (min < 60) return `~${min} мин`;
  const h = Math.floor(min / 60);
  const m = min % 60;
  return m > 0 ? `~${h} ч ${m} мин` : `~${h} ч`;
});

const isSearchLocked = computed(() => loading.value || parseStatus.value.isParsing);

const flatBooks = computed(() => {
  const list = [];
  groupedBooks.value.forEach(ag => {
    ag.seriesGroups.forEach(sg => {
      sg.books.forEach(b => list.push(b));
    });
  });
  return list;
});

const selectedBookIndex = computed(() => {
  if (!selectedBook.value || flatBooks.value.length === 0) return -1;
  return flatBooks.value.findIndex(b => b.id === selectedBook.value.id);
});

const isFavorite = (book) => book && favoriteIds.value.has(book.id);

const showSettingsButton = computed(() => {
  return currentUser.value && currentUser.value.role === 'admin';
});

const readBookUrl = computed(() => {
  if (!selectedBook.value || !readerSettings.value.enabled || !readerSettings.value.url) return '#';

  const safeTitle = sanitizeFilename(selectedBook.value.title);
  const safeAuthor = sanitizeFilename(selectedBook.value.author);
  const fileName = `${safeTitle} - ${safeAuthor}.${selectedBook.value.format}`;
  const downloadUri = `/download/${selectedBook.value.id}/${fileName}`;
  const baseUrl = window.location.origin || 'http://app.books-kriksa.ru';
  const fullDownloadUrl = `${baseUrl}${downloadUri}`;

  return `${readerSettings.value.url}${encodeURIComponent(fullDownloadUrl)}`;
});

const groupedBooks = computed(() => {
  const cacheKey = `${books.value.length}_${expandedAuthors.value.size}_${expandedSeries.value.size}_${JSON.stringify(books.value.slice(0, 3).map(b => b.id))}`;

  if (groupedBooksCache.value.key === cacheKey && groupedBooksCache.value.data) {
    return groupedBooksCache.value.data;
  }

  const groups = {};
  const authorsOrder = [];

  filteredBooks.value.forEach(book => {
    const authorName = book.author || 'Без автора';
    if (!groups[authorName]) {
      groups[authorName] = {};
      authorsOrder.push(authorName);
    }

    const seriesName = book.series || 'Без серии';
    if (!groups[authorName][seriesName]) {
      groups[authorName][seriesName] = [];
    }
    groups[authorName][seriesName].push(book);
  });

  const result = authorsOrder.map(author => {
    const seriesGroups = groups[author];
    const seriesEntries = Object.entries(seriesGroups);

    seriesEntries.sort(([seriesA], [seriesB]) => {
      if (seriesA === 'Без серии' && seriesB !== 'Без серии') return -1;
      if (seriesB === 'Без серии' && seriesA !== 'Без серии') return 1;
      return seriesA.localeCompare(seriesB);
    });

    const processedSeries = seriesEntries.map(([seriesName, booksArray]) => {
      const sortedBooks = booksArray.sort((a, b) => {
        if (a.seriesNo && b.seriesNo) {
          return a.seriesNo - b.seriesNo;
        }
        if (a.seriesNo) return -1;
        if (b.seriesNo) return 1;

        const isRuA = a.language === 'ru' || a.language === 'ru-';
        const isRuB = b.language === 'ru' || b.language === 'ru-';
        if (isRuA && !isRuB) return -1;
        if (!isRuA && isRuB) return 1;
        return a.title.localeCompare(b.title);
      });

      return {
        series: seriesName,
        books: sortedBooks
      };
    });

    return {
      author: author,
      seriesGroups: processedSeries
    };
  });

  groupedBooksCache.value = {
    key: cacheKey,
    data: result
  };

  return result;
});

// ===== МЕТОДЫ =====
const getLanguageName = (langCode) => LANGUAGES_MAP[langCode] || langCode;

const getGenreNames = (genreCodes) => {
  if (!genreCodes || typeof genreCodes !== 'string' || genreCodes.trim() === '') return '—';
  const codes = genreCodes.split(',').map(code => code.trim()).filter(Boolean);
  const names = codes.map(code => {
    const normalized = code.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '');
    return (GENRES_MAP && GENRES_MAP[normalized]) ? GENRES_MAP[normalized] : code;
  });
  return names.join(', ');
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
      .filter(suggestion => suggestion.name.toLowerCase().includes(inputValue))
      .sort((a, b) => {
        const posA = a.name.toLowerCase().indexOf(inputValue);
        const posB = b.name.toLowerCase().indexOf(inputValue);
        if (posA !== posB) return posA - posB;
        return a.name.localeCompare(b.name);
      });

    const uniqueSuggestionsMap = new Map();
    allSuggestions.forEach(s => {
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
  setTimeout(() => { showGenreSuggestions.value = false; }, 200);
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

const fetchWithAuth = (url, options = {}) => {
  const headers = { 'Content-Type': 'application/json', ...options.headers };
  if (token.value) headers['Authorization'] = `Bearer ${token.value}`;
  return fetch(url, { ...options, headers });
};

const toggleAuthor = (authorName) => {
  if (expandedAuthors.value.has(authorName)) expandedAuthors.value.delete(authorName);
  else expandedAuthors.value.add(authorName);
  groupedBooksCache.value = { key: '', data: null };
  // Перенастраиваем observer после изменения DOM
  nextTick(() => setupImageObserver());
};

const toggleSeries = (authorName, seriesName) => {
  const key = `${authorName}_${seriesName}`;
  if (expandedSeries.value.has(key)) expandedSeries.value.delete(key);
  else expandedSeries.value.add(key);
  groupedBooksCache.value = { key: '', data: null };
  // Перенастраиваем observer после изменения DOM
  nextTick(() => setupImageObserver());
};

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
    console.error("Не удалось загрузить настройки читалки", e);
  }
};

const fetchParseStatus = async () => {
  if (parseStatusInFlight.value) return;
  parseStatusInFlight.value = true;

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 1500);
  try {
    const res = await fetch('/api/app-status', {
      cache: 'no-store',
      signal: controller.signal
    });
    if (res.ok) {
      const data = await res.json();
      parseStatus.value = {
        isParsing: data.is_parsing || false,
        progress: Number(data.progress || 0),
        total: Number(data.total || 0),
        message: data.message || "",
        estimatedRemainingSec: data.estimated_remaining_sec || 0,
        currentFile: data.current_file || ""
      };
    }
  } catch (e) {
    if (e?.name !== 'AbortError') {
      console.error("Не удалось загрузить статус парсинга", e);
    }
  } finally {
    clearTimeout(timeoutId);
    parseStatusInFlight.value = false;
  }
};

const handleSearch = async (e) => {
  e.preventDefault();
  loading.value = true;
  imageErrors.value.clear();
  visibleImages.value.clear(); // Очищаем видимые изображения при новом поиске
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
      if (selectedLanguage.value !== "Все языки") {
        const byName = Object.keys(LANGUAGES_MAP).find(
          key => LANGUAGES_MAP[key] === selectedLanguage.value
        );
        if (byName) langCodeToUse = byName;
      }
    } else {
      if (readerSettings.value.defaultSearchLanguage) {
        langCodeToUse = readerSettings.value.defaultSearchLanguage;
      } else if (selectedLanguage.value !== "Все языки") {
        const byName = Object.keys(LANGUAGES_MAP).find(
          key => LANGUAGES_MAP[key] === selectedLanguage.value
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
        .filter(([code, name]) => name.trim().toLowerCase() === inputLower)
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
      return;
    }

    const data = await res.json();
    books.value = data.books || [];
    
    // Настраиваем observer после загрузки результатов
    nextTick(() => setupImageObserver());
  } catch (e) {
    console.error("Ошибка поиска", e);
    books.value = [];
  } finally {
    loading.value = false;
  }
};

const closeModal = () => {
  selectedBook.value = null;
  bookDetails.value = null;
};

const fetchFavorites = async () => {
  if (!token.value) return;
  try {
    const res = await fetchWithAuth('/api/favorites');
    if (res.ok) {
      const data = await res.json();
      favoriteIds.value = new Set((data.book_ids || []).map(Number));
    }
  } catch (e) { console.error('Ошибка загрузки избранного', e); }
};

const toggleFavorite = async (book) => {
  if (!book || !token.value) return;
  const id = book.id;
  const add = !favoriteIds.value.has(id);
  try {
    const url = '/api/favorites';
    const options = {
      method: add ? 'POST' : 'DELETE',
      headers: { 'Content-Type': 'application/json', ...(token.value ? { Authorization: `Bearer ${token.value}` } : {}) },
      body: JSON.stringify({ book_id: id })
    };
    const res = await fetchWithAuth(url, options);
    if (res.ok) {
      if (add) favoriteIds.value.add(id);
      else {
        favoriteIds.value.delete(id);
        if (currentView.value === 'favorites') {
          favoritesBooks.value = favoritesBooks.value.filter(b => b.id !== id);
        }
      }
      favoriteIds.value = new Set(favoriteIds.value);
    }
  } catch (e) { console.error('Ошибка избранного', e); }
};

const navigateBook = (delta) => {
  const list = flatBooks.value;
  if (list.length === 0) return;
  let idx = selectedBookIndex.value;
  if (idx < 0) idx = delta > 0 ? 0 : list.length - 1;
  else idx = (idx + delta + list.length) % list.length;
  selectedBook.value = list[idx];
};

const focusSearch = () => {
  searchInputRef.value?.focus();
};

const openFavorites = async () => {
  currentView.value = 'favorites';
  visibleImages.value.clear(); // Очищаем при переключении на избранное
  if (!token.value) return;
  try {
    const res = await fetchWithAuth('/api/favorites/books');
    if (res.ok) {
      const data = await res.json();
      favoritesBooks.value = data.books || [];
      // Настраиваем observer после загрузки избранного
      nextTick(() => setupImageObserver());
    }
  } catch (e) { favoritesBooks.value = []; }
};

const closeFavorites = () => {
  currentView.value = 'main';
  visibleImages.value.clear();
  nextTick(() => setupImageObserver());
};

const scrollToTop = () => { window.scrollTo({ top: 0, behavior: 'smooth' }); };

const toggleTheme = () => {
  theme.value = theme.value === 'dark' ? 'light' : 'dark';
};

const openSettings = () => {
  if (currentUser.value && currentUser.value.role === 'admin') currentView.value = 'settings';
};

const closeSettings = () => {
  currentView.value = 'main';
  fetchReaderSettings();
  nextTick(() => setupImageObserver());
};

const formatFileSize = (bytes) => (bytes / 1024 / 1024).toFixed(2) + ' МБ';

const getCoverUrl = (book) => {
  if (book.format === 'fb2' || book.format === 'epub') {
    return `/api/cover?file=${book.fileName}&zip=${book.zip}&format=${book.format}`;
  }
  return null;
};

const getImageId = (book) => `${book.id}-${book.fileName}-${book.zip}`;

const shouldLoadImage = (book) => visibleImages.value.has(getImageId(book));

const hasImageError = (book) => imageErrors.value.has(`${book.fileName}-${book.zip}`);

const handleImageError = (event, book) => {
 imageErrors.value = new Set(imageErrors.value).add(`${book.fileName}-${book.zip}`);


};

const handleImageLoad = (event, book) => {
  if (book) imageErrors.value.delete(`${book.fileName}-${book.zip}`);
};

// ===== AUTH METHODS =====
const openAuthModal = (mode = 'login') => {
  authMode.value = mode;
  authForm.value = { username: '', password: '', confirmPassword: '' };
  authError.value = '';
  showAuthModal.value = true;
};

const closeAuthModal = () => {
  showAuthModal.value = false;
  authError.value = '';
};

const handleAuth = async () => {
  try {
    const res = await fetch('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: authForm.value.username, password: authForm.value.password })
    });
    const data = await res.json();
    if (res.ok) {
      token.value = data.token;
      currentUser.value = data.user;
      localStorage.setItem('token', data.token);
      closeAuthModal();
    } else {
      authError.value = data.message || 'Неверное имя пользователя или пароль';
    }
  } catch (e) {
    console.error('Ошибка входа:', e);
    authError.value = 'Произошла ошибка при попытке авторизации';
  }
};

const handleSetup = async () => {
  if (authForm.value.password.length < 3) {
    authError.value = 'Пароль должен содержать минимум 3 символа';
    return;
  }
  try {
    const res = await fetch('/api/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: authForm.value.username, password: authForm.value.password })
    });
    const data = await res.json();
    if (res.ok) {
      token.value = data.token;
      currentUser.value = data.user;
      localStorage.setItem('token', data.token);
      showSetupModal.value = false;
      authError.value = '';
    } else {
      authError.value = data.message || 'Ошибка настройки';
    }
  } catch (e) {
    authError.value = 'Произошла ошибка';
  }
};

const checkSetupRequired = async () => {
  try {
    const res = await fetch('/api/setup-status');
    if (res.ok) {
      const data = await res.json();
      if (data.setup_required) showSetupModal.value = true;
    }
  } catch (e) {}
};

const openProfileModal = () => {
  profileForm.value = { oldPassword: '', newPassword: '', confirmPassword: '', newUsername: currentUser.value?.username || '' };
  profileError.value = '';
  profileSuccess.value = '';
  showProfileModal.value = true;
};

const handleUpdateProfile = async () => {
  profileError.value = '';
  profileSuccess.value = '';
  if (profileForm.value.newPassword && profileForm.value.newPassword !== profileForm.value.confirmPassword) {
    profileError.value = 'Пароли не совпадают';
    return;
  }
  if (profileForm.value.newPassword && profileForm.value.newPassword.length < 3) {
    profileError.value = 'Пароль должен содержать минимум 3 символа';
    return;
  }
  try {
    const body = {};
    if (profileForm.value.newPassword) {
      body.old_password = profileForm.value.oldPassword;
      body.new_password = profileForm.value.newPassword;
    }
    if (profileForm.value.newUsername && profileForm.value.newUsername !== currentUser.value?.username) {
      body.new_username = profileForm.value.newUsername;
    }
    if (Object.keys(body).length === 0) {
      profileError.value = 'Укажите новый пароль или имя';
      return;
    }
    if (body.new_password && !body.old_password) {
      profileError.value = 'Для смены пароля укажите старый пароль';
      return;
    }
    const res = await fetchWithAuth('/api/update-profile', {
      method: 'POST',
      body: JSON.stringify(body)
    });
    const data = await res.json();
    if (res.ok) {
      profileSuccess.value = 'Профиль обновлён';
      if (body.new_username) currentUser.value = { ...currentUser.value, username: body.new_username };
    } else {
      profileError.value = data.message || 'Ошибка обновления';
    }
  } catch (e) {
    profileError.value = 'Ошибка связи с сервером';
  }
};

const handleLogout = () => {
  token.value = null;
  currentUser.value = null;
  localStorage.removeItem('token');
};

const checkWebAuthStatus = async () => {
  try {
    const res = await fetch('/api/web-auth-status');
    if (res.ok) {
      const data = await res.json();
      webPasswordRequired.value = data.password_required;
      if (data.password_required) showWebAuthModal.value = true;
    }
  } catch (e) { console.error(e); }
};

const handleWebAuth = async () => {
  try {
    const res = await fetch('/api/web-auth', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password: webAuthPassword.value })
    });
    if (res.ok) {
      document.cookie = "web_auth_session=authenticated; path=/; max-age=2592000";
      showWebAuthModal.value = false;
      webAuthError.value = '';
      webAuthPassword.value = '';
    } else {
      webAuthError.value = 'Неверный пароль';
    }
  } catch (e) { webAuthError.value = 'Произошла ошибка'; }
};

const sanitizeFilename = (name) => {
  if (!name) return 'book';
  return name.trim().replace(/[^a-zA-Z0-9а-яА-ЯёЁ\s\-\.]/g, '').replace(/\s+/g, '_');
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
      console.error("Ошибка загрузки деталей:", e);
    } finally {
      loadingDetails.value = false;
    }
  }
});

onMounted(async () => {
  const initialPromises = [
    checkWebAuthStatus(),
    checkSetupRequired(),
    fetchReaderSettings(),
    fetchParseStatus()
  ];

  await Promise.all(initialPromises);

  if (showSetupModal.value) return;

  if (token.value) {
    try {
      const res = await fetchWithAuth('/api/user/status');
      if (res.ok) {
        currentUser.value = await res.json();
        await fetchFavorites();
      } else handleLogout();
    } catch (e) { currentUser.value = null; }
  }

  const onKeydown = (e) => {
    const tag = (e.target?.tagName || '').toLowerCase();
    const inInput = tag === 'input' || tag === 'textarea' || tag === 'select';
    if (e.code === 'Slash') {
      if (!inInput && currentView.value === 'main') {
        e.preventDefault();
        focusSearch();
      }
      return;
    }
    if (inInput && e.key !== 'Escape') return;

    if (e.key === 'Escape') {
      if (selectedBook.value) closeModal();
      else if (showHotkeysHelp.value) showHotkeysHelp.value = false;
      return;
    }
    if (currentView.value !== 'main' || isSearchLocked.value) return;

    if (e.key === 'ArrowLeft') {
      e.preventDefault();
      navigateBook(-1);
      return;
    }
    if (e.key === 'ArrowRight') {
      e.preventDefault();
      navigateBook(1);
      return;
    }
    if (e.code === 'KeyF') {
      if (selectedBook.value && !inInput) {
        e.preventDefault();
        toggleFavorite(selectedBook.value);
      }
      return;
    }
    if (e.code === 'KeyD') {
      if (selectedBook.value && !inInput) {
        e.preventDefault();
        window.location.href = `/download/${selectedBook.value.id}`;
      }
      return;
    }
    if (e.code === 'KeyR') {
      if (selectedBook.value && readerSettings.value.enabled && readBookUrl.value !== '#' && !inInput) {
        e.preventDefault();
        window.open(readBookUrl.value, '_blank');
      }
      return;
    }
    if (e.key === ' ') {
      if (!selectedBook.value && flatBooks.value.length > 0 && !inInput) {
        e.preventDefault();
        selectedBook.value = flatBooks.value[0];
      }
      return;
    }
  };
  window.addEventListener('keydown', onKeydown);

  const parseInterval = setInterval(fetchParseStatus, 2000);

  const handleScroll = () => {
    showScrollTop.value = window.scrollY > 300;
  };
  window.addEventListener('scroll', handleScroll);

  theme.value = localStorage.getItem('theme') || 'light';

  return () => {
    window.removeEventListener('scroll', handleScroll);
    window.removeEventListener('keydown', onKeydown);
    clearInterval(parseInterval);
    if (genreTimeout) clearTimeout(genreTimeout);
    if (imageObserver) imageObserver.disconnect();
  };
});

watch(theme, (newTheme) => {
  document.documentElement.setAttribute('data-theme', newTheme);
  localStorage.setItem('theme', newTheme);
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
</script>

<template>
  <div class="min-h-screen transition-colors duration-300 overflow-x-hidden"
       :class="theme === 'dark' ? 'bg-gray-900 text-gray-100' : 'bg-gradient-to-br from-slate-100 to-slate-200 text-slate-800'">

    <div v-if="currentView === 'main'">
      <!-- Header -->
      <header class="sticky top-0 z-40 backdrop-blur-md border-b transition-colors"
              :class="theme === 'dark' ? 'bg-gray-800/80 border-gray-700' : 'bg-slate-100/80 border-slate-300'">
        <div class="max-w-7xl mx-auto px-4 py-3 flex items-center justify-between">
          <div class="flex items-center space-x-2">
            <div class="p-2 rounded-lg" :class="theme === 'dark' ? 'bg-indigo-600' : 'bg-slate-300'">
              <BookOpen class="h-5 w-5" :class="theme === 'dark' ? 'text-white' : 'text-slate-700'" />
            </div>
            <h1 class="text-xl font-bold bg-gradient-to-r from-indigo-600 to-purple-600 bg-clip-text text-transparent">
              Моя Библиотека
            </h1>
          </div>
          <div class="flex items-center space-x-2">
            <button @click="toggleTheme" class="p-2 rounded-md transition-colors"
                    :class="theme === 'dark' ? 'bg-gray-700 text-yellow-400 hover:bg-gray-600' : 'bg-slate-300 text-slate-700 hover:bg-slate-400'">
              <Sun v-if="theme === 'dark'" class="h-4 w-4" />
              <Moon v-else class="h-4 w-4" />
            </button>
            <button v-if="showSettingsButton" @click="openSettings" class="p-2 rounded-md transition-colors"
                    :class="theme === 'dark' ? 'text-gray-300 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-300'">
              <Settings class="h-5 w-5" />
            </button>
            <button v-if="currentUser" @click="openFavorites" class="flex items-center gap-1.5 px-2 py-2 rounded-md text-sm transition-colors"
                    :class="theme === 'dark' ? 'text-gray-300 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-300'">
              <Heart class="h-4 w-4" />
              <span class="hidden sm:inline">Книжная полка</span>
            </button>
            <button @click="showHotkeysHelp = !showHotkeysHelp" class="hidden sm:flex p-2 rounded-md transition-colors"
                    :class="theme === 'dark' ? 'text-gray-400 hover:bg-gray-700' : 'text-slate-500 hover:bg-slate-300'">
              <Keyboard class="h-5 w-5" />
            </button>
            <button v-if="!currentUser" @click="openAuthModal('login')" class="p-2 rounded-md transition-colors"
                    :class="theme === 'dark' ? 'text-gray-300 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-300'">
              <User class="h-5 w-5" />
            </button>
            <template v-else>
              <button @click="openProfileModal" class="px-3 py-2 rounded-md text-sm font-medium transition-colors"
                      :class="theme === 'dark' ? 'text-gray-300 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-300'">
                {{ currentUser.username }}
              </button>
              <button @click="handleLogout" class="p-2 rounded-md transition-colors"
                      :class="theme === 'dark' ? 'text-red-400 hover:bg-gray-700' : 'text-red-600 hover:bg-slate-300'">
                <LogOut class="h-5 w-5" />
              </button>
            </template>
          </div>
        </div>
      </header>

      <!-- Search Form -->
      <section class="py-5 px-4">
        <div class="max-w-5xl mx-auto">
          <form @submit.prevent="handleSearch" class="space-y-4" :class="isSearchLocked ? 'opacity-60 pointer-events-none select-none' : ''">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div>
                <label for="title" class="block text-sm font-medium mb-1"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Название</label>
                <div class="relative">
                  <input ref="searchInputRef" v-model="searchTitle" :disabled="isSearchLocked" type="text"
                         class="w-full px-3 py-2.5 pr-10 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                         :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'" />
                  <button v-if="searchTitle && !isSearchLocked"
                          type="button"
                          @click="clearSearchField('title')"
                          class="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded-md transition-colors"
                          :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'">
                    <X class="h-4 w-4" />
                  </button>
                </div>
              </div>
              <div>
                <label for="author" class="block text-sm font-medium mb-1"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Автор</label>
                <div class="relative">
                  <input v-model="searchAuthor" :disabled="isSearchLocked" type="text"
                         class="w-full px-3 py-2.5 pr-10 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                         :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'" />
                  <button v-if="searchAuthor && !isSearchLocked"
                          type="button"
                          @click="clearSearchField('author')"
                          class="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded-md transition-colors"
                          :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'">
                    <X class="h-4 w-4" />
                  </button>
                </div>
              </div>
              <div class="relative">
                <label for="genre" class="block text-sm font-medium mb-1"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Жанр</label>
                <input v-model="searchGenre"
                       @input="updateGenreSuggestions"
                       @focus="updateGenreSuggestions"
                       @blur="hideGenreSuggestions"
                       :disabled="isSearchLocked"
                       type="text"
                       class="w-full px-3 py-2.5 pr-10 rounded-lg border focus:outline-none focus:ring-2 transition-colors z-10 relative"
                       :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'" />
                <button v-if="searchGenre && !isSearchLocked"
                        type="button"
                        @click="clearSearchField('genre')"
                        class="absolute right-2 top-[30px] p-1 rounded-md transition-colors z-30"
                        :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'">
                  <X class="h-4 w-4" />
                </button>
                <div v-show="showGenreSuggestions"
                     class="absolute z-20 mt-1 w-full rounded-md shadow-lg max-h-60 overflow-auto"
                     :class="theme === 'dark' ? 'bg-gray-800 border border-gray-700' : 'bg-slate-100 border border-slate-300'">
                  <ul class="py-1">
                    <li v-for="suggestion in filteredGenreSuggestions"
                        :key="suggestion.code"
                        @click="selectGenreSuggestion(suggestion)"
                        class="px-4 py-2 text-sm cursor-pointer hover:opacity-90 transition-opacity"
                        :class="theme === 'dark' ? 'text-gray-200 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-200'">
                      {{ suggestion.name }}
                    </li>
                  </ul>
                </div>
              </div>
              <div>
                <label for="series" class="block text-sm font-medium mb-1"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Серия</label>
                <div class="relative">
                  <input v-model="searchSeries" :disabled="isSearchLocked" type="text"
                         class="w-full px-3 py-2.5 pr-10 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                         :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'" />
                  <button v-if="searchSeries && !isSearchLocked"
                          type="button"
                          @click="clearSearchField('series')"
                          class="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded-md transition-colors"
                          :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'">
                    <X class="h-4 w-4" />
                  </button>
                </div>
              </div>
            </div>
            <div class="flex justify-center">
              <div class="w-full max-w-md">
                <label class="block text-sm font-medium mb-2 text-center"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Язык книги</label>
                <select v-model="selectedLanguage"
                        @change="languageManuallyChanged = true"
                        :disabled="isSearchLocked"
                        class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                        :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'">
                  <option v-for="lang in availableLanguages" :key="lang" :value="lang">
                    {{ lang === 'Все языки' ? 'Все языки' : lang }}
                  </option>
                </select>
              </div>
            </div>
            <div class="text-center">
              <button type="submit" :disabled="isSearchLocked"
                      class="inline-flex items-center gap-2 px-6 py-2.5 rounded-lg font-medium bg-indigo-600 text-white transition-colors"
                      :class="isSearchLocked ? 'opacity-70 cursor-not-allowed' : 'hover:bg-indigo-700'">
                <Search class="h-4 w-4" /> {{ parseStatus.isParsing ? 'БД обновляется…' : 'Найти' }}
              </button>
            </div>
          </form>
        </div>
      </section>

      <!-- ИНДИКАТОР ПАРСИНГА -->
      <div v-if="parseStatus.isParsing" class="mt-2 mb-4">
        <div class="max-w-7xl mx-auto flex flex-col items-center gap-2 px-4">
          <div class="inline-flex items-center gap-2 w-full justify-center">
            <div class="animate-spin rounded-full h-5 w-5 border-2"
                 :class="theme === 'dark' ? 'border-indigo-500 border-t-transparent' : 'border-indigo-600 border-t-transparent'"></div>
            <span class="text-sm break-words"
                  :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-600'">
              {{ parseStatus.message || 'Подождите. Выполняется парсинг' }}
              <span v-if="parseStatus.currentFile" class="ml-1 opacity-80 break-all">
                — {{ parseStatus.currentFile }}
              </span>
              <span v-if="parseProgressPercent != null" class="ml-1 font-medium">
                · {{ parseProgressPercent }}%
              </span>
              <span v-if="parseEstimatedTime" class="ml-1 opacity-80">
                · осталось {{ parseEstimatedTime }}
              </span>
            </span>
          </div>
          <div v-if="parseProgressPercent != null" class="w-full max-w-md rounded-full h-1.5 overflow-hidden">
            <div class="bg-indigo-500 h-1.5 rounded-full transition-all duration-300"
                 :style="{ width: parseProgressPercent + '%' }"></div>
          </div>
        </div>
      </div>

      <!-- Main Content -->
      <main class="px-4 pb-24">
        <div class="max-w-7xl mx-auto">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold"
                :class="theme === 'dark' ? 'text-gray-200' : 'text-slate-700'">
              Найдено: {{ filteredBooks.length }}
              {{ filteredBooks.length === 1 ? 'книга' : (filteredBooks.length % 10 > 1 && filteredBooks.length % 10 < 5 && (filteredBooks.length < 10 || filteredBooks.length > 20)) ? 'книги' : 'книг' }}
            </h2>
          </div>

          <div v-if="loading" class="text-center py-12">
            <div class="inline-flex flex-col items-center gap-3">
              <div class="animate-spin rounded-full h-8 w-8 border-2"
                   :class="theme === 'dark' ? 'border-indigo-500 border-t-transparent' : 'border-indigo-600 border-t-transparent'"></div>
              <span :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-600'">Поиск книг...</span>
            </div>
          </div>

          <div v-else-if="filteredBooks.length === 0" class="text-center py-12">
            <BookOpen class="mx-auto h-12 w-12 mb-3"
                     :class="theme === 'dark' ? 'text-gray-600' : 'text-slate-500'" />
            <p :class="theme === 'dark' ? 'text-gray-400' : 'text-slate-600'">Книги не найдены</p>
          </div>

          <div v-else class="space-y-4">
            <div v-for="authorGroup in groupedBooks" :key="authorGroup.author"
                 class="rounded-xl border overflow-hidden transition-all duration-300"
                 :class="theme === 'dark' ? 'bg-gray-800/50 border-gray-700' : 'bg-white border-slate-200'">
              <!-- Заголовок Автора -->
              <button @click="toggleAuthor(authorGroup.author)"
                      class="w-full flex items-center justify-between p-4 text-left transition-colors hover:bg-opacity-80"
                      :class="theme === 'dark' ? 'hover:bg-gray-700' : 'hover:bg-slate-50'">
                <div class="flex items-center gap-3 w-full">
                  <component :is="expandedAuthors.has(authorGroup.author) ? ChevronDown : ChevronRight"
                             class="h-5 w-5 flex-shrink-0"
                             :class="theme === 'dark' ? 'text-indigo-400' : 'text-indigo-600'"/>
                  <div class="flex-grow min-w-0">
                    <h3 class="font-bold text-lg break-words"
                        :class="theme === 'dark' ? 'text-gray-100' : 'text-slate-800'">
                      {{ authorGroup.author }}
                    </h3>
                    <span class="text-xs font-medium px-2 py-0.5 rounded-full"
                          :class="theme === 'dark' ? 'bg-gray-700 text-gray-300' : 'bg-slate-100 text-slate-600'">
                      Книг: {{ authorGroup.seriesGroups.reduce((acc, sg) => acc + sg.books.length, 0) }}
                    </span>
                  </div>
                </div>
              </button>

              <!-- Контент автора (группы серий) -->
              <div v-show="expandedAuthors.has(authorGroup.author)"
                   class="p-0 border-t transition-all duration-300"
                   :class="theme === 'dark' ? 'border-gray-700 bg-gray-900/30' : 'border-slate-100 bg-slate-50/50'">
                <div v-for="seriesGroup in authorGroup.seriesGroups" :key="`${authorGroup.author}_${seriesGroup.series}`"
                     class="border-b last:border-b-0"
                     :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-100'">
                  <!-- Заголовок Серии -->
                  <button @click="toggleSeries(authorGroup.author, seriesGroup.series)"
                          class="w-full flex items-center justify-between p-4 pl-8 text-left transition-colors hover:bg-opacity-80"
                          :class="theme === 'dark' ? 'hover:bg-gray-700' : 'hover:bg-slate-50'">
                    <div class="flex items-center gap-3 w-full">
                      <component :is="expandedSeries.has(`${authorGroup.author}_${seriesGroup.series}`) ? ChevronDown : ChevronRight"
                                 class="h-5 w-5 flex-shrink-0"
                                 :class="theme === 'dark' ? 'text-indigo-400' : 'text-indigo-600'"/>
                      <div class="flex-grow min-w-0">
                        <h4 class="font-semibold break-words"
                            :class="theme === 'dark' ? 'text-gray-200' : 'text-slate-700'">
                          {{ seriesGroup.series }}
                        </h4>
                        <span class="text-xs font-medium px-2 py-0.5 rounded-full"
                              :class="theme === 'dark' ? 'bg-gray-600 text-gray-300' : 'bg-slate-200 text-slate-600'">
                          Книг: {{ seriesGroup.books.length }}
                        </span>
                      </div>
                    </div>
                  </button>

                  <!-- Контент серии (книги) -->
                  <div v-show="expandedSeries.has(`${authorGroup.author}_${seriesGroup.series}`)"
                       class="p-4 pl-12">
                    <div v-if="viewMode === 'grid'"
                         class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
                      <div v-for="book in seriesGroup.books" :key="book.id"
                           @click="selectedBook = book"
                           class="group cursor-pointer rounded-xl transition-all duration-200 hover:scale-[1.02] flex flex-col border"
                           :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 shadow-md hover:shadow-lg' : 'bg-slate-100 border-slate-300 shadow hover:shadow-md'"
                           style="height: 260px">
                        <div class="h-32 w-full overflow-hidden rounded-t-xl relative book-cover-placeholder"
                             :data-img-id="getImageId(book)">
                          <!-- Плейсхолдер пока изображение не в видимой области -->
                          <div v-if="!shouldLoadImage(book) || hasImageError(book) || !getCoverUrl(book)"
                               class="w-full h-full flex items-center justify-center rounded-t-xl"
                               :class="theme === 'dark' ? 'bg-gray-700' : 'bg-slate-200'">
                            <BookOpen class="h-6 w-6"
                                     :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'" />
                          </div>
                          <!-- Реальное изображение загружается только когда видимо -->
                          <img v-else
                               :src="getCoverUrl(book)"
                               :alt="book.title"
                               class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-200"
                               @error="(e) => handleImageError(e, book)"
                               @load="(e) => handleImageLoad(e, book)"
                               loading="lazy" />
                          <button v-if="currentUser" @click.stop="toggleFavorite(book)"
                                  class="absolute top-1.5 right-1.5 p-1 rounded-full transition-colors"
                                  :class="favoriteIds.has(book.id) ? 'text-red-500 bg-white/90' : 'text-gray-400 bg-white/70 hover:bg-white/90'">
                            <Heart class="h-3.5 w-3.5" :fill="favoriteIds.has(book.id) ? 'currentColor' : 'none'" />
                          </button>
                        </div>
                        <div class="p-3 flex flex-col flex-grow">
                          <h3 class="font-semibold text-sm mb-1 line-clamp-2 leading-tight break-words"
                              :class="theme === 'dark' ? 'text-gray-100' : 'text-slate-800'">
                            {{ book.title }}
                          </h3>
                          <p class="text-xs mb-2 line-clamp-1 break-words"
                             :class="theme === 'dark' ? 'text-gray-400' : 'text-slate-600'">
                            {{ getGenreNames(book.genre) }}
                          </p>
                          <div class="mt-auto space-y-1">
                            <div class="flex justify-between text-xs">
                              <span :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'">Формат:</span>
                              <span class="font-medium uppercase"
                                    :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">{{ book.format }}</span>
                            </div>
                            <div class="flex justify-between text-xs">
                              <span :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'">Язык:</span>
                              <span class="font-medium"
                                    :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">{{ getLanguageName(book.language) }}</span>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>

                    <div v-else class="space-y-3">
                      <div v-for="book in seriesGroup.books" :key="book.id"
                           @click="selectedBook = book"
                           class="group cursor-pointer rounded-xl p-4 transition-all duration-200 flex items-start gap-4 border"
                           :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 hover:bg-gray-750 shadow' : 'bg-slate-100 border-slate-300 hover:bg-slate-200 shadow'">
                        <div class="flex-shrink-0 w-16 h-24 relative book-cover-placeholder"
                             :data-img-id="getImageId(book)">
                          <!-- Плейсхолдер -->
                          <div v-if="!shouldLoadImage(book) || hasImageError(book) || !getCoverUrl(book)"
                               class="w-full h-full flex items-center justify-center rounded-lg"
                               :class="theme === 'dark' ? 'bg-gray-700' : 'bg-slate-200'">
                            <BookOpen class="h-5 w-5"
                                     :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'" />
                          </div>
                          <!-- Реальное изображение -->
                          <img v-else
                               :src="getCoverUrl(book)"
                               :alt="book.title"
                               class="w-full h-full object-cover rounded-lg"
                               @error="(e) => handleImageError(e, book)"
                               @load="(e) => handleImageLoad(e, book)"
                               loading="lazy" />
                          <button v-if="currentUser" @click.stop="toggleFavorite(book)"
                                  class="absolute top-0.5 right-0.5 p-0.5 rounded"
                                  :class="favoriteIds.has(book.id) ? 'text-red-500' : (theme === 'dark' ? 'text-gray-400' : 'text-slate-500')">
                            <Heart class="h-3.5 w-3.5" :fill="favoriteIds.has(book.id) ? 'currentColor' : 'none'" />
                          </button>
                        </div>
                        <div class="flex-grow min-w-0">
                          <h3 class="font-semibold text-base mb-1 line-clamp-2 break-words"
                              :class="theme === 'dark' ? 'text-gray-100' : 'text-slate-800'">
                            {{ book.title }}
                          </h3>
                          <div class="text-sm space-y-1"
                               :class="theme === 'dark' ? 'text-gray-400' : 'text-slate-600'">
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
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </main>

      <!-- Горячие клавиши (подсказка) -->
      <Teleport to="body">
        <div v-if="showHotkeysHelp" class="fixed inset-0 z-[70] flex items-start justify-end p-4 pt-20"
             @click.self="showHotkeysHelp = false">
          <div class="rounded-xl shadow-xl border p-4 max-w-xs text-sm"
               :class="theme === 'dark' ? 'bg-gray-800 border-gray-600' : 'bg-white border-slate-200'">
            <div class="flex justify-between items-center mb-3">
              <span class="font-semibold" :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">Горячие клавиши</span>
              <button @click="showHotkeysHelp = false" class="p-1 rounded" :class="theme === 'dark' ? 'hover:bg-gray-700' : 'hover:bg-slate-100'">
                <X class="h-4 w-4" />
              </button>
            </div>
            <ul class="space-y-2" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-600'">
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">/</kbd> — фокус на поиск</li>
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">Esc</kbd> — закрыть модалку</li>
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">←</kbd><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs ml-0.5">→</kbd> — листать книги</li>
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">F</kbd> — в избранное</li>
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">D</kbd> — скачать</li>
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">R</kbd> — открыть читалку</li>
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">Space</kbd> — быстрый просмотр</li>
            </ul>
          </div>
        </div>
      </Teleport>

      <!-- Scroll to Top Button -->
      <button v-if="showScrollTop" @click="scrollToTop"
              class="fixed bottom-6 right-6 p-3 rounded-full shadow-lg transition-all hover:scale-110 bg-indigo-600 text-white">
        <ArrowUp class="h-5 w-5" />
      </button>
    </div>

    <!-- Favorites (Книжная полка) -->
    <div v-else-if="currentView === 'favorites'" class="min-h-screen p-4 overflow-x-hidden"
         :class="theme === 'dark' ? 'bg-gray-900' : 'bg-gradient-to-br from-slate-100 to-slate-200'">
      <header class="max-w-7xl mx-auto flex items-center justify-between mb-6">
        <h2 class="text-xl font-bold break-words" :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">Книжная полка</h2>
        <button @click="closeFavorites" class="px-4 py-2 rounded-lg font-medium transition-colors"
                :class="theme === 'dark' ? 'bg-gray-700 hover:bg-gray-600 text-white' : 'bg-slate-200 hover:bg-slate-300 text-slate-800'">
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
          <div v-for="book in favoritesBooks" :key="book.id"
               @click="selectedBook = book"
               class="group cursor-pointer rounded-xl transition-all duration-200 hover:scale-[1.02] flex flex-col border"
               :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 shadow-md hover:shadow-lg' : 'bg-slate-100 border-slate-300 shadow hover:shadow-md'"
               style="height: 260px">
            <div class="h-32 w-full overflow-hidden rounded-t-xl relative book-cover-placeholder"
                 :data-img-id="getImageId(book)">
              <!-- Плейсхолдер -->
              <div v-if="!shouldLoadImage(book) || hasImageError(book) || !getCoverUrl(book)"
                   class="w-full h-full flex items-center justify-center rounded-t-xl"
                   :class="theme === 'dark' ? 'bg-gray-700' : 'bg-slate-200'">
                <BookOpen class="h-6 w-6"
                         :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'" />
              </div>
              <!-- Реальное изображение -->
              <img v-else
                   :src="getCoverUrl(book)"
                   :alt="book.title"
                   class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-200"
                   @error="(e) => handleImageError(e, book)"
                   @load="(e) => handleImageLoad(e, book)"
                   loading="lazy" />
              <button @click.stop="toggleFavorite(book)"
                      class="absolute top-2 right-2 p-1.5 rounded-full transition-colors"
                      :class="favoriteIds.has(book.id) ? 'text-red-500 bg-white/90' : 'text-gray-400 bg-white/70 hover:bg-white/90'">
                <Heart class="h-4 w-4" :fill="favoriteIds.has(book.id) ? 'currentColor' : 'none'" />
              </button>
            </div>
            <div class="p-3 flex flex-col flex-grow">
              <h3 class="font-semibold text-sm mb-1 line-clamp-2 leading-tight break-words"
                  :class="theme === 'dark' ? 'text-gray-100' : 'text-slate-800'">
                {{ book.title }}
              </h3>
              <p class="text-xs mb-2 line-clamp-1 break-words"
                 :class="theme === 'dark' ? 'text-gray-400' : 'text-slate-600'">
                {{ getGenreNames(book.genre) }}
              </p>
              <div class="mt-auto space-y-1">
                <div class="flex justify-between text-xs">
                  <span :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'">Формат:</span>
                  <span class="font-medium uppercase"
                        :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">{{ book.format }}</span>
                </div>
                <div class="flex justify-between text-xs">
                  <span :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'">Язык:</span>
                  <span class="font-medium"
                        :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">{{ getLanguageName(book.language) }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Settings View -->
    <div v-else-if="currentView === 'settings'" class="overflow-x-hidden">
      <SettingsView @close="closeSettings" @config-saved="fetchReaderSettings" :token="token" />
    </div>

    <!-- Book Details Modal -->
    <Teleport to="body">
      <div v-if="selectedBook"
           class="fixed inset-0 z-50 flex items-center justify-center p-2 md:p-4 bg-black/60 backdrop-blur-sm overflow-hidden"
           @click="closeModal">
        <div class="relative w-full max-w-4xl max-h-[90vh] overflow-hidden rounded-xl shadow-2xl border flex flex-col"
             :class="theme === 'dark' ? 'bg-gray-800 border-gray-700' : 'bg-slate-50 border-slate-300'"
             @click.stop>
          <div class="flex justify-between items-start p-3 sm:p-4 border-b"
               :class="theme === 'dark' ? 'border-gray-700 bg-gray-800' : 'border-slate-200 bg-white'">
            <div class="pr-4">
              <h2 class="text-lg sm:text-xl font-bold leading-tight break-words"
                  :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">
                {{ selectedBook.title }}
              </h2>
              <p class="text-sm mt-1 break-words"
                 :class="theme === 'dark' ? 'text-indigo-400' : 'text-indigo-600'">
                {{ selectedBook.author }}
              </p>
            </div>
            <div class="flex items-center gap-1">
              <button v-if="currentUser" @click="toggleFavorite(selectedBook)"
                      class="p-1.5 sm:p-2 rounded-full transition-colors"
                      :class="favoriteIds.has(selectedBook.id) ? 'text-red-500' : (theme === 'dark' ? 'text-gray-400 hover:bg-gray-700' : 'text-slate-500 hover:bg-slate-200')">
                <Heart class="h-4 sm:h-5 w-4 sm:w-5" :fill="favoriteIds.has(selectedBook.id) ? 'currentColor' : 'none'" />
              </button>
              <button @click="closeModal"
                      class="p-1.5 sm:p-2 rounded-full transition-colors flex-shrink-0"
                      :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'">
                <X class="h-5 sm:h-6 w-5 sm:w-6" />
              </button>
            </div>
          </div>
          <div class="overflow-y-auto overflow-x-hidden p-0 flex-grow">
            <div class="flex flex-col md:flex-row">
              <div class="md:w-1/3 p-3 sm:p-5 flex flex-col items-center border-b md:border-b-0 md:border-r"
                   :class="theme === 'dark' ? 'border-gray-700 bg-gray-800/50' : 'border-slate-200 bg-white'">
                <div class="w-full max-w-[200px] mb-4 shadow-lg rounded-lg overflow-hidden relative group">
                  <!-- В модалке загружаем сразу, т.к. она уже открыта -->
                  <img v-if="getCoverUrl(selectedBook) && !hasImageError(selectedBook)"
                       :src="getCoverUrl(selectedBook)"
                       :alt="selectedBook.title"
                       class="w-full h-auto object-cover"
                       @error="(e) => handleImageError(e, selectedBook)"
                       @load="(e) => handleImageLoad(e, selectedBook)" />
                  <div v-else class="w-full aspect-[2/3] flex items-center justify-center"
                       :class="theme === 'dark' ? 'bg-gray-700' : 'bg-slate-200'">
                    <BookOpen class="h-12 w-12"
                             :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'" />
                  </div>
                </div>
                <a :href="`/download/${selectedBook.id}`"
                   class="w-full flex items-center justify-center gap-2 py-3 px-4 rounded-lg font-bold shadow-lg transform transition hover:scale-105 mb-3"
                   :class="theme === 'dark' ? 'bg-indigo-600 hover:bg-indigo-500 text-white' : 'bg-indigo-600 hover:bg-indigo-700 text-white'">
                  <Download class="h-5 w-5" /> Скачать ({{ selectedBook.format.toUpperCase() }})
                </a>
                <a v-if="readerSettings.enabled && readerSettings.url"
                   :href="readBookUrl"
                   target="_blank"
                   class="w-full flex items-center justify-center gap-2 py-2 px-4 rounded-lg font-bold shadow-md transform transition hover:scale-105 mb-6 border"
                   :class="theme === 'dark' ? 'border-indigo-500 text-indigo-400 hover:bg-gray-700' : 'border-indigo-600 text-indigo-700 hover:bg-indigo-50'">
                  <BookOpen class="h-5 w-5" /> Читать
                </a>
                <div class="w-full space-y-3 text-sm"
                     :class="theme === 'dark' ? 'text-gray-200' : 'text-slate-800'">
                  <div class="flex justify-between border-b pb-2" :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-200'">
                    <span class="opacity-70 flex-shrink-0">Серия:</span>
                    <span class="font-medium text-right break-words ml-2">
                      {{ selectedBook.series || '—' }}
                      <span v-if="selectedBook.seriesNo" class="bg-indigo-100 text-indigo-800 text-xs px-1.5 py-0.5 rounded ml-1">#{{ selectedBook.seriesNo }}</span>
                    </span>
                  </div>
                  <div class="flex justify-between border-b pb-2" :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-200'">
                    <span class="opacity-70 flex-shrink-0">Жанр:</span>
                    <span class="font-medium text-right break-words ml-2">{{ getGenreNames(selectedBook.genre) }}</span>
                  </div>
                  <div class="flex justify-between border-b pb-2" :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-200'">
                    <span class="opacity-70 flex-shrink-0">Язык:</span>
                    <span class="font-medium text-right break-words ml-2">{{ getLanguageName(selectedBook.language) }}</span>
                  </div>
                  <div class="flex justify-between border-b pb-2" :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-200'">
                    <span class="opacity-70 flex-shrink-0">Размер:</span>
                    <span class="font-medium text-right break-words ml-2">{{ formatFileSize(selectedBook.fileSize) }}</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="opacity-70 flex-shrink-0">Добавлено:</span>
                    <span class="font-medium text-right break-words ml-2">{{ new Date(selectedBook.addedAt).toLocaleDateString() }}</span>
                  </div>
                </div>
              </div>
              <div class="md:w-2/3 p-3 sm:p-5 md:p-6"
                   :class="theme === 'dark' ? 'bg-gray-900/50' : 'bg-slate-50'">
                <div v-if="loadingDetails" class="flex flex-col items-center justify-center h-40 space-y-3">
                  <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-500"></div>
                  <span class="text-sm opacity-70">Распаковка описания...</span>
                </div>
                <div v-else>
                  <div v-if="bookDetails && bookDetails.titleInfo && bookDetails.titleInfo.annotationHtml" class="mb-8">
                    <h3 class="text-lg font-bold mb-3 flex items-center gap-2"
                        :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">
                      <span class="w-1 h-6 bg-indigo-500 rounded-full"></span>
                      Аннотация
                    </h3>
                    <div class="prose max-w-none text-sm leading-relaxed book-annotation"
                         :class="theme === 'dark' ? 'prose-invert text-gray-300' : 'text-slate-700'"
                         v-html="bookDetails.titleInfo.annotationHtml">
                    </div>
                  </div>
                  <div v-else-if="(selectedBook.format === 'fb2' || selectedBook.format === 'epub') && !loadingDetails" class="mb-8 text-center py-4 opacity-50 italic">
                    Описание отсутствует
                  </div>
                  <div v-else-if="selectedBook.format !== 'fb2' && selectedBook.format !== 'epub'" class="mb-8 p-4 rounded-lg border border-dashed text-center text-sm opacity-70"
                       :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-300'">
                    Детальное описание доступно только для FB2 и EPUB файлов
                  </div>
                  <div v-if="bookDetails" class="space-y-6">
                    <div v-if="bookDetails.publishInfo && (bookDetails.publishInfo.publisher || bookDetails.publishInfo.year)">
                      <h3 class="text-md font-bold mb-2 opacity-80 uppercase text-xs tracking-wider">Информация об издании</h3>
                      <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-2 text-sm rounded-lg p-3"
                           :class="theme === 'dark' ? 'bg-gray-800' : 'bg-white border'">
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
                      <div class="text-sm rounded-lg p-3"
                           :class="theme === 'dark' ? 'bg-gray-800' : 'bg-white border'">
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
    </Teleport>

    <!-- Modals -->
    <Teleport to="body">
      <!-- Setup Modal -->
      <div v-if="showSetupModal"
           class="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
        <div class="relative w-full max-w-md rounded-xl shadow-2xl border bg-gray-800 border-gray-700 p-6" @click.stop>
          <h2 class="text-xl font-bold mb-4 text-center text-white">Первоначальная настройка</h2>
          <p class="text-gray-300 text-sm mb-4 text-center">Создайте учётную запись администратора</p>
          <form @submit.prevent="handleSetup" class="space-y-4">
            <div>
              <label class="block text-sm font-medium mb-1 text-gray-300">Имя пользователя</label>
              <input v-model="authForm.username" type="text" required
                     class="w-full px-3 py-2.5 rounded-lg border bg-gray-700 border-gray-600 text-white focus:ring-indigo-500" />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1 text-gray-300">Пароль (мин. 3 символа)</label>
              <input v-model="authForm.password" type="password" required
                     class="w-full px-3 py-2.5 rounded-lg border bg-gray-700 border-gray-600 text-white focus:ring-indigo-500" />
            </div>
            <div v-if="authError" class="text-red-400 text-sm text-center">{{ authError }}</div>
            <button type="submit"
                    class="w-full py-2.5 px-4 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white">
              Создать администратора
            </button>
          </form>
        </div>
      </div>

      <!-- Profile Modal -->
      <div v-if="showProfileModal"
           class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm"
           @click="showProfileModal = false">
        <div class="relative w-full max-w-md rounded-xl shadow-2xl border p-6"
             :class="theme === 'dark' ? 'bg-gray-800 border-gray-700' : 'bg-slate-100 border-slate-300'"
             @click.stop>
          <h2 class="text-xl font-bold mb-4" :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">Мой профиль</h2>
          <form @submit.prevent="handleUpdateProfile" class="space-y-4">
            <div>
              <label class="block text-sm font-medium mb-1" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Имя пользователя</label>
              <input v-model="profileForm.newUsername" type="text"
                     class="w-full px-3 py-2.5 rounded-lg border"
                     :class="theme === 'dark' ? 'bg-gray-700 border-gray-600 text-white' : 'bg-slate-100 border-slate-300 text-slate-800'" />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Новый пароль (оставьте пустым, если не меняете)</label>
              <input v-model="profileForm.newPassword" type="password"
                     class="w-full px-3 py-2.5 rounded-lg border"
                     :class="theme === 'dark' ? 'bg-gray-700 border-gray-600 text-white' : 'bg-slate-100 border-slate-300 text-slate-800'" />
            </div>
            <div v-if="profileForm.newPassword">
              <label class="block text-sm font-medium mb-1" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Текущий пароль</label>
              <input v-model="profileForm.oldPassword" type="password"
                     class="w-full px-3 py-2.5 rounded-lg border"
                     :class="theme === 'dark' ? 'bg-gray-700 border-gray-600 text-white' : 'bg-slate-100 border-slate-300 text-slate-800'" />
            </div>
            <div v-if="profileForm.newPassword">
              <label class="block text-sm font-medium mb-1" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Подтверждение пароля</label>
              <input v-model="profileForm.confirmPassword" type="password"
                     class="w-full px-3 py-2.5 rounded-lg border"
                     :class="theme === 'dark' ? 'bg-gray-700 border-gray-600 text-white' : 'bg-slate-100 border-slate-300 text-slate-800'" />
            </div>
            <div v-if="profileError" class="text-red-500 text-sm">{{ profileError }}</div>
            <div v-if="profileSuccess" class="text-green-500 text-sm">{{ profileSuccess }}</div>
            <div class="flex gap-2">
              <button type="submit" class="px-4 py-2.5 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white">
                Сохранить
              </button>
              <button type="button" @click="showProfileModal = false"
                      class="px-4 py-2.5 rounded-lg font-medium border"
                      :class="theme === 'dark' ? 'border-gray-600 text-gray-300 hover:bg-gray-700' : 'border-slate-300 text-slate-700 hover:bg-slate-200'">
                Закрыть
              </button>
            </div>
          </form>
        </div>
      </div>

      <!-- Web Auth Modal -->
      <div v-if="showWebAuthModal"
           class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
        <div class="relative w-full max-w-md rounded-xl shadow-2xl border bg-gray-800 border-gray-700"
             @click.stop>
          <div class="p-6">
            <h2 class="text-xl font-bold mb-4 text-center text-white">
              Вход в систему
            </h2>
            <form @submit.prevent="handleWebAuth" class="space-y-4">
              <div>
                <label for="web-auth-password" class="block text-sm font-medium mb-1 text-gray-300">Пароль</label>
                <input id="web-auth-password"
                       v-model="webAuthPassword"
                       type="password"
                       required
                       class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors bg-gray-700 border-gray-600 text-white focus:ring-indigo-500"
                       placeholder="Введите пароль для доступа" />
              </div>
              <div v-if="webAuthError" class="text-red-400 text-sm text-center">
                {{ webAuthError }}
              </div>
              <button type="submit"
                      class="w-full py-2.5 px-4 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white transition-colors">
                Войти
              </button>
            </form>
          </div>
        </div>
      </div>

      <!-- Auth Modal -->
      <div v-if="showAuthModal"
           class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm"
           @click="closeAuthModal">
        <div class="relative w-full max-w-md rounded-xl shadow-2xl border"
             :class="theme === 'dark' ? 'bg-gray-800 border-gray-700' : 'bg-slate-100 border-slate-300'"
             @click.stop>
          <button @click="closeAuthModal"
                  class="absolute top-3 right-3 p-1.5 rounded-full transition-colors"
                  :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-300'">
            <X class="h-5 w-5" />
          </button>
          <div class="p-6">
            <h2 class="text-xl font-bold mb-4 text-center"
                :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">
              Вход
            </h2>
            <form @submit.prevent="handleAuth" class="space-y-4">
              <div>
                <label for="auth-username" class="block text-sm font-medium mb-1"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Имя пользователя</label>
                <input id="auth-username"
                       v-model="authForm.username"
                       type="text"
                       required
                       class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                       :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'" />
              </div>
              <div>
                <label for="auth-password" class="block text-sm font-medium mb-1"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Пароль</label>
                <input id="auth-password"
                       v-model="authForm.password"
                       type="password"
                       required
                       class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                       :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'" />
              </div>
              <div v-if="authError" class="text-red-500 text-sm text-center">
                {{ authError }}
              </div>
              <button type="submit"
                      class="w-full py-2.5 px-4 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white transition-colors">
                Войти
              </button>
            </form>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  overflow-wrap: break-word;
}
.line-clamp-1 {
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
  overflow-wrap: break-word;
}
.book-annotation {
  overflow-wrap: break-word;
  hyphens: auto;
}
.book-annotation :deep(p) {
  margin-bottom: 0.75em;
  text-align: justify;
}
.book-annotation :deep(strong), .book-annotation :deep(b) {
  font-weight: 700;
  color: inherit;
}
.book-annotation :deep(i), .book-annotation :deep(em) {
  font-style: italic;
}
</style>
<script setup>
import { ref, computed, onMounted, watch, nextTick } from 'vue';
import { Search, BookOpen, Download, X, Sun, Moon, ArrowUp, Settings, User, LogOut, ChevronRight, ChevronDown, Heart, Keyboard } from 'lucide-vue-next';
import SettingsView from './Settings.vue';
import { LANGUAGES_MAP } from './data/languages.js';
import { GENRES_MAP } from './data/genres.js';

// ===== STATE =====
const currentView = ref('main');
const searchTitle = ref("");
const searchAuthor = ref("");
const searchGenre = ref("");
const searchSeries = ref("");
const selectedLanguage = ref("Все языки");
const languageManuallyChanged = ref(false);
const availableLanguages = ref(["Все языки", ...Object.values(LANGUAGES_MAP).filter(lang => lang !== "Все языки").sort()]);
const theme = ref("light");
const viewMode = ref("grid");
const selectedBook = ref(null);
const showScrollTop = ref(false);
const books = ref([]);
const loading = ref(false);
const imageErrors = ref(new Set());
const visibleImages = ref(new Set()); // Для отслеживания видимых изображений

// ===== STATE ДЛЯ СТАТУСА ПАРСИНГА =====
const parseStatus = ref({
  isParsing: false,
  progress: 0,
  total: 0,
  message: "",
  estimatedRemainingSec: 0
});
const parseStatusInFlight = ref(false);

// ===== STATE ДЛЯ ДЕТАЛЕЙ КНИГИ =====
const bookDetails = ref(null);
const loadingDetails = ref(false);

// ===== STATE ДЛЯ АВТОДОПОЛНЕНИЯ ЖАНРА =====
const showGenreSuggestions = ref(false);
const filteredGenreSuggestions = ref([]);
let genreTimeout = null;

// ===== STATE ДЛЯ АВТОРИЗАЦИИ =====
const showAuthModal = ref(false);
const showSetupModal = ref(false);
const showProfileModal = ref(false);
const profileForm = ref({ oldPassword: '', newPassword: '', confirmPassword: '', newUsername: '' });
const profileError = ref('');
const profileSuccess = ref('');
const authMode = ref('login');
const authForm = ref({ username: '', password: '', confirmPassword: '' });
const authError = ref('');
const currentUser = ref(null);
const token = ref(localStorage.getItem('token') || null);

// ===== STATE ДЛЯ ВЕБ-АУТЕНТИФИКАЦИИ =====
const showWebAuthModal = ref(false);
const webAuthPassword = ref('');
const webAuthError = ref('');
const webPasswordRequired = ref(false);

// ===== STATE ДЛЯ ГРУППИРОВКИ АВТОРОВ =====
const expandedAuthors = ref(new Set());
const expandedSeries = ref(new Set());

// ===== STATE ДЛЯ ЧИТАЛКИ =====
const defaultReaderUrl = 'https://reader.example.com/#/read?url=';
const readerSettings = ref({
  enabled: false,
  url: defaultReaderUrl,
  defaultSearchLanguage: ''
});

// ===== ИЗБРАННОЕ =====
const favoriteIds = ref(new Set());
const favoritesBooks = ref([]);
const showHotkeysHelp = ref(false);
const searchInputRef = ref(null);

// ===== КЭШИРОВАНИЕ =====
const groupedBooksCache = ref({
  key: '',
  data: null
});

// ===== INTERSECTION OBSERVER =====
let imageObserver = null;

const setupImageObserver = () => {
  if (imageObserver) {
    imageObserver.disconnect();
  }
  
  imageObserver = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
      if (entry.isIntersecting) {
        const imgId = entry.target.dataset.imgId;
        if (imgId) {
          visibleImages.value.add(imgId);
          // Прекращаем наблюдать за этим элементом после загрузки
          imageObserver.unobserve(entry.target);
        }
      }
    });
  }, {
    root: null,
    rootMargin: '50px', // Начинаем загружать чуть раньше, чем элемент появится
    threshold: 0.1
  });
  
  // Наблюдаем за всеми placeholder'ами изображений
  nextTick(() => {
    const placeholders = document.querySelectorAll('.book-cover-placeholder');
    placeholders.forEach(placeholder => {
      imageObserver.observe(placeholder);
    });
  });
};

// ===== COMPUTED =====
const filteredBooks = computed(() => books.value);

const parseProgressPercent = computed(() => {
  const s = parseStatus.value;
  if (!s.total || s.total <= 0) return null;
  return Math.round((s.progress / s.total) * 100);
});

const parseEstimatedTime = computed(() => {
  const sec = parseStatus.value.estimatedRemainingSec;
  if (!sec || sec <= 0) return null;
  if (sec > 60 * 60 * 24) return null;
  if (sec < 60) return `~${sec} сек`;
  const min = Math.ceil(sec / 60);
  if (min < 60) return `~${min} мин`;
  const h = Math.floor(min / 60);
  const m = min % 60;
  return m > 0 ? `~${h} ч ${m} мин` : `~${h} ч`;
});

const isSearchLocked = computed(() => loading.value || parseStatus.value.isParsing);

const flatBooks = computed(() => {
  const list = [];
  groupedBooks.value.forEach(ag => {
    ag.seriesGroups.forEach(sg => {
      sg.books.forEach(b => list.push(b));
    });
  });
  return list;
});

const selectedBookIndex = computed(() => {
  if (!selectedBook.value || flatBooks.value.length === 0) return -1;
  return flatBooks.value.findIndex(b => b.id === selectedBook.value.id);
});

const isFavorite = (book) => book && favoriteIds.value.has(book.id);

const showSettingsButton = computed(() => {
  return currentUser.value && currentUser.value.role === 'admin';
});

const readBookUrl = computed(() => {
  if (!selectedBook.value || !readerSettings.value.enabled || !readerSettings.value.url) return '#';

  const safeTitle = sanitizeFilename(selectedBook.value.title);
  const safeAuthor = sanitizeFilename(selectedBook.value.author);
  const fileName = `${safeTitle} - ${safeAuthor}.${selectedBook.value.format}`;
  const downloadUri = `/download/${selectedBook.value.id}/${fileName}`;
  const baseUrl = window.location.origin || 'http://app.books-kriksa.ru';
  const fullDownloadUrl = `${baseUrl}${downloadUri}`;

  return `${readerSettings.value.url}${encodeURIComponent(fullDownloadUrl)}`;
});

const groupedBooks = computed(() => {
  const cacheKey = `${books.value.length}_${expandedAuthors.value.size}_${expandedSeries.value.size}_${JSON.stringify(books.value.slice(0, 3).map(b => b.id))}`;

  if (groupedBooksCache.value.key === cacheKey && groupedBooksCache.value.data) {
    return groupedBooksCache.value.data;
  }

  const groups = {};
  const authorsOrder = [];

  filteredBooks.value.forEach(book => {
    const authorName = book.author || 'Без автора';
    if (!groups[authorName]) {
      groups[authorName] = {};
      authorsOrder.push(authorName);
    }

    const seriesName = book.series || 'Без серии';
    if (!groups[authorName][seriesName]) {
      groups[authorName][seriesName] = [];
    }
    groups[authorName][seriesName].push(book);
  });

  const result = authorsOrder.map(author => {
    const seriesGroups = groups[author];
    const seriesEntries = Object.entries(seriesGroups);

    seriesEntries.sort(([seriesA], [seriesB]) => {
      if (seriesA === 'Без серии' && seriesB !== 'Без серии') return -1;
      if (seriesB === 'Без серии' && seriesA !== 'Без серии') return 1;
      return seriesA.localeCompare(seriesB);
    });

    const processedSeries = seriesEntries.map(([seriesName, booksArray]) => {
      const sortedBooks = booksArray.sort((a, b) => {
        if (a.seriesNo && b.seriesNo) {
          return a.seriesNo - b.seriesNo;
        }
        if (a.seriesNo) return -1;
        if (b.seriesNo) return 1;

        const isRuA = a.language === 'ru' || a.language === 'ru-';
        const isRuB = b.language === 'ru' || b.language === 'ru-';
        if (isRuA && !isRuB) return -1;
        if (!isRuA && isRuB) return 1;
        return a.title.localeCompare(b.title);
      });

      return {
        series: seriesName,
        books: sortedBooks
      };
    });

    return {
      author: author,
      seriesGroups: processedSeries
    };
  });

  groupedBooksCache.value = {
    key: cacheKey,
    data: result
  };

  return result;
});

// ===== МЕТОДЫ =====
const getLanguageName = (langCode) => LANGUAGES_MAP[langCode] || langCode;

const getGenreNames = (genreCodes) => {
  if (!genreCodes || typeof genreCodes !== 'string' || genreCodes.trim() === '') return '—';
  const codes = genreCodes.split(',').map(code => code.trim()).filter(Boolean);
  const names = codes.map(code => {
    const normalized = code.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '');
    return (GENRES_MAP && GENRES_MAP[normalized]) ? GENRES_MAP[normalized] : code;
  });
  return names.join(', ');
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
      .filter(suggestion => suggestion.name.toLowerCase().includes(inputValue))
      .sort((a, b) => {
        const posA = a.name.toLowerCase().indexOf(inputValue);
        const posB = b.name.toLowerCase().indexOf(inputValue);
        if (posA !== posB) return posA - posB;
        return a.name.localeCompare(b.name);
      });

    const uniqueSuggestionsMap = new Map();
    allSuggestions.forEach(s => {
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
  setTimeout(() => { showGenreSuggestions.value = false; }, 200);
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

const fetchWithAuth = (url, options = {}) => {
  const headers = { 'Content-Type': 'application/json', ...options.headers };
  if (token.value) headers['Authorization'] = `Bearer ${token.value}`;
  return fetch(url, { ...options, headers });
};

const toggleAuthor = (authorName) => {
  if (expandedAuthors.value.has(authorName)) expandedAuthors.value.delete(authorName);
  else expandedAuthors.value.add(authorName);
  groupedBooksCache.value = { key: '', data: null };
  // Перенастраиваем observer после изменения DOM
  nextTick(() => setupImageObserver());
};

const toggleSeries = (authorName, seriesName) => {
  const key = `${authorName}_${seriesName}`;
  if (expandedSeries.value.has(key)) expandedSeries.value.delete(key);
  else expandedSeries.value.add(key);
  groupedBooksCache.value = { key: '', data: null };
  // Перенастраиваем observer после изменения DOM
  nextTick(() => setupImageObserver());
};

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
    console.error("Не удалось загрузить настройки читалки", e);
  }
};

const fetchParseStatus = async () => {
  if (parseStatusInFlight.value) return;
  parseStatusInFlight.value = true;

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 1500);
  try {
    const res = await fetch('/api/app-status', {
      cache: 'no-store',
      signal: controller.signal
    });
    if (res.ok) {
      const data = await res.json();
      parseStatus.value = {
        isParsing: data.is_parsing || false,
        progress: Number(data.progress || 0),
        total: Number(data.total || 0),
        message: data.message || "",
        estimatedRemainingSec: data.estimated_remaining_sec || 0,
        currentFile: data.current_file || ""
      };
    }
  } catch (e) {
    if (e?.name !== 'AbortError') {
      console.error("Не удалось загрузить статус парсинга", e);
    }
  } finally {
    clearTimeout(timeoutId);
    parseStatusInFlight.value = false;
  }
};

const handleSearch = async (e) => {
  e.preventDefault();
  loading.value = true;
  imageErrors.value.clear();
  visibleImages.value.clear(); // Очищаем видимые изображения при новом поиске
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
      if (selectedLanguage.value !== "Все языки") {
        const byName = Object.keys(LANGUAGES_MAP).find(
          key => LANGUAGES_MAP[key] === selectedLanguage.value
        );
        if (byName) langCodeToUse = byName;
      }
    } else {
      if (readerSettings.value.defaultSearchLanguage) {
        langCodeToUse = readerSettings.value.defaultSearchLanguage;
      } else if (selectedLanguage.value !== "Все языки") {
        const byName = Object.keys(LANGUAGES_MAP).find(
          key => LANGUAGES_MAP[key] === selectedLanguage.value
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
        .filter(([code, name]) => name.trim().toLowerCase() === inputLower)
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
      return;
    }

    const data = await res.json();
    books.value = data.books || [];
    
    // Настраиваем observer после загрузки результатов
    nextTick(() => setupImageObserver());
  } catch (e) {
    console.error("Ошибка поиска", e);
    books.value = [];
  } finally {
    loading.value = false;
  }
};

const closeModal = () => {
  selectedBook.value = null;
  bookDetails.value = null;
};

const fetchFavorites = async () => {
  if (!token.value) return;
  try {
    const res = await fetchWithAuth('/api/favorites');
    if (res.ok) {
      const data = await res.json();
      favoriteIds.value = new Set((data.book_ids || []).map(Number));
    }
  } catch (e) { console.error('Ошибка загрузки избранного', e); }
};

const toggleFavorite = async (book) => {
  if (!book || !token.value) return;
  const id = book.id;
  const add = !favoriteIds.value.has(id);
  try {
    const url = '/api/favorites';
    const options = {
      method: add ? 'POST' : 'DELETE',
      headers: { 'Content-Type': 'application/json', ...(token.value ? { Authorization: `Bearer ${token.value}` } : {}) },
      body: JSON.stringify({ book_id: id })
    };
    const res = await fetchWithAuth(url, options);
    if (res.ok) {
      if (add) favoriteIds.value.add(id);
      else {
        favoriteIds.value.delete(id);
        if (currentView.value === 'favorites') {
          favoritesBooks.value = favoritesBooks.value.filter(b => b.id !== id);
        }
      }
      favoriteIds.value = new Set(favoriteIds.value);
    }
  } catch (e) { console.error('Ошибка избранного', e); }
};

const navigateBook = (delta) => {
  const list = flatBooks.value;
  if (list.length === 0) return;
  let idx = selectedBookIndex.value;
  if (idx < 0) idx = delta > 0 ? 0 : list.length - 1;
  else idx = (idx + delta + list.length) % list.length;
  selectedBook.value = list[idx];
};

const focusSearch = () => {
  searchInputRef.value?.focus();
};

const openFavorites = async () => {
  currentView.value = 'favorites';
  visibleImages.value.clear(); // Очищаем при переключении на избранное
  if (!token.value) return;
  try {
    const res = await fetchWithAuth('/api/favorites/books');
    if (res.ok) {
      const data = await res.json();
      favoritesBooks.value = data.books || [];
      // Настраиваем observer после загрузки избранного
      nextTick(() => setupImageObserver());
    }
  } catch (e) { favoritesBooks.value = []; }
};

const closeFavorites = () => {
  currentView.value = 'main';
  visibleImages.value.clear();
  nextTick(() => setupImageObserver());
};

const scrollToTop = () => { window.scrollTo({ top: 0, behavior: 'smooth' }); };

const toggleTheme = () => {
  theme.value = theme.value === 'dark' ? 'light' : 'dark';
};

const openSettings = () => {
  if (currentUser.value && currentUser.value.role === 'admin') currentView.value = 'settings';
};

const closeSettings = () => {
  currentView.value = 'main';
  fetchReaderSettings();
  nextTick(() => setupImageObserver());
};

const formatFileSize = (bytes) => (bytes / 1024 / 1024).toFixed(2) + ' МБ';

const getCoverUrl = (book) => {
  if (book.format === 'fb2' || book.format === 'epub') {
    return `/api/cover?file=${book.fileName}&zip=${book.zip}&format=${book.format}`;
  }
  return null;
};

const getImageId = (book) => `${book.id}-${book.fileName}-${book.zip}`;

const shouldLoadImage = (book) => visibleImages.value.has(getImageId(book));

const hasImageError = (book) => imageErrors.value.has(`${book.fileName}-${book.zip}`);

const handleImageError = (event, book) => {
 imageErrors.value = new Set(imageErrors.value).add(`${book.fileName}-${book.zip}`);


};

const handleImageLoad = (event, book) => {
  if (book) imageErrors.value.delete(`${book.fileName}-${book.zip}`);
};

// ===== AUTH METHODS =====
const openAuthModal = (mode = 'login') => {
  authMode.value = mode;
  authForm.value = { username: '', password: '', confirmPassword: '' };
  authError.value = '';
  showAuthModal.value = true;
};

const closeAuthModal = () => {
  showAuthModal.value = false;
  authError.value = '';
};

const handleAuth = async () => {
  try {
    const res = await fetch('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: authForm.value.username, password: authForm.value.password })
    });
    const data = await res.json();
    if (res.ok) {
      token.value = data.token;
      currentUser.value = data.user;
      localStorage.setItem('token', data.token);
      closeAuthModal();
    } else {
      authError.value = data.message || 'Неверное имя пользователя или пароль';
    }
  } catch (e) {
    console.error('Ошибка входа:', e);
    authError.value = 'Произошла ошибка при попытке авторизации';
  }
};

const handleSetup = async () => {
  if (authForm.value.password.length < 3) {
    authError.value = 'Пароль должен содержать минимум 3 символа';
    return;
  }
  try {
    const res = await fetch('/api/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: authForm.value.username, password: authForm.value.password })
    });
    const data = await res.json();
    if (res.ok) {
      token.value = data.token;
      currentUser.value = data.user;
      localStorage.setItem('token', data.token);
      showSetupModal.value = false;
      authError.value = '';
    } else {
      authError.value = data.message || 'Ошибка настройки';
    }
  } catch (e) {
    authError.value = 'Произошла ошибка';
  }
};

const checkSetupRequired = async () => {
  try {
    const res = await fetch('/api/setup-status');
    if (res.ok) {
      const data = await res.json();
      if (data.setup_required) showSetupModal.value = true;
    }
  } catch (e) {}
};

const openProfileModal = () => {
  profileForm.value = { oldPassword: '', newPassword: '', confirmPassword: '', newUsername: currentUser.value?.username || '' };
  profileError.value = '';
  profileSuccess.value = '';
  showProfileModal.value = true;
};

const handleUpdateProfile = async () => {
  profileError.value = '';
  profileSuccess.value = '';
  if (profileForm.value.newPassword && profileForm.value.newPassword !== profileForm.value.confirmPassword) {
    profileError.value = 'Пароли не совпадают';
    return;
  }
  if (profileForm.value.newPassword && profileForm.value.newPassword.length < 3) {
    profileError.value = 'Пароль должен содержать минимум 3 символа';
    return;
  }
  try {
    const body = {};
    if (profileForm.value.newPassword) {
      body.old_password = profileForm.value.oldPassword;
      body.new_password = profileForm.value.newPassword;
    }
    if (profileForm.value.newUsername && profileForm.value.newUsername !== currentUser.value?.username) {
      body.new_username = profileForm.value.newUsername;
    }
    if (Object.keys(body).length === 0) {
      profileError.value = 'Укажите новый пароль или имя';
      return;
    }
    if (body.new_password && !body.old_password) {
      profileError.value = 'Для смены пароля укажите старый пароль';
      return;
    }
    const res = await fetchWithAuth('/api/update-profile', {
      method: 'POST',
      body: JSON.stringify(body)
    });
    const data = await res.json();
    if (res.ok) {
      profileSuccess.value = 'Профиль обновлён';
      if (body.new_username) currentUser.value = { ...currentUser.value, username: body.new_username };
    } else {
      profileError.value = data.message || 'Ошибка обновления';
    }
  } catch (e) {
    profileError.value = 'Ошибка связи с сервером';
  }
};

const handleLogout = () => {
  token.value = null;
  currentUser.value = null;
  localStorage.removeItem('token');
};

const checkWebAuthStatus = async () => {
  try {
    const res = await fetch('/api/web-auth-status');
    if (res.ok) {
      const data = await res.json();
      webPasswordRequired.value = data.password_required;
      if (data.password_required) showWebAuthModal.value = true;
    }
  } catch (e) { console.error(e); }
};

const handleWebAuth = async () => {
  try {
    const res = await fetch('/api/web-auth', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password: webAuthPassword.value })
    });
    if (res.ok) {
      document.cookie = "web_auth_session=authenticated; path=/; max-age=2592000";
      showWebAuthModal.value = false;
      webAuthError.value = '';
      webAuthPassword.value = '';
    } else {
      webAuthError.value = 'Неверный пароль';
    }
  } catch (e) { webAuthError.value = 'Произошла ошибка'; }
};

const sanitizeFilename = (name) => {
  if (!name) return 'book';
  return name.trim().replace(/[^a-zA-Z0-9а-яА-ЯёЁ\s\-\.]/g, '').replace(/\s+/g, '_');
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
      console.error("Ошибка загрузки деталей:", e);
    } finally {
      loadingDetails.value = false;
    }
  }
});

onMounted(async () => {
  const initialPromises = [
    checkWebAuthStatus(),
    checkSetupRequired(),
    fetchReaderSettings(),
    fetchParseStatus()
  ];

  await Promise.all(initialPromises);

  if (showSetupModal.value) return;

  if (token.value) {
    try {
      const res = await fetchWithAuth('/api/user/status');
      if (res.ok) {
        currentUser.value = await res.json();
        await fetchFavorites();
      } else handleLogout();
    } catch (e) { currentUser.value = null; }
  }

  const onKeydown = (e) => {
    const tag = (e.target?.tagName || '').toLowerCase();
    const inInput = tag === 'input' || tag === 'textarea' || tag === 'select';
    if (e.code === 'Slash') {
      if (!inInput && currentView.value === 'main') {
        e.preventDefault();
        focusSearch();
      }
      return;
    }
    if (inInput && e.key !== 'Escape') return;

    if (e.key === 'Escape') {
      if (selectedBook.value) closeModal();
      else if (showHotkeysHelp.value) showHotkeysHelp.value = false;
      return;
    }
    if (currentView.value !== 'main' || isSearchLocked.value) return;

    if (e.key === 'ArrowLeft') {
      e.preventDefault();
      navigateBook(-1);
      return;
    }
    if (e.key === 'ArrowRight') {
      e.preventDefault();
      navigateBook(1);
      return;
    }
    if (e.code === 'KeyF') {
      if (selectedBook.value && !inInput) {
        e.preventDefault();
        toggleFavorite(selectedBook.value);
      }
      return;
    }
    if (e.code === 'KeyD') {
      if (selectedBook.value && !inInput) {
        e.preventDefault();
        window.location.href = `/download/${selectedBook.value.id}`;
      }
      return;
    }
    if (e.code === 'KeyR') {
      if (selectedBook.value && readerSettings.value.enabled && readBookUrl.value !== '#' && !inInput) {
        e.preventDefault();
        window.open(readBookUrl.value, '_blank');
      }
      return;
    }
    if (e.key === ' ') {
      if (!selectedBook.value && flatBooks.value.length > 0 && !inInput) {
        e.preventDefault();
        selectedBook.value = flatBooks.value[0];
      }
      return;
    }
  };
  window.addEventListener('keydown', onKeydown);

  const parseInterval = setInterval(fetchParseStatus, 2000);

  const handleScroll = () => {
    showScrollTop.value = window.scrollY > 300;
  };
  window.addEventListener('scroll', handleScroll);

  theme.value = localStorage.getItem('theme') || 'light';

  return () => {
    window.removeEventListener('scroll', handleScroll);
    window.removeEventListener('keydown', onKeydown);
    clearInterval(parseInterval);
    if (genreTimeout) clearTimeout(genreTimeout);
    if (imageObserver) imageObserver.disconnect();
  };
});

watch(theme, (newTheme) => {
  document.documentElement.setAttribute('data-theme', newTheme);
  localStorage.setItem('theme', newTheme);
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
</script>

<template>
  <div class="min-h-screen transition-colors duration-300 overflow-x-hidden"
       :class="theme === 'dark' ? 'bg-gray-900 text-gray-100' : 'bg-gradient-to-br from-slate-100 to-slate-200 text-slate-800'">

    <div v-if="currentView === 'main'">
      <!-- Header -->
      <header class="sticky top-0 z-40 backdrop-blur-md border-b transition-colors"
              :class="theme === 'dark' ? 'bg-gray-800/80 border-gray-700' : 'bg-slate-100/80 border-slate-300'">
        <div class="max-w-7xl mx-auto px-4 py-3 flex items-center justify-between">
          <div class="flex items-center space-x-2">
            <div class="p-2 rounded-lg" :class="theme === 'dark' ? 'bg-indigo-600' : 'bg-slate-300'">
              <BookOpen class="h-5 w-5" :class="theme === 'dark' ? 'text-white' : 'text-slate-700'" />
            </div>
            <h1 class="text-xl font-bold bg-gradient-to-r from-indigo-600 to-purple-600 bg-clip-text text-transparent">
              Моя Библиотека
            </h1>
          </div>
          <div class="flex items-center space-x-2">
            <button @click="toggleTheme" class="p-2 rounded-md transition-colors"
                    :class="theme === 'dark' ? 'bg-gray-700 text-yellow-400 hover:bg-gray-600' : 'bg-slate-300 text-slate-700 hover:bg-slate-400'">
              <Sun v-if="theme === 'dark'" class="h-4 w-4" />
              <Moon v-else class="h-4 w-4" />
            </button>
            <button v-if="showSettingsButton" @click="openSettings" class="p-2 rounded-md transition-colors"
                    :class="theme === 'dark' ? 'text-gray-300 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-300'">
              <Settings class="h-5 w-5" />
            </button>
            <button v-if="currentUser" @click="openFavorites" class="hidden sm:flex items-center gap-1.5 px-3 py-2 rounded-md text-sm transition-colors"
                    :class="theme === 'dark' ? 'text-gray-300 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-300'">
              <Heart class="h-4 w-4" /> Книжная полка
            </button>
            <button @click="showHotkeysHelp = !showHotkeysHelp" class="p-2 rounded-md transition-colors"
                    :class="theme === 'dark' ? 'text-gray-400 hover:bg-gray-700' : 'text-slate-500 hover:bg-slate-300'">
              <Keyboard class="h-5 w-5" />
            </button>
            <button v-if="!currentUser" @click="openAuthModal('login')" class="p-2 rounded-md transition-colors"
                    :class="theme === 'dark' ? 'text-gray-300 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-300'">
              <User class="h-5 w-5" />
            </button>
            <template v-else>
              <button @click="openProfileModal" class="px-3 py-2 rounded-md text-sm font-medium transition-colors"
                      :class="theme === 'dark' ? 'text-gray-300 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-300'">
                {{ currentUser.username }}
              </button>
              <button @click="handleLogout" class="p-2 rounded-md transition-colors"
                      :class="theme === 'dark' ? 'text-red-400 hover:bg-gray-700' : 'text-red-600 hover:bg-slate-300'">
                <LogOut class="h-5 w-5" />
              </button>
            </template>
          </div>
        </div>
      </header>

      <!-- Search Form -->
      <section class="py-5 px-4">
        <div class="max-w-5xl mx-auto">
          <form @submit.prevent="handleSearch" class="space-y-4" :class="isSearchLocked ? 'opacity-60 pointer-events-none select-none' : ''">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div>
                <label for="title" class="block text-sm font-medium mb-1"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Название</label>
                <div class="relative">
                  <input ref="searchInputRef" v-model="searchTitle" :disabled="isSearchLocked" type="text"
                         class="w-full px-3 py-2.5 pr-10 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                         :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'" />
                  <button v-if="searchTitle && !isSearchLocked"
                          type="button"
                          @click="clearSearchField('title')"
                          class="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded-md transition-colors"
                          :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'">
                    <X class="h-4 w-4" />
                  </button>
                </div>
              </div>
              <div>
                <label for="author" class="block text-sm font-medium mb-1"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Автор</label>
                <div class="relative">
                  <input v-model="searchAuthor" :disabled="isSearchLocked" type="text"
                         class="w-full px-3 py-2.5 pr-10 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                         :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'" />
                  <button v-if="searchAuthor && !isSearchLocked"
                          type="button"
                          @click="clearSearchField('author')"
                          class="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded-md transition-colors"
                          :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'">
                    <X class="h-4 w-4" />
                  </button>
                </div>
              </div>
              <div class="relative">
                <label for="genre" class="block text-sm font-medium mb-1"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Жанр</label>
                <input v-model="searchGenre"
                       @input="updateGenreSuggestions"
                       @focus="updateGenreSuggestions"
                       @blur="hideGenreSuggestions"
                       :disabled="isSearchLocked"
                       type="text"
                       class="w-full px-3 py-2.5 pr-10 rounded-lg border focus:outline-none focus:ring-2 transition-colors z-10 relative"
                       :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'" />
                <button v-if="searchGenre && !isSearchLocked"
                        type="button"
                        @click="clearSearchField('genre')"
                        class="absolute right-2 top-[30px] p-1 rounded-md transition-colors z-30"
                        :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'">
                  <X class="h-4 w-4" />
                </button>
                <div v-show="showGenreSuggestions"
                     class="absolute z-20 mt-1 w-full rounded-md shadow-lg max-h-60 overflow-auto"
                     :class="theme === 'dark' ? 'bg-gray-800 border border-gray-700' : 'bg-slate-100 border border-slate-300'">
                  <ul class="py-1">
                    <li v-for="suggestion in filteredGenreSuggestions"
                        :key="suggestion.code"
                        @click="selectGenreSuggestion(suggestion)"
                        class="px-4 py-2 text-sm cursor-pointer hover:opacity-90 transition-opacity"
                        :class="theme === 'dark' ? 'text-gray-200 hover:bg-gray-700' : 'text-slate-700 hover:bg-slate-200'">
                      {{ suggestion.name }}
                    </li>
                  </ul>
                </div>
              </div>
              <div>
                <label for="series" class="block text-sm font-medium mb-1"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Серия</label>
                <div class="relative">
                  <input v-model="searchSeries" :disabled="isSearchLocked" type="text"
                         class="w-full px-3 py-2.5 pr-10 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                         :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'" />
                  <button v-if="searchSeries && !isSearchLocked"
                          type="button"
                          @click="clearSearchField('series')"
                          class="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded-md transition-colors"
                          :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'">
                    <X class="h-4 w-4" />
                  </button>
                </div>
              </div>
            </div>
            <div class="flex justify-center">
              <div class="w-full max-w-md">
                <label class="block text-sm font-medium mb-2 text-center"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Язык книги</label>
                <select v-model="selectedLanguage"
                        @change="languageManuallyChanged = true"
                        :disabled="isSearchLocked"
                        class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                        :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'">
                  <option v-for="lang in availableLanguages" :key="lang" :value="lang">
                    {{ lang === 'Все языки' ? 'Все языки' : lang }}
                  </option>
                </select>
              </div>
            </div>
            <div class="text-center">
              <button type="submit" :disabled="isSearchLocked"
                      class="inline-flex items-center gap-2 px-6 py-2.5 rounded-lg font-medium bg-indigo-600 text-white transition-colors"
                      :class="isSearchLocked ? 'opacity-70 cursor-not-allowed' : 'hover:bg-indigo-700'">
                <Search class="h-4 w-4" /> {{ parseStatus.isParsing ? 'БД обновляется…' : 'Найти' }}
              </button>
            </div>
          </form>
        </div>
      </section>

      <!-- ИНДИКАТОР ПАРСИНГА -->
      <div v-if="parseStatus.isParsing" class="mt-2 mb-4">
        <div class="max-w-7xl mx-auto flex flex-col items-center gap-2 px-4">
          <div class="inline-flex items-center gap-2 w-full justify-center">
            <div class="animate-spin rounded-full h-5 w-5 border-2"
                 :class="theme === 'dark' ? 'border-indigo-500 border-t-transparent' : 'border-indigo-600 border-t-transparent'"></div>
            <span class="text-sm break-words"
                  :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-600'">
              {{ parseStatus.message || 'Подождите. Выполняется парсинг' }}
              <span v-if="parseStatus.currentFile" class="ml-1 opacity-80 break-all">
                — {{ parseStatus.currentFile }}
              </span>
              <span v-if="parseProgressPercent != null" class="ml-1 font-medium">
                · {{ parseProgressPercent }}%
              </span>
              <span v-if="parseEstimatedTime" class="ml-1 opacity-80">
                · осталось {{ parseEstimatedTime }}
              </span>
            </span>
          </div>
          <div v-if="parseProgressPercent != null" class="w-full max-w-md rounded-full h-1.5 overflow-hidden">
            <div class="bg-indigo-500 h-1.5 rounded-full transition-all duration-300"
                 :style="{ width: parseProgressPercent + '%' }"></div>
          </div>
        </div>
      </div>

      <!-- Main Content -->
      <main class="px-4 pb-24">
        <div class="max-w-7xl mx-auto">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold"
                :class="theme === 'dark' ? 'text-gray-200' : 'text-slate-700'">
              Найдено: {{ filteredBooks.length }}
              {{ filteredBooks.length === 1 ? 'книга' : (filteredBooks.length % 10 > 1 && filteredBooks.length % 10 < 5 && (filteredBooks.length < 10 || filteredBooks.length > 20)) ? 'книги' : 'книг' }}
            </h2>
          </div>

          <div v-if="loading" class="text-center py-12">
            <div class="inline-flex flex-col items-center gap-3">
              <div class="animate-spin rounded-full h-8 w-8 border-2"
                   :class="theme === 'dark' ? 'border-indigo-500 border-t-transparent' : 'border-indigo-600 border-t-transparent'"></div>
              <span :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-600'">Поиск книг...</span>
            </div>
          </div>

          <div v-else-if="filteredBooks.length === 0" class="text-center py-12">
            <BookOpen class="mx-auto h-12 w-12 mb-3"
                     :class="theme === 'dark' ? 'text-gray-600' : 'text-slate-500'" />
            <p :class="theme === 'dark' ? 'text-gray-400' : 'text-slate-600'">Книги не найдены</p>
          </div>

          <div v-else class="space-y-4">
            <div v-for="authorGroup in groupedBooks" :key="authorGroup.author"
                 class="rounded-xl border overflow-hidden transition-all duration-300"
                 :class="theme === 'dark' ? 'bg-gray-800/50 border-gray-700' : 'bg-white border-slate-200'">
              <!-- Заголовок Автора -->
              <button @click="toggleAuthor(authorGroup.author)"
                      class="w-full flex items-center justify-between p-4 text-left transition-colors hover:bg-opacity-80"
                      :class="theme === 'dark' ? 'hover:bg-gray-700' : 'hover:bg-slate-50'">
                <div class="flex items-center gap-3 w-full">
                  <component :is="expandedAuthors.has(authorGroup.author) ? ChevronDown : ChevronRight"
                             class="h-5 w-5 flex-shrink-0"
                             :class="theme === 'dark' ? 'text-indigo-400' : 'text-indigo-600'"/>
                  <div class="flex-grow min-w-0">
                    <h3 class="font-bold text-lg break-words"
                        :class="theme === 'dark' ? 'text-gray-100' : 'text-slate-800'">
                      {{ authorGroup.author }}
                    </h3>
                    <span class="text-xs font-medium px-2 py-0.5 rounded-full"
                          :class="theme === 'dark' ? 'bg-gray-700 text-gray-300' : 'bg-slate-100 text-slate-600'">
                      Книг: {{ authorGroup.seriesGroups.reduce((acc, sg) => acc + sg.books.length, 0) }}
                    </span>
                  </div>
                </div>
              </button>

              <!-- Контент автора (группы серий) -->
              <div v-show="expandedAuthors.has(authorGroup.author)"
                   class="p-0 border-t transition-all duration-300"
                   :class="theme === 'dark' ? 'border-gray-700 bg-gray-900/30' : 'border-slate-100 bg-slate-50/50'">
                <div v-for="seriesGroup in authorGroup.seriesGroups" :key="`${authorGroup.author}_${seriesGroup.series}`"
                     class="border-b last:border-b-0"
                     :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-100'">
                  <!-- Заголовок Серии -->
                  <button @click="toggleSeries(authorGroup.author, seriesGroup.series)"
                          class="w-full flex items-center justify-between p-4 pl-8 text-left transition-colors hover:bg-opacity-80"
                          :class="theme === 'dark' ? 'hover:bg-gray-700' : 'hover:bg-slate-50'">
                    <div class="flex items-center gap-3 w-full">
                      <component :is="expandedSeries.has(`${authorGroup.author}_${seriesGroup.series}`) ? ChevronDown : ChevronRight"
                                 class="h-5 w-5 flex-shrink-0"
                                 :class="theme === 'dark' ? 'text-indigo-400' : 'text-indigo-600'"/>
                      <div class="flex-grow min-w-0">
                        <h4 class="font-semibold break-words"
                            :class="theme === 'dark' ? 'text-gray-200' : 'text-slate-700'">
                          {{ seriesGroup.series }}
                        </h4>
                        <span class="text-xs font-medium px-2 py-0.5 rounded-full"
                              :class="theme === 'dark' ? 'bg-gray-600 text-gray-300' : 'bg-slate-200 text-slate-600'">
                          Книг: {{ seriesGroup.books.length }}
                        </span>
                      </div>
                    </div>
                  </button>

                  <!-- Контент серии (книги) -->
                  <div v-show="expandedSeries.has(`${authorGroup.author}_${seriesGroup.series}`)"
                       class="p-4 pl-12">
                    <div v-if="viewMode === 'grid'"
                         class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
                      <div v-for="book in seriesGroup.books" :key="book.id"
                           @click="selectedBook = book"
                           class="group cursor-pointer rounded-xl transition-all duration-200 hover:scale-[1.02] flex flex-col border"
                           :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 shadow-md hover:shadow-lg' : 'bg-slate-100 border-slate-300 shadow hover:shadow-md'"
                           style="height: 260px">
                        <div class="h-32 w-full overflow-hidden rounded-t-xl relative book-cover-placeholder"
                             :data-img-id="getImageId(book)">
                          <!-- Плейсхолдер пока изображение не в видимой области -->
                          <div v-if="!shouldLoadImage(book) || hasImageError(book) || !getCoverUrl(book)"
                               class="w-full h-full flex items-center justify-center rounded-t-xl"
                               :class="theme === 'dark' ? 'bg-gray-700' : 'bg-slate-200'">
                            <BookOpen class="h-6 w-6"
                                     :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'" />
                          </div>
                          <!-- Реальное изображение загружается только когда видимо -->
                          <img v-else
                               :src="getCoverUrl(book)"
                               :alt="book.title"
                               class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-200"
                               @error="(e) => handleImageError(e, book)"
                               @load="(e) => handleImageLoad(e, book)"
                               loading="lazy" />
                          <button v-if="currentUser" @click.stop="toggleFavorite(book)"
                                  class="absolute top-1.5 right-1.5 p-1 rounded-full transition-colors"
                                  :class="favoriteIds.has(book.id) ? 'text-red-500 bg-white/90' : 'text-gray-400 bg-white/70 hover:bg-white/90'">
                            <Heart class="h-3.5 w-3.5" :fill="favoriteIds.has(book.id) ? 'currentColor' : 'none'" />
                          </button>
                        </div>
                        <div class="p-3 flex flex-col flex-grow">
                          <h3 class="font-semibold text-sm mb-1 line-clamp-2 leading-tight break-words"
                              :class="theme === 'dark' ? 'text-gray-100' : 'text-slate-800'">
                            {{ book.title }}
                          </h3>
                          <p class="text-xs mb-2 line-clamp-1 break-words"
                             :class="theme === 'dark' ? 'text-gray-400' : 'text-slate-600'">
                            {{ getGenreNames(book.genre) }}
                          </p>
                          <div class="mt-auto space-y-1">
                            <div class="flex justify-between text-xs">
                              <span :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'">Формат:</span>
                              <span class="font-medium uppercase"
                                    :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">{{ book.format }}</span>
                            </div>
                            <div class="flex justify-between text-xs">
                              <span :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'">Язык:</span>
                              <span class="font-medium"
                                    :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">{{ getLanguageName(book.language) }}</span>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>

                    <div v-else class="space-y-3">
                      <div v-for="book in seriesGroup.books" :key="book.id"
                           @click="selectedBook = book"
                           class="group cursor-pointer rounded-xl p-4 transition-all duration-200 flex items-start gap-4 border"
                           :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 hover:bg-gray-750 shadow' : 'bg-slate-100 border-slate-300 hover:bg-slate-200 shadow'">
                        <div class="flex-shrink-0 w-16 h-24 relative book-cover-placeholder"
                             :data-img-id="getImageId(book)">
                          <!-- Плейсхолдер -->
                          <div v-if="!shouldLoadImage(book) || hasImageError(book) || !getCoverUrl(book)"
                               class="w-full h-full flex items-center justify-center rounded-lg"
                               :class="theme === 'dark' ? 'bg-gray-700' : 'bg-slate-200'">
                            <BookOpen class="h-5 w-5"
                                     :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'" />
                          </div>
                          <!-- Реальное изображение -->
                          <img v-else
                               :src="getCoverUrl(book)"
                               :alt="book.title"
                               class="w-full h-full object-cover rounded-lg"
                               @error="(e) => handleImageError(e, book)"
                               @load="(e) => handleImageLoad(e, book)"
                               loading="lazy" />
                          <button v-if="currentUser" @click.stop="toggleFavorite(book)"
                                  class="absolute top-0.5 right-0.5 p-0.5 rounded"
                                  :class="favoriteIds.has(book.id) ? 'text-red-500' : (theme === 'dark' ? 'text-gray-400' : 'text-slate-500')">
                            <Heart class="h-3.5 w-3.5" :fill="favoriteIds.has(book.id) ? 'currentColor' : 'none'" />
                          </button>
                        </div>
                        <div class="flex-grow min-w-0">
                          <h3 class="font-semibold text-base mb-1 line-clamp-2 break-words"
                              :class="theme === 'dark' ? 'text-gray-100' : 'text-slate-800'">
                            {{ book.title }}
                          </h3>
                          <div class="text-sm space-y-1"
                               :class="theme === 'dark' ? 'text-gray-400' : 'text-slate-600'">
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
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </main>

      <!-- Горячие клавиши (подсказка) -->
      <Teleport to="body">
        <div v-if="showHotkeysHelp" class="fixed inset-0 z-[70] flex items-start justify-end p-4 pt-20"
             @click.self="showHotkeysHelp = false">
          <div class="rounded-xl shadow-xl border p-4 max-w-xs text-sm"
               :class="theme === 'dark' ? 'bg-gray-800 border-gray-600' : 'bg-white border-slate-200'">
            <div class="flex justify-between items-center mb-3">
              <span class="font-semibold" :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">Горячие клавиши</span>
              <button @click="showHotkeysHelp = false" class="p-1 rounded" :class="theme === 'dark' ? 'hover:bg-gray-700' : 'hover:bg-slate-100'">
                <X class="h-4 w-4" />
              </button>
            </div>
            <ul class="space-y-2" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-600'">
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">/</kbd> — фокус на поиск</li>
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">Esc</kbd> — закрыть модалку</li>
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">←</kbd><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs ml-0.5">→</kbd> — листать книги</li>
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">F</kbd> — в избранное</li>
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">D</kbd> — скачать</li>
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">R</kbd> — открыть читалку</li>
              <li><kbd class="px-1.5 py-0.5 rounded bg-gray-700 text-gray-200 text-xs">Space</kbd> — быстрый просмотр</li>
            </ul>
          </div>
        </div>
      </Teleport>

      <!-- Scroll to Top Button -->
      <button v-if="showScrollTop" @click="scrollToTop"
              class="fixed bottom-6 right-6 p-3 rounded-full shadow-lg transition-all hover:scale-110 bg-indigo-600 text-white">
        <ArrowUp class="h-5 w-5" />
      </button>
    </div>

    <!-- Favorites (Книжная полка) -->
    <div v-else-if="currentView === 'favorites'" class="min-h-screen p-4 overflow-x-hidden"
         :class="theme === 'dark' ? 'bg-gray-900' : 'bg-gradient-to-br from-slate-100 to-slate-200'">
      <header class="max-w-7xl mx-auto flex items-center justify-between mb-6">
        <h2 class="text-xl font-bold break-words" :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">Книжная полка</h2>
        <button @click="closeFavorites" class="px-4 py-2 rounded-lg font-medium transition-colors"
                :class="theme === 'dark' ? 'bg-gray-700 hover:bg-gray-600 text-white' : 'bg-slate-200 hover:bg-slate-300 text-slate-800'">
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
          <div v-for="book in favoritesBooks" :key="book.id"
               @click="selectedBook = book"
               class="group cursor-pointer rounded-xl transition-all duration-200 hover:scale-[1.02] flex flex-col border"
               :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 shadow-md hover:shadow-lg' : 'bg-slate-100 border-slate-300 shadow hover:shadow-md'"
               style="height: 260px">
            <div class="h-32 w-full overflow-hidden rounded-t-xl relative book-cover-placeholder"
                 :data-img-id="getImageId(book)">
              <!-- Плейсхолдер -->
              <div v-if="!shouldLoadImage(book) || hasImageError(book) || !getCoverUrl(book)"
                   class="w-full h-full flex items-center justify-center rounded-t-xl"
                   :class="theme === 'dark' ? 'bg-gray-700' : 'bg-slate-200'">
                <BookOpen class="h-6 w-6"
                         :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'" />
              </div>
              <!-- Реальное изображение -->
              <img v-else
                   :src="getCoverUrl(book)"
                   :alt="book.title"
                   class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-200"
                   @error="(e) => handleImageError(e, book)"
                   @load="(e) => handleImageLoad(e, book)"
                   loading="lazy" />
              <button @click.stop="toggleFavorite(book)"
                      class="absolute top-2 right-2 p-1.5 rounded-full transition-colors"
                      :class="favoriteIds.has(book.id) ? 'text-red-500 bg-white/90' : 'text-gray-400 bg-white/70 hover:bg-white/90'">
                <Heart class="h-4 w-4" :fill="favoriteIds.has(book.id) ? 'currentColor' : 'none'" />
              </button>
            </div>
            <div class="p-3 flex flex-col flex-grow">
              <h3 class="font-semibold text-sm mb-1 line-clamp-2 leading-tight break-words"
                  :class="theme === 'dark' ? 'text-gray-100' : 'text-slate-800'">
                {{ book.title }}
              </h3>
              <p class="text-xs mb-2 line-clamp-1 break-words"
                 :class="theme === 'dark' ? 'text-gray-400' : 'text-slate-600'">
                {{ getGenreNames(book.genre) }}
              </p>
              <div class="mt-auto space-y-1">
                <div class="flex justify-between text-xs">
                  <span :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'">Формат:</span>
                  <span class="font-medium uppercase"
                        :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">{{ book.format }}</span>
                </div>
                <div class="flex justify-between text-xs">
                  <span :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'">Язык:</span>
                  <span class="font-medium"
                        :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">{{ getLanguageName(book.language) }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Settings View -->
    <div v-else-if="currentView === 'settings'" class="overflow-x-hidden">
      <SettingsView @close="closeSettings" @config-saved="fetchReaderSettings" :token="token" />
    </div>

    <!-- Book Details Modal -->
    <Teleport to="body">
      <div v-if="selectedBook"
           class="fixed inset-0 z-50 flex items-center justify-center p-2 md:p-4 bg-black/60 backdrop-blur-sm overflow-hidden"
           @click="closeModal">
        <div class="relative w-full max-w-4xl max-h-[90vh] overflow-hidden rounded-xl shadow-2xl border flex flex-col"
             :class="theme === 'dark' ? 'bg-gray-800 border-gray-700' : 'bg-slate-50 border-slate-300'"
             @click.stop>
          <div class="flex justify-between items-start p-3 sm:p-4 border-b"
               :class="theme === 'dark' ? 'border-gray-700 bg-gray-800' : 'border-slate-200 bg-white'">
            <div class="pr-4">
              <h2 class="text-lg sm:text-xl font-bold leading-tight break-words"
                  :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">
                {{ selectedBook.title }}
              </h2>
              <p class="text-sm mt-1 break-words"
                 :class="theme === 'dark' ? 'text-indigo-400' : 'text-indigo-600'">
                {{ selectedBook.author }}
              </p>
            </div>
            <div class="flex items-center gap-1">
              <button v-if="currentUser" @click="toggleFavorite(selectedBook)"
                      class="p-1.5 sm:p-2 rounded-full transition-colors"
                      :class="favoriteIds.has(selectedBook.id) ? 'text-red-500' : (theme === 'dark' ? 'text-gray-400 hover:bg-gray-700' : 'text-slate-500 hover:bg-slate-200')">
                <Heart class="h-4 sm:h-5 w-4 sm:w-5" :fill="favoriteIds.has(selectedBook.id) ? 'currentColor' : 'none'" />
              </button>
              <button @click="closeModal"
                      class="p-1.5 sm:p-2 rounded-full transition-colors flex-shrink-0"
                      :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-200'">
                <X class="h-5 sm:h-6 w-5 sm:w-6" />
              </button>
            </div>
          </div>
          <div class="overflow-y-auto overflow-x-hidden p-0 flex-grow">
            <div class="flex flex-col md:flex-row">
              <div class="md:w-1/3 p-3 sm:p-5 flex flex-col items-center border-b md:border-b-0 md:border-r"
                   :class="theme === 'dark' ? 'border-gray-700 bg-gray-800/50' : 'border-slate-200 bg-white'">
                <div class="w-full max-w-[200px] mb-4 shadow-lg rounded-lg overflow-hidden relative group">
                  <!-- В модалке загружаем сразу, т.к. она уже открыта -->
                  <img v-if="getCoverUrl(selectedBook) && !hasImageError(selectedBook)"
                       :src="getCoverUrl(selectedBook)"
                       :alt="selectedBook.title"
                       class="w-full h-auto object-cover"
                       @error="(e) => handleImageError(e, selectedBook)"
                       @load="(e) => handleImageLoad(e, selectedBook)" />
                  <div v-else class="w-full aspect-[2/3] flex items-center justify-center"
                       :class="theme === 'dark' ? 'bg-gray-700' : 'bg-slate-200'">
                    <BookOpen class="h-12 w-12"
                             :class="theme === 'dark' ? 'text-gray-500' : 'text-slate-500'" />
                  </div>
                </div>
                <a :href="`/download/${selectedBook.id}`"
                   class="w-full flex items-center justify-center gap-2 py-3 px-4 rounded-lg font-bold shadow-lg transform transition hover:scale-105 mb-3"
                   :class="theme === 'dark' ? 'bg-indigo-600 hover:bg-indigo-500 text-white' : 'bg-indigo-600 hover:bg-indigo-700 text-white'">
                  <Download class="h-5 w-5" /> Скачать ({{ selectedBook.format.toUpperCase() }})
                </a>
                <a v-if="readerSettings.enabled && readerSettings.url"
                   :href="readBookUrl"
                   target="_blank"
                   class="w-full flex items-center justify-center gap-2 py-2 px-4 rounded-lg font-bold shadow-md transform transition hover:scale-105 mb-6 border"
                   :class="theme === 'dark' ? 'border-indigo-500 text-indigo-400 hover:bg-gray-700' : 'border-indigo-600 text-indigo-700 hover:bg-indigo-50'">
                  <BookOpen class="h-5 w-5" /> Читать
                </a>
                <div class="w-full space-y-3 text-sm"
                     :class="theme === 'dark' ? 'text-gray-200' : 'text-slate-800'">
                  <div class="flex justify-between border-b pb-2" :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-200'">
                    <span class="opacity-70 flex-shrink-0">Серия:</span>
                    <span class="font-medium text-right break-words ml-2">
                      {{ selectedBook.series || '—' }}
                      <span v-if="selectedBook.seriesNo" class="bg-indigo-100 text-indigo-800 text-xs px-1.5 py-0.5 rounded ml-1">#{{ selectedBook.seriesNo }}</span>
                    </span>
                  </div>
                  <div class="flex justify-between border-b pb-2" :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-200'">
                    <span class="opacity-70 flex-shrink-0">Жанр:</span>
                    <span class="font-medium text-right break-words ml-2">{{ getGenreNames(selectedBook.genre) }}</span>
                  </div>
                  <div class="flex justify-between border-b pb-2" :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-200'">
                    <span class="opacity-70 flex-shrink-0">Язык:</span>
                    <span class="font-medium text-right break-words ml-2">{{ getLanguageName(selectedBook.language) }}</span>
                  </div>
                  <div class="flex justify-between border-b pb-2" :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-200'">
                    <span class="opacity-70 flex-shrink-0">Размер:</span>
                    <span class="font-medium text-right break-words ml-2">{{ formatFileSize(selectedBook.fileSize) }}</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="opacity-70 flex-shrink-0">Добавлено:</span>
                    <span class="font-medium text-right break-words ml-2">{{ new Date(selectedBook.addedAt).toLocaleDateString() }}</span>
                  </div>
                </div>
              </div>
              <div class="md:w-2/3 p-3 sm:p-5 md:p-6"
                   :class="theme === 'dark' ? 'bg-gray-900/50' : 'bg-slate-50'">
                <div v-if="loadingDetails" class="flex flex-col items-center justify-center h-40 space-y-3">
                  <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-500"></div>
                  <span class="text-sm opacity-70">Распаковка описания...</span>
                </div>
                <div v-else>
                  <div v-if="bookDetails && bookDetails.titleInfo && bookDetails.titleInfo.annotationHtml" class="mb-8">
                    <h3 class="text-lg font-bold mb-3 flex items-center gap-2"
                        :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">
                      <span class="w-1 h-6 bg-indigo-500 rounded-full"></span>
                      Аннотация
                    </h3>
                    <div class="prose max-w-none text-sm leading-relaxed book-annotation"
                         :class="theme === 'dark' ? 'prose-invert text-gray-300' : 'text-slate-700'"
                         v-html="bookDetails.titleInfo.annotationHtml">
                    </div>
                  </div>
                  <div v-else-if="(selectedBook.format === 'fb2' || selectedBook.format === 'epub') && !loadingDetails" class="mb-8 text-center py-4 opacity-50 italic">
                    Описание отсутствует
                  </div>
                  <div v-else-if="selectedBook.format !== 'fb2' && selectedBook.format !== 'epub'" class="mb-8 p-4 rounded-lg border border-dashed text-center text-sm opacity-70"
                       :class="theme === 'dark' ? 'border-gray-700' : 'border-slate-300'">
                    Детальное описание доступно только для FB2 и EPUB файлов
                  </div>
                  <div v-if="bookDetails" class="space-y-6">
                    <div v-if="bookDetails.publishInfo && (bookDetails.publishInfo.publisher || bookDetails.publishInfo.year)">
                      <h3 class="text-md font-bold mb-2 opacity-80 uppercase text-xs tracking-wider">Информация об издании</h3>
                      <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-2 text-sm rounded-lg p-3"
                           :class="theme === 'dark' ? 'bg-gray-800' : 'bg-white border'">
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
                      <div class="text-sm rounded-lg p-3"
                           :class="theme === 'dark' ? 'bg-gray-800' : 'bg-white border'">
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
    </Teleport>

    <!-- Modals -->
    <Teleport to="body">
      <!-- Setup Modal -->
      <div v-if="showSetupModal"
           class="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
        <div class="relative w-full max-w-md rounded-xl shadow-2xl border bg-gray-800 border-gray-700 p-6" @click.stop>
          <h2 class="text-xl font-bold mb-4 text-center text-white">Первоначальная настройка</h2>
          <p class="text-gray-300 text-sm mb-4 text-center">Создайте учётную запись администратора</p>
          <form @submit.prevent="handleSetup" class="space-y-4">
            <div>
              <label class="block text-sm font-medium mb-1 text-gray-300">Имя пользователя</label>
              <input v-model="authForm.username" type="text" required
                     class="w-full px-3 py-2.5 rounded-lg border bg-gray-700 border-gray-600 text-white focus:ring-indigo-500" />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1 text-gray-300">Пароль (мин. 3 символа)</label>
              <input v-model="authForm.password" type="password" required
                     class="w-full px-3 py-2.5 rounded-lg border bg-gray-700 border-gray-600 text-white focus:ring-indigo-500" />
            </div>
            <div v-if="authError" class="text-red-400 text-sm text-center">{{ authError }}</div>
            <button type="submit"
                    class="w-full py-2.5 px-4 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white">
              Создать администратора
            </button>
          </form>
        </div>
      </div>

      <!-- Profile Modal -->
      <div v-if="showProfileModal"
           class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm"
           @click="showProfileModal = false">
        <div class="relative w-full max-w-md rounded-xl shadow-2xl border p-6"
             :class="theme === 'dark' ? 'bg-gray-800 border-gray-700' : 'bg-slate-100 border-slate-300'"
             @click.stop>
          <h2 class="text-xl font-bold mb-4" :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">Мой профиль</h2>
          <form @submit.prevent="handleUpdateProfile" class="space-y-4">
            <div>
              <label class="block text-sm font-medium mb-1" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Имя пользователя</label>
              <input v-model="profileForm.newUsername" type="text"
                     class="w-full px-3 py-2.5 rounded-lg border"
                     :class="theme === 'dark' ? 'bg-gray-700 border-gray-600 text-white' : 'bg-slate-100 border-slate-300 text-slate-800'" />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Новый пароль (оставьте пустым, если не меняете)</label>
              <input v-model="profileForm.newPassword" type="password"
                     class="w-full px-3 py-2.5 rounded-lg border"
                     :class="theme === 'dark' ? 'bg-gray-700 border-gray-600 text-white' : 'bg-slate-100 border-slate-300 text-slate-800'" />
            </div>
            <div v-if="profileForm.newPassword">
              <label class="block text-sm font-medium mb-1" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Текущий пароль</label>
              <input v-model="profileForm.oldPassword" type="password"
                     class="w-full px-3 py-2.5 rounded-lg border"
                     :class="theme === 'dark' ? 'bg-gray-700 border-gray-600 text-white' : 'bg-slate-100 border-slate-300 text-slate-800'" />
            </div>
            <div v-if="profileForm.newPassword">
              <label class="block text-sm font-medium mb-1" :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Подтверждение пароля</label>
              <input v-model="profileForm.confirmPassword" type="password"
                     class="w-full px-3 py-2.5 rounded-lg border"
                     :class="theme === 'dark' ? 'bg-gray-700 border-gray-600 text-white' : 'bg-slate-100 border-slate-300 text-slate-800'" />
            </div>
            <div v-if="profileError" class="text-red-500 text-sm">{{ profileError }}</div>
            <div v-if="profileSuccess" class="text-green-500 text-sm">{{ profileSuccess }}</div>
            <div class="flex gap-2">
              <button type="submit" class="px-4 py-2.5 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white">
                Сохранить
              </button>
              <button type="button" @click="showProfileModal = false"
                      class="px-4 py-2.5 rounded-lg font-medium border"
                      :class="theme === 'dark' ? 'border-gray-600 text-gray-300 hover:bg-gray-700' : 'border-slate-300 text-slate-700 hover:bg-slate-200'">
                Закрыть
              </button>
            </div>
          </form>
        </div>
      </div>

      <!-- Web Auth Modal -->
      <div v-if="showWebAuthModal"
           class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
        <div class="relative w-full max-w-md rounded-xl shadow-2xl border bg-gray-800 border-gray-700"
             @click.stop>
          <div class="p-6">
            <h2 class="text-xl font-bold mb-4 text-center text-white">
              Вход в систему
            </h2>
            <form @submit.prevent="handleWebAuth" class="space-y-4">
              <div>
                <label for="web-auth-password" class="block text-sm font-medium mb-1 text-gray-300">Пароль</label>
                <input id="web-auth-password"
                       v-model="webAuthPassword"
                       type="password"
                       required
                       class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors bg-gray-700 border-gray-600 text-white focus:ring-indigo-500"
                       placeholder="Введите пароль для доступа" />
              </div>
              <div v-if="webAuthError" class="text-red-400 text-sm text-center">
                {{ webAuthError }}
              </div>
              <button type="submit"
                      class="w-full py-2.5 px-4 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white transition-colors">
                Войти
              </button>
            </form>
          </div>
        </div>
      </div>

      <!-- Auth Modal -->
      <div v-if="showAuthModal"
           class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm"
           @click="closeAuthModal">
        <div class="relative w-full max-w-md rounded-xl shadow-2xl border"
             :class="theme === 'dark' ? 'bg-gray-800 border-gray-700' : 'bg-slate-100 border-slate-300'"
             @click.stop>
          <button @click="closeAuthModal"
                  class="absolute top-3 right-3 p-1.5 rounded-full transition-colors"
                  :class="theme === 'dark' ? 'text-gray-400 hover:text-white hover:bg-gray-700' : 'text-slate-500 hover:text-slate-700 hover:bg-slate-300'">
            <X class="h-5 w-5" />
          </button>
          <div class="p-6">
            <h2 class="text-xl font-bold mb-4 text-center"
                :class="theme === 'dark' ? 'text-white' : 'text-slate-800'">
              Вход
            </h2>
            <form @submit.prevent="handleAuth" class="space-y-4">
              <div>
                <label for="auth-username" class="block text-sm font-medium mb-1"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Имя пользователя</label>
                <input id="auth-username"
                       v-model="authForm.username"
                       type="text"
                       required
                       class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                       :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'" />
              </div>
              <div>
                <label for="auth-password" class="block text-sm font-medium mb-1"
                       :class="theme === 'dark' ? 'text-gray-300' : 'text-slate-700'">Пароль</label>
                <input id="auth-password"
                       v-model="authForm.password"
                       type="password"
                       required
                       class="w-full px-3 py-2.5 rounded-lg border focus:outline-none focus:ring-2 transition-colors"
                       :class="theme === 'dark' ? 'bg-gray-800 border-gray-700 text-white focus:ring-indigo-500' : 'bg-slate-100 border-slate-300 text-slate-800 focus:ring-slate-500'" />
              </div>
              <div v-if="authError" class="text-red-500 text-sm text-center">
                {{ authError }}
              </div>
              <button type="submit"
                      class="w-full py-2.5 px-4 rounded-lg font-medium bg-indigo-600 hover:bg-indigo-700 text-white transition-colors">
                Войти
              </button>
            </form>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  overflow-wrap: break-word;
}
.line-clamp-1 {
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
  overflow-wrap: break-word;
}
.book-annotation {
  overflow-wrap: break-word;
  hyphens: auto;
}
.book-annotation :deep(p) {
  margin-bottom: 0.75em;
  text-align: justify;
}
.book-annotation :deep(strong), .book-annotation :deep(b) {
  font-weight: 700;
  color: inherit;
}
.book-annotation :deep(i), .book-annotation :deep(em) {
  font-style: italic;
}
</style>
