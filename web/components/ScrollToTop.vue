<script setup>
import { ArrowUp } from 'lucide-vue-next';

defineEmits(['click']);
</script>

<template>
  <button
    type="button"
    class="fixed bottom-6 right-6 p-3 rounded-full shadow-lg transition-all hover:scale-110 bg-indigo-600 text-white"
    @click="$emit('click')"
  >
    <ArrowUp class="h-5 w-5" />
  </button>
</template>
