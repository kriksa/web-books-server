<template>
  <div class="reader-shell" :class="{ mobile: isMobile }" :data-theme="themeMode">
    <header
      v-show="hudVisible"
      class="rd-hud reader-chrome"
      :class="{
        'rd-hud--dock': isMobile && bookUrl && !fatalError,
        'rd-hud--solo': isMobile && fatalError,
      }"
      aria-label="Управление читалкой"
    >
      <button type="button" class="rd-fab" @click="closeReader" aria-label="Закрыть">
        <svg class="rd-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <path d="M19 12H5M12 19l-7-7 7-7" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
      <template v-if="bookUrl && !fatalError">
        <p v-if="isMobile" class="rd-hud__ttl">{{ displayTitle }}</p>
        <button
          type="button"
          class="rd-fab rd-fab--accent"
          aria-label="Настройки отображения"
          :aria-expanded="settingsSheetOpen"
          @click="settingsSheetOpen = true"
        >
          <svg class="rd-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M2 14h4M10 8h4M18 16h4" stroke-linecap="round" />
          </svg>
        </button>
      </template>
    </header>

    <div v-if="fatalError" class="rd-screen rd-screen--err reader-chrome" role="alert">
      <div class="rd-card">
        <p class="rd-card__tag">Ошибка</p>
        <p class="rd-card__msg">{{ fatalError }}</p>
        <p class="reader-hint rd-card__more">
          Основные форматы: FB2, EPUB, MOBI, PDF — во foliate; TXT, DOC/DOCX, RTF, DJVU и др. — текст через сервер в FB2.
          PDF в foliate без перекодирования. Для DJVU на сервере нужен djvulibre (djvutxt); для старого DOC — antiword или catdoc.
        </p>
      </div>
    </div>

    <div v-else-if="metaLoading" class="rd-screen rd-screen--wait reader-chrome" role="status" aria-live="polite">
      <div class="rd-wait">
        <div class="rd-wait__orb" aria-hidden="true" />
        <p class="rd-wait__txt">Загрузка книги…</p>
      </div>
    </div>

    <div v-else-if="bookUrl" ref="readerStageRef" class="reader-stage" tabindex="-1">
      <div v-if="showCoverPage" class="cover-first rd-intro" @click="goNext">
        <div ref="coverFirstInnerRef" class="cover-first-inner rd-intro__grid" tabindex="-1" @click.stop>
          <div v-if="coverImageUrl" class="rd-intro__art">
            <img class="cover-first-img rd-intro__img" :src="coverImageUrl" alt="" loading="lazy" />
          </div>
          <div class="cover-first-meta rd-intro__card">
            <div class="cover-first-title rd-intro__h">{{ displayTitle }}</div>
            <div v-if="metaAuthor" class="cover-first-author rd-intro__by">{{ metaAuthor }}</div>
            <p class="cover-first-hint rd-intro__note">
              На ПК: клавиатура и мышь (колесо, клики) работают одновременно, без переключения режимов. Стрелки, PgUp/PgDn, пробел, Enter, Backspace — листать и закрыть обложку.
            </p>
            <button type="button" class="rd-cta" @click.stop="goNext">Читать</button>
          </div>
        </div>
      </div>
      <VueReader
        :key="activeBookId"
        :url="bookUrl"
        :title="displayTitle"
        :show-toc="true"
        :init-option="readerInitOption"
        :get-rendition="onFoliateView"
        @update:location="persistLocation"
      />
      <div
        v-if="isMobile"
        class="tap-layer"
        @touchstart="onTapStart"
        @touchmove="onTapMove"
        @touchend="onTapEnd"
        @touchcancel="onTapCancel"
        @click="onClickTap"
      />
      <div v-if="coverOpen && coverImageUrl" class="cover-overlay rd-pop" @click="coverOpen = false">
        <img class="cover-overlay-img rd-pop__img" :src="coverImageUrl" alt="" loading="lazy" @click.stop />
      </div>
    </div>

    <div
      v-if="settingsSheetOpen && bookUrl && !fatalError"
      class="rd-scrim reader-chrome"
      @click="settingsSheetOpen = false"
      @wheel.prevent.stop
    >
      <div class="rd-sheet reader-chrome" role="dialog" aria-modal="true" aria-labelledby="rd-sheet-h" @click.stop>
        <div class="rd-sheet__cap" aria-hidden="true" />
        <div class="rd-sheet__head">
          <h2 id="rd-sheet-h" class="rd-sheet__h">Настройки</h2>
          <button type="button" class="rd-fab rd-fab--sm" aria-label="Закрыть" @click="settingsSheetOpen = false">
            <svg class="rd-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
              <path d="M18 6L6 18M6 6l12 12" stroke-linecap="round" />
            </svg>
          </button>
        </div>
        <div class="rd-sheet__body">
          <section class="rd-sec">
            <span class="rd-sec__lab">Тема</span>
            <div class="rd-palette" role="group" aria-label="Тема">
              <button type="button" class="rd-tile" :class="{ on: themeMode === 'light' }" @click="setTheme('light')">
                <span class="rd-tile__sw rd-tile__sw--lt" />
                <span class="rd-tile__tx">Светлая</span>
              </button>
              <button type="button" class="rd-tile" :class="{ on: themeMode === 'sepia' }" @click="setTheme('sepia')">
                <span class="rd-tile__sw rd-tile__sw--sp" />
                <span class="rd-tile__tx">Сепия</span>
              </button>
              <button type="button" class="rd-tile" :class="{ on: themeMode === 'dark' }" @click="setTheme('dark')">
                <span class="rd-tile__sw rd-tile__sw--dk" />
                <span class="rd-tile__tx">Тёмная</span>
              </button>
            </div>
          </section>
          <section class="rd-sec">
            <span class="rd-sec__lab">Страница</span>
            <div class="rd-pills" role="group">
              <button type="button" class="rd-pill" :class="{ on: spreadMode === 'single' }" @click="setSpread('single')">Одна</button>
              <button type="button" class="rd-pill" :class="{ on: spreadMode === 'double' }" @click="setSpread('double')">Разворот</button>
            </div>
          </section>
          <section class="rd-sec">
            <span class="rd-sec__lab">Текст</span>
            <div class="rd-pills" role="group">
              <button type="button" class="rd-pill" :class="{ on: textAlign === 'start' }" @click="setAlign('start')">Влево</button>
              <button type="button" class="rd-pill" :class="{ on: textAlign === 'justify' }" @click="setAlign('justify')">Ширина</button>
              <button type="button" class="rd-pill" :class="{ on: textAlign === 'end' }" @click="setAlign('end')">Вправо</button>
            </div>
          </section>
          <section v-if="coverImageUrl" class="rd-sec">
            <button
              type="button"
              class="rd-linkbtn"
              @click="
                coverOpen = true;
                settingsSheetOpen = false;
              "
            >
              Обложка
            </button>
          </section>
        </div>
        <div class="rd-sheet__done">
          <button type="button" class="rd-cta rd-cta--wide" @click="settingsSheetOpen = false">Готово</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import { VueReader } from 'vue-book-reader';
import { getOrCreateReaderInstanceId, syncGuestReaderProgressCookie } from './reader-instance-id.js';

const props = defineProps({
  /** > 0 — книга из родительского приложения (встроенный режим); 0 — взять id из URL (отдельная reader.html). */
  bookId: { type: Number, default: 0 },
});

const emit = defineEmits(['close']);

const LS_SPREAD = 'books-reader-spread';
const LS_ALIGN = 'books-reader-text-align';
const LS_THEME = 'books-reader-theme';

// Современные темы (минимум: светлая/тёмная/сепия)
const themeMode = ref('dark'); // dark | light | sepia

function readBookIdFromUrl() {
  const p = new URLSearchParams(window.location.search);
  return Number(p.get('book') || p.get('id') || 0) || 0;
}

const activeBookId = computed(() => (props.bookId > 0 ? props.bookId : readBookIdFromUrl()));

const cfiKey = () => `books-reader-cfi-${activeBookId.value}`;

/**
 * vue-book-reader передаёт в update:location строку CFI/fragment или объект { cfi, href, … }.
 * Раньше ожидали только detail.cfi — из‑за этого localStorage не заполнялся.
 */
function extractProgressLocation(payload) {
  if (payload == null) return null;
  if (typeof payload === 'string') {
    const s = payload.trim();
    return s.length > 0 ? s : null;
  }
  if (typeof payload === 'object') {
    const o = payload;
    const cand =
      (typeof o.cfi === 'string' && o.cfi) ||
      (typeof o.fragment === 'string' && o.fragment) ||
      (typeof o.href === 'string' && o.href) ||
      (typeof o.target === 'string' && o.target) ||
      (o.bookmark && typeof o.bookmark.href === 'string' && o.bookmark.href) ||
      (o.location && typeof o.location === 'object' && typeof o.location.cfi === 'string' && o.location.cfi);
    if (typeof cand === 'string' && cand.trim().length > 0) return cand.trim();
  }
  return null;
}

function readAuthToken() {
  try {
    return localStorage.getItem('token') || sessionStorage.getItem('token') || '';
  } catch {
    return '';
  }
}

/** Залогинен — в приоритете JWT (user_id); иначе гостевой прогресс по cookie/заголовку. */
function progressFetchHeaders() {
  const headers = {
    'X-Books-Guest-Reader': getOrCreateReaderInstanceId(),
  };
  const token = readAuthToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  return headers;
}

/** Ответ GET /api/reading-progress: position может быть объектом или строкой JSON/CFI. */
function extractCfiFromServerPosition(pos) {
  if (pos == null || pos === '') return null;
  if (typeof pos === 'string') {
    const t = pos.trim();
    if (t.startsWith('{')) {
      try {
        const o = JSON.parse(t);
        return typeof o?.cfi === 'string' && o.cfi.length > 0 ? o.cfi : null;
      } catch {
        return t.length > 8 ? t : null;
      }
    }
    return t.length > 8 ? t : null;
  }
  if (typeof pos === 'object' && typeof pos.cfi === 'string' && pos.cfi.length > 0) return pos.cfi;
  return null;
}

async function syncProgressFromServer() {
  const id = activeBookId.value;
  if (!id) return;
  const controller = new AbortController();
  const tid = window.setTimeout(() => controller.abort(), 12000);
  try {
    const r = await fetch(`/api/reading-progress?book_id=${encodeURIComponent(String(id))}`, {
      headers: progressFetchHeaders(),
      credentials: 'same-origin',
      signal: controller.signal,
    });
    if (!r.ok) return;
    const data = await r.json();
    const cfi = extractCfiFromServerPosition(data.position) || extractProgressLocation(data.position);
    if (!cfi) return;
    savedCFI.value = cfi;
    try {
      localStorage.setItem(cfiKey(), cfi);
    } catch {}
  } catch {
    //
  } finally {
    clearTimeout(tid);
  }
}

/** Только блоки текста — не трогаем body/html и типичную обложку (картинка без абзацев). */
const TEXT_ALIGN_SELECTORS =
  'p, li, dd, dt, td, th, blockquote, figcaption, address, h1, h2, h3, h4, h5, h6';

const metaTitle = ref('');
const metaAuthor = ref('');
const metaFormat = ref('');
const metaFileName = ref('');
const metaZip = ref('');
const fatalError = ref('');
/** Пока не пришли метаданные — не монтируем VueReader (иначе пустой экран «вечная загрузка»). */
const metaLoading = ref(true);

const savedCFI = ref(null);
const spreadMode = ref('double');
const textAlign = ref('start');

const isMobile = ref(false);
const controlsVisible = ref(false);

let foliateView = null;
/** Снять подписки клавиатуры с ранее загруженных iframe-документов (смена книги). */
const foliateDocKeyCleanups = [];
const foliateKeyBoundDocs = new WeakSet();
let persistTimer = 0;
/** Последний известный CFI для сохранения при уходе со страницы (до срабатывания debounce). */
let pendingCfiForFlush = null;
let loadHandler = null;
let resizeHandler = null;
let ro = null;

const tapStartX = ref(0);
const tapStartY = ref(0);
const tapMoved = ref(false);
const tapActive = ref(false);

const coverOpen = ref(false);
const showCoverPage = ref(false);
/** Нижняя панель настроек (тема, разворот, выравнивание). */
const settingsSheetOpen = ref(false);

/** Контейнер читалки: навешиваем wheel/touch в capture, чтобы ловить события из web-компонента foliate. */
const readerStageRef = ref(null);
const coverFirstInnerRef = ref(null);
let stageNavCleanup = null;

function readThemeLS() {
  try {
    const v = localStorage.getItem(LS_THEME);
    if (v === 'dark' || v === 'light' || v === 'sepia') return v;
  } catch {}
  return 'dark';
}

function setTheme(mode) {
  themeMode.value = mode;
  try {
    localStorage.setItem(LS_THEME, mode);
  } catch {}
}

themeMode.value = readThemeLS();

function readNumLS(key, fallback) {
  try {
    const v = localStorage.getItem(key);
    if (v == null || v === '') return fallback;
    const n = Number(v);
    return Number.isFinite(n) ? n : fallback;
  } catch {
    return fallback;
  }
}

function readSpreadLS() {
  try {
    const v = localStorage.getItem(LS_SPREAD);
    if (v === 'single' || v === 'double') return v;
  } catch {}
  if (typeof window !== 'undefined' && window.matchMedia?.('(max-width: 720px)').matches) return 'single';
  return 'double';
}

function readAlignLS() {
  try {
    const v = localStorage.getItem(LS_ALIGN);
    if (v === 'start' || v === 'justify' || v === 'end') return v;
  } catch {}
  return 'start';
}

spreadMode.value = readSpreadLS();
textAlign.value = readAlignLS();

function sanitizeForPath(name) {
  if (!name) return '';
  return name
    .trim()
    .replace(/[^a-zA-Z0-9а-яА-ЯёЁ\s\-.]/g, '')
    .replace(/\s+/g, '_');
}

function extFromFileName(fn) {
  if (!fn || typeof fn !== 'string') return '';
  const m = fn.match(/\.([^.]+)$/);
  return m ? m[1].toLowerCase() : '';
}

function downloadPathTail() {
  const fmt = (metaFormat.value || extFromFileName(metaFileName.value) || '').toLowerCase();
  if (!fmt) return '';
  const st = sanitizeForPath(metaTitle.value) || 'book';
  const au = sanitizeForPath(metaAuthor.value) || 'unknown';
  return `${st} - ${au}.${fmt}`;
}

const bookUrl = computed(() => {
  if (!activeBookId.value || fatalError.value) return '';
  const fmt = (metaFormat.value || extFromFileName(metaFileName.value) || '').toLowerCase();
  if (!fmt) return '';
  try {
    if (FOLIATE_SERVER_CONVERTED_FORMATS.has(fmt)) {
      const path = `/api/reader/foliate-converted.fb2?id=${encodeURIComponent(String(activeBookId.value))}`;
      return new URL(path, window.location.origin).href;
    }
    const tail = downloadPathTail();
    if (!tail) return '';
    const path = `/download/${encodeURIComponent(activeBookId.value)}/${encodeURIComponent(tail)}`;
    return new URL(path, window.location.origin).href;
  } catch {
    return '';
  }
});

/** Плавающие «Закрыть / Настройки»: десктоп — всегда при открытой книге; мобил — по центральному тапу. */
const hudVisible = computed(() => {
  if (fatalError.value) return true;
  if (metaLoading.value) return false;
  if (!bookUrl.value) return false;
  if (!isMobile.value) return true;
  return controlsVisible.value;
});

const displayTitle = computed(() => metaTitle.value || (activeBookId.value ? `Книга #${activeBookId.value}` : 'Чтение'));

const coverImageUrl = computed(() => {
  const fmt = (metaFormat.value || '').toLowerCase();
  if (fmt !== 'epub' && fmt !== 'fb2') return '';
  const fn = metaFileName.value;
  const z = metaZip.value;
  if (!fn || !z) return '';
  const q = new URLSearchParams({ file: fn, zip: z, format: fmt });
  try {
    return new URL(`/api/cover?${q}`, window.location.origin).href;
  } catch {
    return '';
  }
});

function openCoverAsFirstPageIfAny() {
  // Требование: первая страница — обложка (если есть).
  // Для foliate проще показать cover-page поверх ридера до первого действия пользователя.
  showCoverPage.value = !!coverImageUrl.value && !fatalError.value;
}

function closeCoverPage() {
  showCoverPage.value = false;
  focusReaderStage();
}

/** Фокус на области чтения: события клавиатуры в iframe не всплывают в родительское window. */
function focusReaderStage() {
  nextTick(() => {
    try {
      readerStageRef.value?.focus({ preventScroll: true });
    } catch {
      try {
        readerStageRef.value?.focus();
      } catch {}
    }
  });
}

const readerInitOption = computed(() => {
  const cfi = savedCFI.value;
  if (cfi) return { lastLocation: cfi };
  return undefined;
});

// foliate-js открывает напрямую (см. vue-book-reader / makeBook).
const FOLIATE_NATIVE_FORMATS = new Set([
  'epub',
  'mobi',
  'azw3',
  'kf8',
  'fb2',
  'cbz',
  'pdf',
  'zip',
  'azw',
  'prc',
  'fbz'
]);

// Через сервер convertRawToFB2UTF8 → отдаём FB2 по /api/reader/foliate-converted.fb2 (тот же конвейер, что Liberama).
const FOLIATE_SERVER_CONVERTED_FORMATS = new Set([
  'txt',
  'html',
  'htm',
  'doc',
  'docx',
  'rtf',
  'djvu',
  'tcr',
  'odt',
  'htmlz',
  'rb',
  'pml',
  'pmlz'
]);

function readerFormatSupported(fmt) {
  if (!fmt) return false;
  const f = String(fmt).toLowerCase().trim();
  return FOLIATE_NATIVE_FORMATS.has(f) || FOLIATE_SERVER_CONVERTED_FORMATS.has(f);
}

function notifyParent(type) {
  const id = activeBookId.value;
  const payload = {
    type,
    bookId: id,
    readerInstanceId: getOrCreateReaderInstanceId(),
  };
  try {
    const bc = new BroadcastChannel(`embedded-reader-${id}`);
    bc.postMessage(payload);
    bc.close();
  } catch {}
  try {
    if (window.opener && !window.opener.closed) window.opener.postMessage(payload, window.location.origin);
  } catch {}
}

function injectTextAlign(doc) {
  if (!doc?.head) return;
  const a = textAlign.value;
  let el = doc.getElementById('books-reader-user-align');
  if (!el) {
    el = doc.createElement('style');
    el.id = 'books-reader-user-align';
    doc.head.appendChild(el);
  }
  el.textContent = `${TEXT_ALIGN_SELECTORS} { text-align: ${a} !important; }`;
}

function refreshTextAlignAll() {
  if (!foliateView?.renderer?.getContents) return;
  try {
    for (const c of foliateView.renderer.getContents()) {
      if (c?.doc) injectTextAlign(c.doc);
    }
  } catch {}
}

function applySpread(view) {
  const r = view?.renderer;
  if (!r?.setAttribute) return;
  if (String(r.tagName || '').toLowerCase() !== 'foliate-paginator') return;
  r.setAttribute('max-column-count', spreadMode.value === 'double' ? '2' : '1');
}

function computeAutoLayout() {
  // Требование: текст занимает всю доступную область экрана (без «колонки по центру»).
  // Для разворота (2 страницы) делим ширину на 2.
  const rect = foliateView?.renderer?.getBoundingClientRect?.();
  const w = Math.max(320, rect?.width || window.innerWidth || 0);
  const h = Math.max(480, rect?.height || window.innerHeight || 0);
  const isPortrait = h >= w;
  const gap = 0; // без разделителя/полей
  const margin = 0; // без верх/низ/боковых полей (иначе «сжимает»)
  // В портретной ориентации foliate сам переключает spread на 1 колонку, поэтому
  // max-inline-size должен быть "на всю ширину", иначе колонка окажется по центру.
  const effectiveDouble = !isPortrait && spreadMode.value === 'double';
  const perColumn = effectiveDouble ? Math.max(240, w / 2) : w;
  // max-inline-size влияет на то, сколько колонок помещается; держим около ширины колонки.
  const maxInlineSize = Math.max(240, perColumn);
  return { margin, gap, maxInlineSize, w, h };
}

function applyRendererLayout(view) {
  const r = view?.renderer;
  if (!r?.style) return;
  if (String(r.tagName || '').toLowerCase() !== 'foliate-paginator') return;
  const { margin, gap, maxInlineSize } = computeAutoLayout();

  // max-inline-size / margin / gap — это публичные атрибуты foliate-paginator
  // (они прокидываются в CSS vars --_max-inline-size/--_margin/--_gap).
  r.setAttribute('margin', String(margin));
  r.setAttribute('gap', String(gap));
  // Число без unit ок: внутри parseFloat → px логика.
  r.setAttribute('max-inline-size', String(maxInlineSize));
  // Явно задаём var в px, чтобы не было «сжатия по центру» из-за rem-логики.
  r.style.setProperty('--_max-inline-size', `${maxInlineSize}px`);
}

function clearFoliateDocKeyBindings() {
  while (foliateDocKeyCleanups.length) {
    const fn = foliateDocKeyCleanups.pop();
    try {
      if (typeof fn === 'function') fn();
    } catch {}
  }
}

/** Хром читалки (шапка, модалки): не листать страницы с клавиатуры/колеса. */
function eventPathIncludesReaderChrome(e) {
  const path = typeof e.composedPath === 'function' ? e.composedPath() : [];
  for (let i = 0; i < path.length; i++) {
    const n = path[i];
    if (n instanceof Element && n.classList?.contains('reader-chrome')) return true;
  }
  return false;
}

function eventPathHasEditableControl(e) {
  const path = typeof e.composedPath === 'function' ? e.composedPath() : [e.target];
  for (let i = 0; i < path.length; i++) {
    const n = path[i];
    if (!n || typeof n !== 'object') continue;
    const tag = String(n.tagName || '').toLowerCase();
    if (tag === 'input' || tag === 'textarea' || tag === 'select') return true;
    try {
      if (n.isContentEditable) return true;
    } catch {
      //
    }
  }
  return false;
}

function eventPathIncludesTag(e, tagName) {
  const want = tagName.toLowerCase();
  const path = typeof e.composedPath === 'function' ? e.composedPath() : [e.target];
  for (let i = 0; i < path.length; i++) {
    const n = path[i];
    if (n instanceof Element && String(n.tagName || '').toLowerCase() === want) return true;
  }
  return false;
}

const foliateShadowRootsBound = new WeakSet();

function attachFoliateShadowKeyCapture(view) {
  const sr = view?.shadowRoot;
  if (!sr || foliateShadowRootsBound.has(sr)) return;
  foliateShadowRootsBound.add(sr);
  const onShadowKeydown = (e) => {
    if (handleReaderNavKey(e) === 'consume') {
      e.preventDefault();
      e.stopImmediatePropagation();
    }
  };
  sr.addEventListener('keydown', onShadowKeydown, true);
  foliateDocKeyCleanups.push(() => {
    try {
      sr.removeEventListener('keydown', onShadowKeydown, true);
      foliateShadowRootsBound.delete(sr);
    } catch {}
  });
}

/** Внутри iframe vue-book-reader вешает keyup со стрелками — перехватываем keydown/keyup (capture), чтобы не было двойного листания; мышь и клавиатура при этом независимы. */
function bindFoliateDocNavigationKeys(doc) {
  if (!doc || foliateKeyBoundDocs.has(doc)) return;
  foliateKeyBoundDocs.add(doc);
  const isArrowKeyEvent = (ev) =>
    ev.key === 'ArrowUp' ||
    ev.key === 'ArrowDown' ||
    ev.key === 'ArrowLeft' ||
    ev.key === 'ArrowRight' ||
    ev.code === 'ArrowUp' ||
    ev.code === 'ArrowDown' ||
    ev.code === 'ArrowLeft' ||
    ev.code === 'ArrowRight';
  const onDocKeydown = (e) => {
    if (handleReaderNavKey(e) === 'consume') {
      e.preventDefault();
      e.stopImmediatePropagation();
    }
  };
  const onDocKeyup = (e) => {
    if (isArrowKeyEvent(e)) e.stopImmediatePropagation();
  };
  doc.addEventListener('keydown', onDocKeydown, true);
  doc.addEventListener('keyup', onDocKeyup, true);
  foliateDocKeyCleanups.push(() => {
    try {
      doc.removeEventListener('keydown', onDocKeydown, true);
      doc.removeEventListener('keyup', onDocKeyup, true);
    } catch {}
    foliateKeyBoundDocs.delete(doc);
  });
}

function onFoliateView(view) {
  if (foliateView && loadHandler) {
    try {
      foliateView.removeEventListener('load', loadHandler);
    } catch {}
  }
  clearFoliateDocKeyBindings();
  foliateView = view;
  /** Фокус часто внутри shadow/open shadow у foliate — window может не получить keydown; capture на хосте ловит те же клавиши. */
  const onHostKeydown = (e) => {
    if (handleReaderNavKey(e) === 'consume') {
      e.preventDefault();
      e.stopImmediatePropagation();
    }
  };
  view.addEventListener('keydown', onHostKeydown, true);
  foliateDocKeyCleanups.push(() => {
    try {
      view.removeEventListener('keydown', onHostKeydown, true);
    } catch {}
  });
  /** Резерв: foliate-view шлёт relocate при смене позиции (если @update:location не сработал). */
  const onRelocate = (e) => {
    const d = e?.detail;
    if (d != null) persistLocation(d);
  };
  view.addEventListener('relocate', onRelocate);
  foliateDocKeyCleanups.push(() => {
    try {
      view.removeEventListener('relocate', onRelocate);
    } catch {}
  });
  loadHandler = ({ detail }) => {
    if (detail?.doc) {
      injectTextAlign(detail.doc);
      bindFoliateDocNavigationKeys(detail.doc);
    }
    requestAnimationFrame(() => attachFoliateShadowKeyCapture(view));
  };
  view.addEventListener('load', loadHandler);
  requestAnimationFrame(() => {
    applySpread(view);
    applyRendererLayout(view);
    refreshTextAlignAll();
    attachFoliateShadowKeyCapture(view);
  });

  // Авто-подстройка при повороте/ресайзе.
  if (resizeHandler) window.removeEventListener('resize', resizeHandler);
  resizeHandler = () => {
    applySpread(foliateView);
    applyRendererLayout(foliateView);
  };
  window.addEventListener('resize', resizeHandler, { passive: true });

  // И точнее — при изменении размеров самого renderer (в т.ч. когда скрывается/показывается toolbar).
  try {
    ro?.disconnect();
    ro = new ResizeObserver(() => {
      applySpread(foliateView);
      applyRendererLayout(foliateView);
    });
    if (view?.renderer) ro.observe(view.renderer);
  } catch {
    ro = null;
  }
}

function goPrev() {
  if (showCoverPage.value) {
    // На обложке "назад" просто закрывает/прячет обложку
    closeCoverPage();
    return;
  }
  if (!foliateView) return;
  // goLeft/goRight учитывает RTL, если доступно
  if (typeof foliateView.goLeft === 'function') foliateView.goLeft();
  else void foliateView.prev?.();
}

function goNext() {
  if (showCoverPage.value) {
    closeCoverPage();
    // Вперёд после обложки — к началу текста
    if (foliateView?.setLocation) {
      try {
        foliateView.setLocation?.(null);
      } catch {}
    }
    return;
  }
  if (!foliateView) return;
  if (typeof foliateView.goRight === 'function') foliateView.goRight();
  else void foliateView.next?.();
}

function onWheel(e) {
  if (!e) return;
  if (e.target instanceof Element && e.target.closest('.reader-chrome')) return;
  const dy = e.deltaY || 0;
  const dx = e.deltaX || 0;
  // Трекпад: горизонтальный жест тоже листаем
  const primary = Math.abs(dy) >= Math.abs(dx) ? dy : dx;
  if (!primary) return;
  e.preventDefault();
  if (primary > 0) goNext();
  else goPrev();
}

function clearStageNavBindings() {
  if (typeof stageNavCleanup === 'function') {
    stageNavCleanup();
    stageNavCleanup = null;
  }
}

function bindStageNav(el) {
  clearStageNavBindings();
  if (!el) return;

  const wheelHandler = (e) => {
    if (!(e instanceof WheelEvent)) return;
    onWheel(e);
  };

  el.addEventListener('wheel', wheelHandler, { passive: false, capture: true });

  let t0x = 0;
  let t0y = 0;
  let tracking = false;

  const touchStart = (e) => {
    if (e.target instanceof Element && e.target.closest('.reader-chrome')) return;
    if (!e.touches || e.touches.length !== 1) return;
    t0x = e.touches[0].clientX;
    t0y = e.touches[0].clientY;
    tracking = true;
  };

  const touchEnd = (e) => {
    if (!tracking) return;
    tracking = false;
    const t = e.changedTouches && e.changedTouches[0];
    if (!t) return;
    const dy = t.clientY - t0y;
    const dx = t.clientX - t0x;
    const threshold = 36;
    if (Math.abs(dy) < threshold || Math.abs(dy) < Math.abs(dx)) return;
    e.preventDefault();
    if (dy > 0) goPrev();
    else goNext();
  };

  /** На мобильном вертикальный свайп — в tap-layer; здесь тач только для планшетов/десктопа с сенсором. */
  const touchBound = !isMobile.value;
  if (touchBound) {
    el.addEventListener('touchstart', touchStart, { passive: true, capture: true });
    el.addEventListener('touchend', touchEnd, { passive: false, capture: true });
  }

  stageNavCleanup = () => {
    el.removeEventListener('wheel', wheelHandler, { capture: true });
    if (touchBound) {
      el.removeEventListener('touchstart', touchStart, { capture: true });
      el.removeEventListener('touchend', touchEnd, { capture: true });
    }
  };
}

watch([readerStageRef, bookUrl, fatalError, isMobile], () => {
  clearStageNavBindings();
  if (fatalError.value || !bookUrl.value) return;
  nextTick(() => {
    const el = readerStageRef.value;
    if (el) bindStageNav(el);
  });
});

function toggleControls() {
  controlsVisible.value = !controlsVisible.value;
}

function viewportTapAction(clientX) {
  const w = window.innerWidth || 1;
  const x = clientX / w;
  // Как в liberama: зоны слева/справа листают, центр — UI.
  if (x <= 0.33) return 'prev';
  if (x >= 0.67) return 'next';
  return 'toggle';
}

function handleTap(clientX) {
  const action = viewportTapAction(clientX);
  if (action === 'prev') goPrev();
  else if (action === 'next') goNext();
  else toggleControls();
}

function onTapStart(e) {
  const t = e.touches?.[0];
  if (!t) return;
  tapActive.value = true;
  tapMoved.value = false;
  tapStartX.value = t.clientX;
  tapStartY.value = t.clientY;
}

function onTapMove(e) {
  if (!tapActive.value) return;
  const t = e.touches?.[0];
  if (!t) return;
  const dx = Math.abs(t.clientX - tapStartX.value);
  const dy = Math.abs(t.clientY - tapStartY.value);
  if (dx > 12 || dy > 12) tapMoved.value = true;
}

function onTapEnd(e) {
  if (!tapActive.value) return;
  tapActive.value = false;
  const t = e.changedTouches?.[0];
  if (!t) return;
  const dy = t.clientY - tapStartY.value;
  const dx = t.clientX - tapStartX.value;
  const swipeTh = 36;
  if (Math.abs(dy) >= swipeTh && Math.abs(dy) > Math.abs(dx)) {
    if (dy > 0) goPrev();
    else goNext();
    return;
  }
  if (tapMoved.value) return;
  handleTap(t.clientX);
}

function onTapCancel() {
  tapActive.value = false;
  tapMoved.value = false;
}

function onClickTap(e) {
  // На случай desktop-подобного клика в mobile-webview
  if (!isMobile.value) return;
  handleTap(e.clientX);
}

function setSpread(mode) {
  spreadMode.value = mode;
  try {
    localStorage.setItem(LS_SPREAD, mode);
  } catch {}
  applySpread(foliateView);
}

function setAlign(mode) {
  textAlign.value = mode;
  try {
    localStorage.setItem(LS_ALIGN, mode);
  } catch {}
  refreshTextAlignAll();
}

watch(textAlign, () => refreshTextAlignAll());

watch(showCoverPage, (open) => {
  if (!open) return;
  nextTick(() => {
    try {
      coverFirstInnerRef.value?.focus({ preventScroll: true });
    } catch {
      try {
        coverFirstInnerRef.value?.focus();
      } catch {}
    }
  });
});

watch([controlsVisible, isMobile], () => {
  if (isMobile.value && !controlsVisible.value) settingsSheetOpen.value = false;
});

watch(activeBookId, async (newId, oldId) => {
  if (!newId || !oldId || newId === oldId) return;
  fatalError.value = '';
  metaTitle.value = '';
  metaAuthor.value = '';
  metaFormat.value = '';
  metaFileName.value = '';
  metaZip.value = '';
  coverOpen.value = false;
  settingsSheetOpen.value = false;
  try {
    savedCFI.value = localStorage.getItem(cfiKey()) || null;
  } catch {
    savedCFI.value = null;
  }
  await loadMeta();
  if (!fatalError.value) await syncProgressFromServer();
  if (!fatalError.value) notifyParent('embedded-reader-ready');
});

function persistLocation(payload) {
  const loc = extractProgressLocation(payload);
  if (!loc || !activeBookId.value) return;
  pendingCfiForFlush = loc;
  try {
    localStorage.setItem(cfiKey(), loc);
  } catch {
    //
  }
  clearTimeout(persistTimer);
  persistTimer = window.setTimeout(() => {
    const body = {
      book_id: activeBookId.value,
      position: {
        cfi: loc,
        savedAt: Date.now(),
        reader: 'foliate',
        readerInstanceId: getOrCreateReaderInstanceId(),
      },
    };
    fetch('/api/reading-progress', {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        ...progressFetchHeaders(),
      },
      body: JSON.stringify(body),
      credentials: 'same-origin',
    }).catch(() => {});
  }, 450);
}

function flushProgressOnLeave() {
  clearTimeout(persistTimer);
  persistTimer = 0;
  const cfi = pendingCfiForFlush;
  const id = activeBookId.value;
  if (!cfi || !id) return;
  try {
    localStorage.setItem(cfiKey(), cfi);
  } catch {}
  const body = {
    book_id: id,
    position: {
      cfi,
      savedAt: Date.now(),
      reader: 'foliate',
      readerInstanceId: getOrCreateReaderInstanceId(),
    },
  };
  try {
    fetch('/api/reading-progress', {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        ...progressFetchHeaders(),
      },
      body: JSON.stringify(body),
      credentials: 'same-origin',
      keepalive: true,
    });
  } catch {
    //
  }
}

function isArrowLeft(e) {
  return e.key === 'ArrowLeft' || e.code === 'ArrowLeft';
}
function isArrowRight(e) {
  return e.key === 'ArrowRight' || e.code === 'ArrowRight';
}
function isArrowUp(e) {
  return e.key === 'ArrowUp' || e.code === 'ArrowUp';
}
function isArrowDown(e) {
  return e.key === 'ArrowDown' || e.code === 'ArrowDown';
}
function isPageUp(e) {
  return e.key === 'PageUp' || e.code === 'PageUp';
}
function isPageDown(e) {
  return e.key === 'PageDown' || e.code === 'PageDown';
}
function isSpaceNav(e) {
  return e.key === ' ' || e.code === 'Space';
}

/** @returns {'consume' | 'ignore'} */
function handleReaderNavKey(e) {
  if (eventPathHasEditableControl(e)) return 'ignore';
  if (settingsSheetOpen.value) {
    if (e.key === 'Escape') {
      settingsSheetOpen.value = false;
      return 'consume';
    }
    if (
      isArrowLeft(e) ||
      isArrowRight(e) ||
      isArrowUp(e) ||
      isArrowDown(e) ||
      isPageUp(e) ||
      isPageDown(e) ||
      isSpaceNav(e) ||
      e.key === 'Backspace'
    ) {
      return 'consume';
    }
    return 'ignore';
  }
  if (eventPathIncludesReaderChrome(e)) return 'ignore';

  if (showCoverPage.value) {
    if (isArrowDown(e) || isArrowRight(e) || isSpaceNav(e) || e.key === 'Enter') {
      goNext();
      return 'consume';
    }
    if (isArrowUp(e) || isArrowLeft(e) || e.key === 'Backspace') {
      goPrev();
      return 'consume';
    }
    if (e.key === 'Escape') {
      closeCoverPage();
      return 'consume';
    }
    return 'ignore';
  }

  if (!foliateView) return 'ignore';

  if (isArrowLeft(e)) {
    goPrev();
    return 'consume';
  }
  if (isArrowRight(e)) {
    goNext();
    return 'consume';
  }
  if (isArrowUp(e)) {
    goPrev();
    return 'consume';
  }
  if (isArrowDown(e)) {
    goNext();
    return 'consume';
  }
  if (isPageDown(e)) {
    goNext();
    return 'consume';
  }
  if (isPageUp(e)) {
    goPrev();
    return 'consume';
  }
  if (isSpaceNav(e) && !eventPathIncludesTag(e, 'button')) {
    goNext();
    return 'consume';
  }
  if (e.key === 'Backspace') {
    goPrev();
    return 'consume';
  }
  return 'ignore';
}

function onWinKeydown(e) {
  if (handleReaderNavKey(e) === 'consume') {
    e.preventDefault();
    e.stopImmediatePropagation();
  }
}

async function loadMeta() {
  const id = activeBookId.value;
  metaLoading.value = true;
  if (!id) {
    fatalError.value = 'Не указан идентификатор книги (параметр book или id).';
    metaLoading.value = false;
    return;
  }
  try {
    const controller = new AbortController();
    const tid = window.setTimeout(() => controller.abort(), 15000);
    let r;
    try {
      r = await fetch(`/api/book/reader-meta?id=${encodeURIComponent(id)}`, { signal: controller.signal });
    } finally {
      clearTimeout(tid);
    }
    if (!r.ok) {
      fatalError.value = 'Не удалось загрузить метаданные книги.';
      return;
    }
    const d = await r.json();
    metaTitle.value = d.title || '';
    metaAuthor.value = d.author || '';
    metaFormat.value = (d.format || '').toLowerCase();
    metaFileName.value = d.file_name || '';
    metaZip.value = d.zip || '';
    const fmt = metaFormat.value;
    if (fmt && !readerFormatSupported(fmt)) {
      fatalError.value = `Формат «${fmt}» встроенная читалка не обрабатывает. Скачайте файл из каталога или конвертируйте в EPUB/FB2.`;
    }
  } catch (e) {
    if (e?.name === 'AbortError') {
      fatalError.value = 'Превышено время ожидания ответа сервера. Проверьте сеть и перезагрузите страницу.';
    } else {
      fatalError.value = 'Ошибка сети при загрузке метаданных.';
    }
  } finally {
    metaLoading.value = false;
  }
}

function closeReader() {
  emit('close');
  try {
    if (typeof window !== 'undefined' && window.self !== window.top) {
      window.parent.postMessage({ type: 'books-embedded-reader-close' }, window.location.origin);
      return;
    }
    window.close();
  } catch {
    //
  }
}

onMounted(async () => {
  syncGuestReaderProgressCookie();
  const mq = window.matchMedia?.('(max-width: 720px)') || null;
  const setMobile = () => {
    isMobile.value = !!mq?.matches;
    // В мобайле текст должен быть на весь экран, UI скрыт по умолчанию.
    if (isMobile.value) controlsVisible.value = false;
  };
  if (mq) {
    setMobile();
    try {
      mq.addEventListener('change', setMobile);
    } catch {
      // Safari legacy
      mq.addListener?.(setMobile);
    }
  }
  try {
    const cfi = localStorage.getItem(cfiKey());
    if (cfi) savedCFI.value = cfi;
  } catch {}
  await Promise.all([syncProgressFromServer(), loadMeta()]);
  if (fatalError.value) {
    notifyParent('embedded-reader-failed');
    return;
  }
  openCoverAsFirstPageIfAny();
  notifyParent('embedded-reader-ready');
  document.addEventListener('keydown', onWinKeydown, true);
  window.addEventListener('pagehide', flushProgressOnLeave);
  if (!showCoverPage.value) focusReaderStage();
});

onUnmounted(() => {
  flushProgressOnLeave();
  window.removeEventListener('pagehide', flushProgressOnLeave);
  clearStageNavBindings();
  clearFoliateDocKeyBindings();
  document.removeEventListener('keydown', onWinKeydown, true);
  if (resizeHandler) window.removeEventListener('resize', resizeHandler);
  resizeHandler = null;
  try {
    ro?.disconnect();
  } catch {}
  ro = null;
  clearTimeout(persistTimer);
  if (foliateView && loadHandler) {
    try {
      foliateView.removeEventListener('load', loadHandler);
    } catch {}
  }
  foliateView = null;
  loadHandler = null;
});
</script>

<style>
/* Оболочка v2 (rd-*). Сохранены: .tap-layer, .reader-stage, логика зон тапа и колеса/клавиш. */
.reader-shell {
  --safe-top: env(safe-area-inset-top, 0px);
  --safe-bottom: env(safe-area-inset-bottom, 0px);
  --safe-left: env(safe-area-inset-left, 0px);
  --safe-right: env(safe-area-inset-right, 0px);
  --accent: #0d9488;
  --accent-dim: #0f766e;
  --glow: rgba(13, 148, 136, 0.35);
  --r-xl: 24px;
  --r-lg: 18px;
  --r-md: 12px;
  --font: 'Inter', system-ui, sans-serif;
  --shadow: 0 16px 48px rgba(0, 0, 0, 0.2);

  display: flex;
  flex-direction: column;
  height: 100dvh;
  height: 100vh;
  margin: 0;
  position: relative;
  font-family: var(--font);
  -webkit-font-smoothing: antialiased;
  background: #090807;
  color: var(--ink, #fafaf9);
}

.reader-shell[data-theme='light'] {
  --ink: #1c1917;
  --muted: rgba(28, 25, 23, 0.55);
  --glass: rgba(255, 255, 255, 0.82);
  --glass-b: rgba(28, 25, 23, 0.1);
  --page: #fafaf9;
  --page-ink: #1c1917;
  --fab: rgba(255, 255, 255, 0.92);
  --fab2: rgba(28, 25, 23, 0.06);
  --scrim: rgba(15, 23, 23, 0.45);
}

.reader-shell[data-theme='sepia'] {
  --ink: #422006;
  --muted: rgba(66, 32, 6, 0.55);
  --glass: rgba(255, 251, 235, 0.93);
  --glass-b: rgba(120, 53, 15, 0.12);
  --page: #fff7ed;
  --page-ink: #431407;
  --fab: rgba(255, 247, 237, 0.95);
  --fab2: rgba(124, 45, 18, 0.08);
  --scrim: rgba(67, 20, 7, 0.4);
}

.reader-shell[data-theme='dark'] {
  --ink: #e7e5e4;
  --muted: rgba(231, 229, 228, 0.5);
  --glass: rgba(28, 25, 23, 0.88);
  --glass-b: rgba(255, 255, 255, 0.08);
  --page: #1c1917;
  --page-ink: #e7e5e4;
  --fab: rgba(41, 37, 36, 0.92);
  --fab2: rgba(255, 255, 255, 0.06);
  --scrim: rgba(0, 0, 0, 0.55);
}

/* Плавающий HUD */
.rd-hud {
  position: fixed;
  z-index: 70;
  display: flex;
  align-items: center;
  gap: 10px;
  pointer-events: none;
}
.rd-hud > * {
  pointer-events: auto;
}
.rd-hud:not(.rd-hud--dock):not(.rd-hud--solo) {
  top: calc(12px + var(--safe-top));
  left: calc(12px + var(--safe-left));
  flex-direction: column;
}
.rd-hud--dock {
  left: 50%;
  bottom: calc(14px + var(--safe-bottom));
  transform: translateX(-50%);
  flex-direction: row;
  flex-wrap: nowrap;
  max-width: calc(100vw - 24px);
  padding: 8px 12px;
  border-radius: 999px;
  background: var(--glass);
  border: 1px solid var(--glass-b);
  backdrop-filter: blur(18px);
  -webkit-backdrop-filter: blur(18px);
  box-shadow: var(--shadow);
}
.rd-hud--solo {
  left: 50%;
  bottom: calc(18px + var(--safe-bottom));
  transform: translateX(-50%);
}
.rd-hud__ttl {
  margin: 0;
  flex: 1 1 auto;
  min-width: 0;
  max-width: 46vw;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: -0.02em;
  line-height: 1.2;
  color: var(--ink);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  text-align: center;
}

.rd-fab {
  width: 48px;
  height: 48px;
  border: 1px solid var(--glass-b);
  border-radius: 16px;
  background: var(--fab);
  color: var(--ink);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.12);
  transition: transform 0.12s ease, box-shadow 0.18s ease;
}
.rd-fab:active {
  transform: scale(0.94);
}
.rd-fab--sm {
  width: 42px;
  height: 42px;
  border-radius: 14px;
}
.rd-fab--accent {
  border-color: rgba(13, 148, 136, 0.45);
  box-shadow: 0 0 0 1px var(--glow);
}
.rd-svg {
  width: 22px;
  height: 22px;
}

.reader-stage {
  flex: 1 1 auto;
  min-height: 0;
  position: relative;
  outline: none;
  background: var(--page);
  color: var(--page-ink);
  transition: background 0.2s ease, color 0.2s ease;
}
.reader-stage > * {
  height: 100%;
}

.tap-layer {
  position: absolute;
  inset: 0;
  z-index: 50;
  background: transparent;
  touch-action: manipulation;
}

.cover-first.rd-intro {
  position: absolute;
  inset: 0;
  z-index: 60;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  padding-bottom: calc(20px + var(--safe-bottom));
  background: radial-gradient(ellipse 100% 60% at 50% -10%, rgba(13, 148, 136, 0.12), transparent), var(--page);
  color: var(--page-ink);
}
.cover-first-inner.rd-intro__grid {
  width: min(760px, 100%);
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
  align-items: stretch;
}
.rd-intro__art {
  border-radius: var(--r-xl);
  overflow: hidden;
  border: 1px solid var(--glass-b);
  background: var(--fab2);
  box-shadow: var(--shadow);
  min-height: 280px;
}
.cover-first-img.rd-intro__img {
  display: block;
  width: 100%;
  height: 100%;
  min-height: 280px;
  object-fit: contain;
}
.cover-first-meta.rd-intro__card {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 10px;
  padding: 24px;
  border-radius: var(--r-xl);
  background: var(--glass);
  border: 1px solid var(--glass-b);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  box-shadow: var(--shadow);
}
.cover-first-title.rd-intro__h {
  font-size: clamp(1.2rem, 3.2vw, 1.55rem);
  font-weight: 800;
  letter-spacing: -0.03em;
  line-height: 1.15;
}
.cover-first-author.rd-intro__by {
  font-size: 14px;
  opacity: 0.75;
  font-weight: 500;
}
.cover-first-hint.rd-intro__note {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  opacity: 0.55;
}
.rd-cta {
  margin-top: 8px;
  align-self: flex-start;
  border: none;
  border-radius: 999px;
  padding: 12px 22px;
  font-family: inherit;
  font-size: 14px;
  font-weight: 700;
  color: #fff;
  cursor: pointer;
  background: linear-gradient(135deg, var(--accent), var(--accent-dim));
  box-shadow: 0 8px 28px var(--glow);
  transition: transform 0.12s ease;
}
.rd-cta:active {
  transform: scale(0.97);
}
.rd-cta--wide {
  align-self: stretch;
  width: 100%;
  margin-top: 0;
}
@media (max-width: 860px) {
  .cover-first-inner.rd-intro__grid {
    grid-template-columns: 1fr;
  }
}

.cover-overlay.rd-pop {
  position: absolute;
  inset: 0;
  z-index: 80;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  padding-bottom: calc(20px + var(--safe-bottom));
  background: var(--scrim);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
}
.cover-overlay-img.rd-pop__img {
  max-width: min(92vw, 520px);
  max-height: min(86vh, 720px);
  border-radius: var(--r-lg);
  box-shadow: 0 24px 64px rgba(0, 0, 0, 0.45);
}

.rd-screen {
  flex: 1 1 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  padding-bottom: calc(24px + var(--safe-bottom));
  padding-left: calc(24px + var(--safe-left));
  padding-right: calc(24px + var(--safe-right));
}
.rd-screen--err {
  background: radial-gradient(circle at 30% 20%, rgba(220, 38, 38, 0.08), transparent 50%), #090807;
}
.rd-screen--wait {
  background: var(--page);
  color: var(--page-ink);
}
.rd-card {
  width: 100%;
  max-width: 400px;
  padding: 28px 22px;
  border-radius: var(--r-xl);
  background: var(--glass);
  border: 1px solid var(--glass-b);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  box-shadow: var(--shadow);
}
.rd-card__tag {
  margin: 0;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #f87171;
}
.rd-card__msg {
  margin: 12px 0 0;
  font-size: 17px;
  font-weight: 700;
  line-height: 1.35;
  color: var(--ink);
}
.reader-hint {
  color: var(--muted);
  font-size: 13px;
  line-height: 1.55;
}
.rd-card__more {
  margin-top: 12px;
}

.rd-wait {
  text-align: center;
}
.rd-wait__orb {
  width: 52px;
  height: 52px;
  margin: 0 auto;
  border-radius: 50%;
  border: 3px solid var(--glass-b);
  border-top-color: var(--accent);
  animation: rd-spin 0.8s linear infinite;
}
.rd-wait__txt {
  margin: 18px 0 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--muted);
}
@keyframes rd-spin {
  to {
    transform: rotate(360deg);
  }
}

.rd-scrim {
  position: fixed;
  inset: 0;
  z-index: 120;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  background: var(--scrim);
  backdrop-filter: blur(4px);
  -webkit-backdrop-filter: blur(4px);
}
.rd-sheet {
  width: 100%;
  max-width: 480px;
  max-height: min(88dvh, 680px);
  display: flex;
  flex-direction: column;
  border-radius: var(--r-xl) var(--r-xl) 0 0;
  background: var(--glass);
  border: 1px solid var(--glass-b);
  border-bottom: none;
  box-shadow: 0 -20px 60px rgba(0, 0, 0, 0.25);
  animation: rd-up 0.3s cubic-bezier(0.22, 1, 0.36, 1);
}
@keyframes rd-up {
  from {
    transform: translateY(20px);
    opacity: 0.9;
  }
  to {
    transform: translateY(0);
    opacity: 1;
  }
}
.rd-sheet__cap {
  width: 36px;
  height: 4px;
  margin: 10px auto 4px;
  border-radius: 99px;
  background: var(--glass-b);
}
.rd-sheet__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 14px 12px 20px;
}
.rd-sheet__h {
  margin: 0;
  font-size: 18px;
  font-weight: 800;
  letter-spacing: -0.02em;
  color: var(--ink);
}
.rd-sheet__body {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
  padding: 6px 20px 20px;
  -webkit-overflow-scrolling: touch;
}
.rd-sheet__done {
  padding: 12px 20px calc(14px + var(--safe-bottom));
  border-top: 1px solid var(--glass-b);
}
.rd-sec + .rd-sec {
  margin-top: 20px;
}
.rd-sec__lab {
  display: block;
  margin-bottom: 10px;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--muted);
}
.rd-palette {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
}
.rd-tile {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 6px;
  padding: 10px 6px 12px;
  border: 2px solid transparent;
  border-radius: var(--r-md);
  background: var(--fab2);
  cursor: pointer;
  font-family: inherit;
  transition: border-color 0.15s ease, transform 0.12s ease;
}
.rd-tile:active {
  transform: scale(0.98);
}
.rd-tile.on {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--glow);
}
.rd-tile__sw {
  width: 100%;
  aspect-ratio: 1;
  border-radius: 10px;
  border: 1px solid var(--glass-b);
}
.rd-tile__sw--lt {
  background: linear-gradient(145deg, #fff, #d6d3d1);
}
.rd-tile__sw--sp {
  background: linear-gradient(145deg, #fff7ed, #d6c4a8);
}
.rd-tile__sw--dk {
  background: linear-gradient(145deg, #44403c, #1c1917);
}
.rd-tile__tx {
  font-size: 11px;
  font-weight: 700;
  color: var(--ink);
}

.rd-pills {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 5px;
  border-radius: 999px;
  background: var(--fab2);
  border: 1px solid var(--glass-b);
}
.rd-pill {
  flex: 1 1 auto;
  min-width: 0;
  border: none;
  border-radius: 999px;
  padding: 11px 8px;
  font-family: inherit;
  font-size: 12px;
  font-weight: 700;
  color: var(--muted);
  background: transparent;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}
.rd-pill.on {
  background: var(--accent);
  color: #fff;
  box-shadow: 0 4px 16px var(--glow);
}
.rd-linkbtn {
  width: 100%;
  border: 1px dashed var(--glass-b);
  border-radius: var(--r-md);
  padding: 14px;
  font-family: inherit;
  font-size: 14px;
  font-weight: 700;
  color: var(--ink);
  background: transparent;
  cursor: pointer;
}
.rd-linkbtn:active {
  background: var(--fab2);
}

@media (max-width: 720px) {
  .reader-stage button.arrow.pre,
  .reader-stage button.arrow.next {
    display: none !important;
  }
}

.reader-shell ::-webkit-scrollbar {
  width: 4px;
}
.reader-shell ::-webkit-scrollbar-thumb {
  background: var(--glass-b);
  border-radius: 99px;
}
</style>
