/**
 * Один UUID на браузер/профиль (ПК и мобильный): для привязки прогресса к устройству, где начато чтение.
 * Хранится в localStorage, переживает перезапуск вкладки; при очистке сайта — создаётся новый.
 * Совпадает с Liberama (liberama-reader-device-id) после первого открытия любой читалки.
 */
const LS_KEY = 'books-reader-instance-id';
const LS_KEY_LEGACY_LIBERAMA = 'liberama-reader-device-id';

function randomId() {
  try {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID();
    }
  } catch {
    //
  }
  return `rid-${Date.now()}-${Math.random().toString(36).slice(2, 14)}`;
}

export function getOrCreateReaderInstanceId() {
  try {
    let id = localStorage.getItem(LS_KEY);
    if (typeof id === 'string' && id.length >= 16) {
      try {
        if (localStorage.getItem(LS_KEY_LEGACY_LIBERAMA) !== id) localStorage.setItem(LS_KEY_LEGACY_LIBERAMA, id);
      } catch {
        //
      }
      return id;
    }
    id = localStorage.getItem(LS_KEY_LEGACY_LIBERAMA);
    if (typeof id === 'string' && id.length >= 16) {
      localStorage.setItem(LS_KEY, id);
      return id;
    }
    id = randomId();
    localStorage.setItem(LS_KEY, id);
    try {
      localStorage.setItem(LS_KEY_LEGACY_LIBERAMA, id);
    } catch {
      //
    }
    return id;
  } catch {
    return randomId();
  }
}

/** Дублируем ключ в cookie для GET /api/reading-progress без явного заголовка (тот же origin). */
export function syncGuestReaderProgressCookie() {
  const id = getOrCreateReaderInstanceId();
  try {
    document.cookie = `books_guest_reader=${id}; path=/; max-age=${86400 * 400}; SameSite=Lax`;
  } catch {
    //
  }
}
