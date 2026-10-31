<template>
  <MediaStudioWorkspace v-if="ownerID" :key="ownerID" :owner-i-d="ownerID" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useAuthStore } from '@/features/auth'
import { normalizeMediaStudioOwnerID } from '@/features/media-studio/data/datasources/mediaStudioImageStore'
import MediaStudioWorkspace from '@/features/media-studio/presentation/widgets/MediaStudioWorkspace.vue'

const auth = useAuthStore()
const ownerID = computed(() => normalizeMediaStudioOwnerID(auth.user?.id))
</script>
