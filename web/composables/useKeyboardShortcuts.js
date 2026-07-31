/**
 * Регистрирует обработчик горячих клавиш. Возвращает функцию снятия подписки.
 */
export function registerKeyboardShortcuts(ctx) {
  const onKeydown = (e) => {
    const tag = (e.target?.tagName || '').toLowerCase();
    const inInput = tag === 'input' || tag === 'textarea' || tag === 'select';
    if (e.code === 'Slash') {
      if (!inInput && ctx.currentView.value === 'main') {
        e.preventDefault();
        ctx.focusSearch();
      }
      return;
    }
    if (inInput && e.key !== 'Escape') return;

    if (e.key === 'Escape') {
      if (ctx.showUserMenu?.value) {
        ctx.showUserMenu.value = false;
        return;
      }
      if (ctx.selectedBook.value) ctx.closeModal();
      else if (ctx.showHotkeysHelp.value) ctx.showHotkeysHelp.value = false;
      return;
    }
    if (ctx.currentView.value !== 'main' || ctx.isSearchLocked.value) return;

    if (e.key === 'ArrowLeft') {
      e.preventDefault();
      ctx.navigateBook(-1);
      return;
    }
    if (e.key === 'ArrowRight') {
      e.preventDefault();
      ctx.navigateBook(1);
      return;
    }
    if (e.code === 'KeyF') {
      if (ctx.selectedBook.value && !inInput) {
        e.preventDefault();
        ctx.toggleFavorite(ctx.selectedBook.value);
      }
      return;
    }
    if (e.code === 'KeyD') {
      if (ctx.selectedBook.value && !inInput) {
        e.preventDefault();
        window.location.href = `/download/${ctx.selectedBook.value.id}`;
      }
      return;
    }
    if (e.code === 'KeyR') {
      if (ctx.selectedBook.value && ctx.readerSettings.value.enabled && !inInput) {
        e.preventDefault();
        ctx.openReadBook?.();
      }
      return;
    }
    if (e.key === ' ') {
      if (!ctx.selectedBook.value && ctx.flatBooks.value.length > 0 && !inInput) {
        e.preventDefault();
        ctx.selectedBook.value = ctx.flatBooks.value[0];
      }
      return;
    }
  };

  window.addEventListener('keydown', onKeydown);
  return () => window.removeEventListener('keydown', onKeydown);
}
