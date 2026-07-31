import { ref, computed, unref } from 'vue';

export function useGroupedBooks(booksRef, booksEpochRef) {
  const expandedAuthors = ref(new Set());
  const expandedSeries = ref(new Set());
  const groupedBooksCache = ref({ key: '', data: null });

  const filteredBooks = computed(() => booksRef.value);

  const invalidateGroupedCache = () => {
    groupedBooksCache.value = { key: '', data: null };
  };

  const groupedBooks = computed(() => {
    const epoch = booksEpochRef != null ? unref(booksEpochRef) : 0;
    const list = booksRef.value;
    const len = list.length;
    const head = len ? `${list[0]?.id ?? ''}` : '';
    const tail = len > 1 ? `${list[len - 1]?.id ?? ''}` : '';
    const cacheKey = `${epoch}_${len}_${expandedAuthors.value.size}_${expandedSeries.value.size}_${head}_${tail}`;

    if (groupedBooksCache.value.key === cacheKey && groupedBooksCache.value.data) {
      return groupedBooksCache.value.data;
    }

    const groups = {};
    const authorsOrder = [];

    filteredBooks.value.forEach((book) => {
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

    const result = authorsOrder.map((author) => {
      const seriesGroups = groups[author];
      const seriesEntries = Object.entries(seriesGroups);

      seriesEntries.sort(([seriesA], [seriesB]) => {
        if (seriesA === 'Без серии' && seriesB !== 'Без серии') return -1;
        if (seriesB === 'Без серии' && seriesA !== 'Без серии') return 1;
        return seriesA.localeCompare(seriesB);
      });

      const processedSeries = seriesEntries.map(([seriesName, booksArray]) => {
        const sortedBooks = [...booksArray].sort((a, b) => {
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
        author,
        seriesGroups: processedSeries
      };
    });

    groupedBooksCache.value = {
      key: cacheKey,
      data: result
    };

    return result;
  });

  const flatBooks = computed(() => {
    const list = [];
    groupedBooks.value.forEach((ag) => {
      ag.seriesGroups.forEach((sg) => {
        sg.books.forEach((b) => list.push(b));
      });
    });
    return list;
  });

  const toggleAuthor = (authorName) => {
    if (expandedAuthors.value.has(authorName)) expandedAuthors.value.delete(authorName);
    else expandedAuthors.value.add(authorName);
    invalidateGroupedCache();
  };

  const toggleSeries = (authorName, seriesName) => {
    const key = `${authorName}_${seriesName}`;
    if (expandedSeries.value.has(key)) expandedSeries.value.delete(key);
    else expandedSeries.value.add(key);
    invalidateGroupedCache();
  };

  const clearExpansion = () => {
    expandedAuthors.value.clear();
    expandedSeries.value.clear();
    invalidateGroupedCache();
  };

  return {
    expandedAuthors,
    expandedSeries,
    groupedBooksCache,
    filteredBooks,
    groupedBooks,
    flatBooks,
    invalidateGroupedCache,
    toggleAuthor,
    toggleSeries,
    clearExpansion
  };
}
