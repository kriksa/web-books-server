import { Buffer } from 'safe-buffer';
import iconv from 'iconv-lite';
import textUtils from '../../server/core/Reader/BookConverter/textUtils';

function isValidUtf8Buffer(buf) {
    try {
        const u8 = buf instanceof Uint8Array ? buf : Uint8Array.from(buf);
        new TextDecoder('utf-8', { fatal: true }).decode(u8);
        return true;
    } catch (e) {
        return false;
    }
}

function looksLikeFb2Xml(ab) {
    const n = Math.min(ab.byteLength, 12000);
    if (n < 12) return false;
    const u = new Uint8Array(ab, 0, n);
    let s = '';
    for (let i = 0; i < n; i++) {
        const b = u[i];
        if (b === 0) return false;
        s += String.fromCharCode(b);
    }
    return /<FictionBook/i.test(s);
}

/**
 * Same idea as server ConvertFb2.checkEncoding + UTF-16 pre-pass, using Liberama textUtils.getEncoding.
 */
function normalizeFb2Buffer(data) {
    if (!data.length)
        return data;

    if (data[0] === 32) {
        let i = 0;
        while (i < data.length && (data[i] === 32 || data[i] === 9 || data[i] === 10 || data[i] === 13)) {
            i++;
        }
        if (i > 0) {
            data = data.slice(i);
        }
    }

    let enc = textUtils.getEncoding(data.slice(0, Math.min(data.length, 1024)));
    if (enc.indexOf('UTF-16') === 0) {
        data = Buffer.from(iconv.decode(data, enc), 'utf8');
    }

    let result = data;
    let q = '"';
    let left = data.indexOf('<?xml version="1.0"');
    if (left < 0) {
        left = data.indexOf('<?xml version=\'1.0\'');
        q = '\'';
    }

    if (left < 0)
        return result;

    const right = data.indexOf('?>', left);
    if (right < 0)
        return result;

    const head = data.slice(left, right + 2).toString('latin1');
    const m = head.match(/encoding=['"](.*?)['"]/i);
    if (!m)
        return result;

    const declaredEnc = m[1].toLowerCase().trim();
    let calcEncoding = textUtils.getEncoding(data);

    const redecodeAs = (fromEnc) => {
        const decoded = iconv.decode(data, fromEnc);
        return Buffer.from(decoded.replace(m[0], `encoding=${q}utf-8${q}`), 'utf8');
    };

    if (declaredEnc !== 'utf-8') {
        if (calcEncoding.indexOf('ISO-8859') >= 0) {
            calcEncoding = declaredEnc;
        }
        result = redecodeAs(calcEncoding);
        return result;
    }

    const calcLower = String(calcEncoding || '').toLowerCase();
    if (!isValidUtf8Buffer(data) && calcLower && calcLower !== 'utf-8' && calcLower.indexOf('utf-16') !== 0) {
        try {
            result = redecodeAs(calcEncoding);
        } catch (e) {
            result = data;
        }
    }

    return result;
}

/**
 * @param {ArrayBuffer} arrayBuffer
 * @returns {string}
 */
export default function fb2NormalizeStringFromBytes(arrayBuffer) {
    let data = Buffer.from(arrayBuffer);
    data = normalizeFb2Buffer(data);
    return data.toString('utf8');
}

/**
 * For strings that were already mis-decoded by the browser (e.g. UTF-8 bytes read as Latin-1).
 */
export function repairMojibakeUtf8String(str) {
    if (!str || typeof str !== 'string' || str.indexOf('<FictionBook') < 0)
        return str;

    const sample = str.slice(0, Math.min(str.length, 40000));
    const countCyr = (s) => (s.match(/[а-яёА-ЯЁ]/g) || []).length;
    const cyr = countCyr(sample);
    if (cyr > 80)
        return str;

    try {
        const bytes = new Uint8Array(str.length);
        for (let i = 0; i < str.length; i++)
            bytes[i] = str.charCodeAt(i) & 0xff;

        const recovered = new TextDecoder('utf-8').decode(bytes);
        if (recovered.indexOf('<FictionBook') < 0)
            return str;
        const cyr2 = countCyr(recovered.slice(0, sample.length));
        if (cyr2 > cyr + 30 || (cyr < 15 && cyr2 > 40))
            return recovered;
    } catch (e) {
        //
    }
    return str;
}

export { looksLikeFb2Xml };
