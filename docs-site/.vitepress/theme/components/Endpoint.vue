<script setup lang="ts">
defineProps<{ method: string; path: string }>()
</script>

<template>
  <div class="endpoint">
    <span class="endpoint__method" :class="`is-${method.toLowerCase()}`">{{ method.toUpperCase() }}</span>
    <code class="endpoint__path">{{ path }}</code>
  </div>
</template>
