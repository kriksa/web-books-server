<script setup>
import { computed } from 'vue';

const props = defineProps({
  show: { type: Boolean, default: false },
  theme: { type: String, required: true },
  parseStatus: { type: Object, required: true },
  parseProgressPercent: { type: Number, default: null },
  parseEstimatedTime: { type: String, default: null }
});

const RING_SIZE = 120;
const RING_R = 52;
const RING_C = 2 * Math.PI * RING_R;

const currentFileLabel = computed(() => {
  const raw = (props.parseStatus.currentFile || '').trim();
  if (!raw) return '';
  const parts = raw.split(/[/\\]/);
  return parts[parts.length - 1] || raw;
});

const stageHint = computed(() => {
  const msg = (props.parseStatus.message || '').trim();
  if (!msg) return '';
  const skip = new Set([
    'Парсинг INPX',
    'Обновление базы книг',
    'Подсчет общего прогресса',
    'Выполняется парсинг',
    'Подождите. Выполняется парсинг',
    'Парсинг завершен'
  ]);
  return skip.has(msg) ? '' : msg;
});

const fileLine = computed(() => currentFileLabel.value || stageHint.value || '…');

const percentValue = computed(() => {
  if (props.parseProgressPercent == null) return null;
  return Math.max(0, Math.min(100, Math.round(props.parseProgressPercent)));
});

const percentLabel = computed(() => (percentValue.value != null ? `${percentValue.value}%` : '…'));

const ringOffset = computed(() => {
  if (percentValue.value == null) return RING_C * 0.92;
  return RING_C * (1 - percentValue.value / 100);
});

const isAtFullProgress = computed(() => {
  if (percentValue.value === 100) return true;
  const s = props.parseStatus;
  return s.total > 0 && s.progress >= s.total;
});

const etaLabel = computed(() => {
  if (isAtFullProgress.value) return 'Подождите';
  if (props.parseEstimatedTime) return `осталось ${props.parseEstimatedTime}`;
  return 'осталось: подсчёт…';
});

const themeClass = computed(() => (props.theme === 'dark' ? 'parse-theme--dark' : 'parse-theme--light'));
</script>

<template>
  <Teleport to="body">
    <div
      v-if="show && parseStatus.isParsing"
      :class="['parse-overlay', themeClass]"
      role="status"
      aria-live="polite"
      :aria-label="percentValue != null ? `Выполняется парсинг, ${percentValue}%` : 'Выполняется парсинг'"
    >
      <!-- Один столбец: заголовок → круг → inp → время (одинаково на mobile и desktop) -->
      <div class="parse-overlay__panel">
        <p class="parse-overlay__title">Выполняется парсинг</p>

        <div class="parse-ring">
          <svg
            class="parse-ring__svg"
            :width="RING_SIZE"
            :height="RING_SIZE"
            :viewBox="`0 0 ${RING_SIZE} ${RING_SIZE}`"
            xmlns="http://www.w3.org/2000/svg"
            aria-hidden="true"
          >
            <circle
              class="parse-ring__track"
              :cx="RING_SIZE / 2"
              :cy="RING_SIZE / 2"
              :r="RING_R"
              fill="none"
              stroke-width="8"
            />
            <circle
              class="parse-ring__fill"
              :class="{ 'parse-ring__fill--indeterminate': percentValue == null }"
              :cx="RING_SIZE / 2"
              :cy="RING_SIZE / 2"
              :r="RING_R"
              fill="none"
              stroke-width="8"
              stroke-linecap="round"
              :stroke-dasharray="`${RING_C} ${RING_C}`"
              :stroke-dashoffset="ringOffset"
              :transform="`rotate(-90 ${RING_SIZE / 2} ${RING_SIZE / 2})`"
            />
          </svg>
          <span class="parse-ring__percent">{{ percentLabel }}</span>
        </div>

        <p class="parse-overlay__file">{{ fileLine }}</p>
        <p class="parse-overlay__eta">{{ etaLabel }}</p>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.parse-overlay {
  position: fixed;
  inset: 0;
  z-index: 44;
  pointer-events: none;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
  box-sizing: border-box;
}

.parse-theme--light {
  background: rgba(248, 250, 252, 0.72);
  color: #1e293b;
}

.parse-theme--dark {
  background: rgba(17, 24, 39, 0.78);
  color: #f3f4f6;
}

.parse-overlay__panel {
  display: flex;
  flex-direction: column;
  flex-wrap: nowrap;
  align-items: center;
  justify-content: center;
  width: min(100%, 22rem);
  max-width: 22rem;
  text-align: center;
}

.parse-overlay__title {
  order: 1;
  flex: 0 0 auto;
  margin: 0 0 0.625rem;
  width: 100%;
  font-size: 1rem;
  font-weight: 600;
  line-height: 1.3;
}

.parse-ring {
  order: 2;
  position: relative;
  flex: 0 0 auto;
  width: 7.5rem;
  height: 7.5rem;
  min-width: 7.5rem;
  min-height: 7.5rem;
  margin: 0;
}

.parse-ring__svg {
  display: block;
  width: 7.5rem;
  height: 7.5rem;
  min-width: 7.5rem;
  min-height: 7.5rem;
}

.parse-ring__track {
  stroke: rgba(99, 102, 241, 0.18);
}

.parse-theme--dark .parse-ring__track {
  stroke: rgba(129, 140, 248, 0.22);
}

.parse-ring__fill {
  stroke: #6366f1;
  transition: stroke-dashoffset 0.45s ease;
}

.parse-theme--dark .parse-ring__fill {
  stroke: #818cf8;
}

.parse-ring__fill--indeterminate {
  animation: parse-ring-pulse 1.4s ease-in-out infinite;
}

.parse-ring__percent {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.375rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  line-height: 1;
}

.parse-overlay__file {
  order: 3;
  flex: 0 0 auto;
  margin: 0.875rem 0 0;
  width: 100%;
  font-size: 0.8125rem;
  line-height: 1.45;
  word-break: break-all;
  opacity: 0.9;
}

.parse-overlay__eta {
  order: 4;
  flex: 0 0 auto;
  margin: 0.5rem 0 0;
  width: 100%;
  font-size: 0.875rem;
  line-height: 1.35;
  opacity: 0.88;
}

@media (min-width: 768px) {
  .parse-theme--light {
    background: rgba(248, 250, 252, 0.55);
  }

  .parse-theme--dark {
    background: rgba(17, 24, 39, 0.62);
  }

  .parse-overlay__panel {
    max-width: 26rem;
  }

  .parse-overlay__title {
    margin-bottom: 0.75rem;
    font-size: 1.0625rem;
  }

  .parse-ring,
  .parse-ring__svg {
    width: 8.5rem;
    height: 8.5rem;
    min-width: 8.5rem;
    min-height: 8.5rem;
  }

  .parse-ring__percent {
    font-size: 1.5rem;
  }
}

@media (max-width: 767px) {
  .parse-overlay__panel {
    flex-direction: column;
    align-items: center;
  }

  .parse-overlay__title {
    order: 1;
    margin-bottom: 0.625rem;
  }

  .parse-ring {
    order: 2;
  }

  .parse-overlay__file {
    order: 3;
  }

  .parse-overlay__eta {
    order: 4;
  }
}

@keyframes parse-ring-pulse {
  0%,
  100% {
    stroke-dashoffset: 280;
    opacity: 0.55;
  }
  50% {
    stroke-dashoffset: 200;
    opacity: 1;
  }
}
</style>
