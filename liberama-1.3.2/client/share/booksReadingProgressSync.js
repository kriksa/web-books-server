import { getOrCreateReaderDeviceId } from './readerDeviceId';

const DOWNLOAD_RE = /\/download\/(\d+)\//;

export function parseBooksDownloadId(pathOrUrl) {
    if (!pathOrUrl) return null;
    const m = String(pathOrUrl).match(DOWNLOAD_RE);
    if (!m) return null;
    const id = parseInt(m[1], 10);
    return id > 0 ? id : null;
}

function readAuthToken() {
    try {
        return localStorage.getItem('token') || sessionStorage.getItem('token') || '';
    } catch (e) {
        return '';
    }
}

function progressHeaders() {
    const headers = {
        'Content-Type': 'application/json',
        'X-Books-Guest-Reader': getOrCreateReaderDeviceId(),
    };
    const token = readAuthToken();
    if (token) {
        headers.Authorization = `Bearer ${token}`;
    }
    return headers;
}

let timer = 0;
/** @type {{ bookId: number, bookPos: * } | null} */
let pending = null;

function sendNow(bookId, bookPos) {
    if (!bookId || bookPos == null || bookPos === '') return;
    const body = {
        book_id: bookId,
        position: {
            bookPos,
            savedAt: Date.now(),
            reader: 'liberama',
            readerDeviceId: getOrCreateReaderDeviceId(),
        },
    };
    fetch('/api/reading-progress', {
        method: 'PUT',
        headers: progressHeaders(),
        body: JSON.stringify(body),
        credentials: 'same-origin',
        keepalive: true,
    }).catch(() => {});
}

/** Дебаунс PUT в каталожный /api/reading-progress (Foliate и админка используют ту же таблицу). */
export function scheduleBooksReadingProgressSync(bookId, bookPos) {
    if (!bookId || bookPos == null || bookPos === '') return;
    pending = { bookId, bookPos };
    clearTimeout(timer);
    timer = setTimeout(() => {
        const p = pending;
        pending = null;
        timer = 0;
        if (!p) return;
        sendNow(p.bookId, p.bookPos);
    }, 450);
}

export function flushBooksReadingProgressSync() {
    clearTimeout(timer);
    timer = 0;
    const p = pending;
    pending = null;
    if (!p) return;
    sendNow(p.bookId, p.bookPos);
}

export function syncGuestReaderProgressCookie() {
    const id = getOrCreateReaderDeviceId();
    try {
        document.cookie = `books_guest_reader=${id}; path=/; max-age=${86400 * 400}; SameSite=Lax`;
    } catch (e) {
        //
    }
}
