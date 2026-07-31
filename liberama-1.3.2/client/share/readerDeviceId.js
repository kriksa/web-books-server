/**
 * Общий с каталожной читалкой (Foliate) ключ: один UUID на браузер для гостевого прогресса и Liberama identity.
 * Старый ключ liberama-reader-device-id подхватывается при миграции.
 */
const LS_KEY_PRIMARY = 'books-reader-instance-id';
const LS_KEY_LEGACY = 'liberama-reader-device-id';

function randomId() {
    try {
        if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
            return crypto.randomUUID();
        }
    } catch (e) {
        //
    }
    return `lr-${Date.now()}-${Math.random().toString(36).slice(2, 14)}`;
}

export function getOrCreateReaderDeviceId() {
    try {
        let id = localStorage.getItem(LS_KEY_PRIMARY);
        if (typeof id === 'string' && id.length >= 16) {
            try {
                if (localStorage.getItem(LS_KEY_LEGACY) !== id) localStorage.setItem(LS_KEY_LEGACY, id);
            } catch (e) {
                //
            }
            return id;
        }
        id = localStorage.getItem(LS_KEY_LEGACY);
        if (typeof id === 'string' && id.length >= 16) {
            localStorage.setItem(LS_KEY_PRIMARY, id);
            return id;
        }
        id = randomId();
        localStorage.setItem(LS_KEY_PRIMARY, id);
        try {
            localStorage.setItem(LS_KEY_LEGACY, id);
        } catch (e) {
            //
        }
        return id;
    } catch (e) {
        return randomId();
    }
}
