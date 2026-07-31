import { ref, watch, onMounted } from 'vue';

export function useTheme() {
  const theme = ref('light');

  const toggleTheme = () => {
    theme.value = theme.value === 'dark' ? 'light' : 'dark';
  };

  onMounted(() => {
    theme.value = localStorage.getItem('theme') || 'light';
  });

  watch(theme, (newTheme) => {
    document.documentElement.setAttribute('data-theme', newTheme);
    localStorage.setItem('theme', newTheme);
  });

  return { theme, toggleTheme };
}
