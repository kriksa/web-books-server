import { ref } from 'vue';

export function useFavorites({ token, fetchWithAuth, currentView }) {
  const favoriteIds = ref(new Set());
  const favoritesBooks = ref([]);

  const fetchFavorites = async () => {
    if (!token.value) return;
    try {
      const res = await fetchWithAuth('/api/favorites');
      if (res.ok) {
        const data = await res.json();
        favoriteIds.value = new Set((data.book_ids || []).map(Number));
      }
    } catch (e) {
      console.error('Ошибка загрузки избранного', e);
    }
  };

  const toggleFavorite = async (book) => {
    if (!book || !token.value) return;
    const id = book.id;
    const add = !favoriteIds.value.has(id);
    try {
      const url = '/api/favorites';
      const options = {
        method: add ? 'POST' : 'DELETE',
        headers: {
          'Content-Type': 'application/json',
          ...(token.value ? { Authorization: `Bearer ${token.value}` } : {})
        },
        body: JSON.stringify({ book_id: id })
      };
      const res = await fetchWithAuth(url, options);
      if (res.ok) {
        if (add) favoriteIds.value.add(id);
        else {
          favoriteIds.value.delete(id);
          if (currentView.value === 'favorites') {
            favoritesBooks.value = favoritesBooks.value.filter((b) => b.id !== id);
          }
        }
        favoriteIds.value = new Set(favoriteIds.value);
      }
    } catch (e) {
      console.error('Ошибка избранного', e);
    }
  };

  const isFavorite = (book) => book && favoriteIds.value.has(book.id);

  return {
    favoriteIds,
    favoritesBooks,
    fetchFavorites,
    toggleFavorite,
    isFavorite
  };
}
