import { ref, nextTick, onUnmounted } from 'vue';
import { getImageId } from '../utils/helpers.js';

export function useImageLazyLoad() {
  const visibleImages = ref(new Set());
  const imageErrors = ref(new Set());
  let imageObserver = null;

  const disconnect = () => {
    if (imageObserver) {
      imageObserver.disconnect();
      imageObserver = null;
    }
  };

  const refreshObservers = () => {
    disconnect();

    imageObserver = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            const imgId = entry.target.dataset.imgId;
            if (imgId) {
              visibleImages.value.add(imgId);
              imageObserver.unobserve(entry.target);
            }
          }
        });
      },
      {
        root: null,
        rootMargin: '50px',
        threshold: 0.1
      }
    );

    nextTick(() => {
      const placeholders = document.querySelectorAll('.book-cover-placeholder');
      placeholders.forEach((placeholder) => {
        imageObserver.observe(placeholder);
      });
    });
  };

  const shouldLoadImage = (book) => visibleImages.value.has(getImageId(book));

  const hasImageError = (book) => imageErrors.value.has(`${book.fileName}-${book.zip}`);

  const handleImageError = (_event, book) => {
    imageErrors.value = new Set(imageErrors.value).add(`${book.fileName}-${book.zip}`);
  };

  const handleImageLoad = (_event, book) => {
    if (book) imageErrors.value.delete(`${book.fileName}-${book.zip}`);
  };

  onUnmounted(() => {
    disconnect();
  });

  return {
    visibleImages,
    imageErrors,
    refreshObservers,
    disconnect,
    getImageId,
    shouldLoadImage,
    hasImageError,
    handleImageError,
    handleImageLoad
  };
}
