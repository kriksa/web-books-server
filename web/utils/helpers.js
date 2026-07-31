import { LANGUAGES_MAP } from '../data/languages.js';
import { GENRES_MAP } from '../data/genres.js';

export const sanitizeFilename = (name) => {
  if (!name) return 'book';
  return name.trim().replace(/[^a-zA-Z0-9а-яА-ЯёЁ\s\-\.]/g, '').replace(/\s+/g, '_');
};

export const formatFileSize = (bytes) => (bytes / 1024 / 1024).toFixed(2) + ' МБ';

export const getLanguageName = (langCode) => LANGUAGES_MAP[langCode] || langCode;

export const getGenreNames = (genreCodes) => {
  if (!genreCodes || typeof genreCodes !== 'string' || genreCodes.trim() === '') return '—';
  const codes = genreCodes.split(',').map(code => code.trim()).filter(Boolean);
  const names = codes.map(code => {
    const normalized = code.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '');
    return (GENRES_MAP && GENRES_MAP[normalized]) ? GENRES_MAP[normalized] : code;
  });
  return names.join(', ');
};

export const getCoverUrl = (book) => {
  if (book.format === 'fb2' || book.format === 'epub') {
    return `/api/cover?file=${book.fileName}&zip=${book.zip}&format=${book.format}`;
  }
  return null;
};

export const getImageId = (book) => `${book.id}-${book.fileName}-${book.zip}`;
