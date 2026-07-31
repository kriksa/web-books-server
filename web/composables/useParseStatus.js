import { ref, computed } from 'vue';

export function useParseStatus() {
  const parseStatus = ref({
    isParsing: false,
    progress: 0,
    total: 0,
    message: '',
    estimatedRemainingSec: 0,
    currentFile: ''
  });
  const parseStatusInFlight = ref(false);

  const parseProgressPercent = computed(() => {
    const s = parseStatus.value;
    if (!s.total || s.total <= 0) return null;
    return Math.round((s.progress / s.total) * 100);
  });

  const parseEstimatedTime = computed(() => {
    const sec = parseStatus.value.estimatedRemainingSec;
    if (!sec || sec <= 0) return null;
    if (sec > 60 * 60 * 24) return null;
    if (sec < 60) return `~${sec} сек`;
    const min = Math.ceil(sec / 60);
    if (min < 60) return `~${min} мин`;
    const h = Math.floor(min / 60);
    const m = min % 60;
    return m > 0 ? `~${h} ч ${m} мин` : `~${h} ч`;
  });

  const fetchParseStatus = async () => {
    if (parseStatusInFlight.value) return;
    parseStatusInFlight.value = true;

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 1500);
    try {
      const res = await fetch('/api/app-status', {
        cache: 'no-store',
        signal: controller.signal
      });
      if (res.ok) {
        const data = await res.json();
        parseStatus.value = {
          isParsing: data.is_parsing || false,
          progress: Number(data.progress || 0),
          total: Number(data.total || 0),
          message: data.message || '',
          estimatedRemainingSec: data.estimated_remaining_sec || 0,
          currentFile: data.current_file || ''
        };
      }
    } catch (e) {
      if (e?.name !== 'AbortError') {
        console.error('Не удалось загрузить статус парсинга', e);
      }
    } finally {
      clearTimeout(timeoutId);
      parseStatusInFlight.value = false;
    }
  };

  return {
    parseStatus,
    parseStatusInFlight,
    parseProgressPercent,
    parseEstimatedTime,
    fetchParseStatus
  };
}
