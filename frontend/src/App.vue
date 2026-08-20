<script setup lang="ts">
import { RouterView, useRouter, useRoute } from 'vue-router'
import { computed, defineAsyncComponent, onMounted, onBeforeUnmount, watch } from 'vue'
import Toast from '@/common/widgets/feedback/Toast.vue'
import NavigationProgress from '@/common/widgets/feedback/NavigationProgress.vue'
import { resolveRouteDocumentTitle } from '@/core/routes/title'
import { useAppStore } from '@/core/stores/appStore'
import { useAuthStore } from '@/features/auth/presentation/stores/authStore'
import { useSubscriptionStore } from '@/features/subscriptions/presentation/stores/subscriptionsStore'
import { useAnnouncementStore } from '@/features/announcements/presentation/stores/announcementsStore'
import { useAdminComplianceStore } from '@/features/admin-settings/presentation/stores/adminComplianceStore'
import { useAdminSettingsStore } from '@/features/admin-settings/presentation/stores/adminSettingsStore'
import { getSetupStatus } from '@/features/setup/data/datasources/setupDatasource'
import { useSupportUnreadPolling } from '@/features/support-chat/presentation/composables/useSupportUnreadPolling'
import { updateFavicon } from '@/core/services/branding'
import { isOpaqueDocument } from '@/core/utils/embedded-url'

const AnnouncementPopup = defineAsyncComponent(
  () => import('@/common/widgets/data/AnnouncementPopup.vue'),
)
const AdminComplianceDialog = defineAsyncComponent(
  () => import('@/features/admin-settings/presentation/widgets/AdminComplianceDialog.vue'),
)

const router = useRouter()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()
const subscriptionStore = useSubscriptionStore()
const announcementStore = useAnnouncementStore()
const adminComplianceStore = useAdminComplianceStore()
const adminSettingsStore = useAdminSettingsStore()
const hasAnnouncementPopup = computed(() => announcementStore.currentPopup !== null)
const needsAdminCompliance = computed(
  () => authStore.isAuthenticated && authStore.isAdmin && adminComplianceStore.shouldShow,
)
useSupportUnreadPolling({
  isAuthenticated: () => authStore.isAuthenticated,
  isAdmin: () => authStore.isAdmin,
})

function updateDocumentTitle() {
  const customMenuItems = [
    ...(appStore.cachedPublicSettings?.custom_menu_items ?? []),
    ...(authStore.isAdmin ? adminSettingsStore.customMenuItems : []),
  ]
  document.title = resolveRouteDocumentTitle(route, appStore.siteName, customMenuItems)
}

// Watch for site settings changes and update favicon/title
watch(
  () => appStore.siteLogo,
  (newLogo) => {
    if (newLogo) {
      updateFavicon(newLogo)
    }
  },
  { immediate: true }
)

watch(
  [
    () => route.fullPath,
    () => route.meta.title,
    () => route.meta.titleKey,
    () => appStore.siteName,
    () => appStore.cachedPublicSettings?.custom_menu_items,
    () => authStore.isAdmin,
    () => adminSettingsStore.customMenuItems,
  ],
  updateDocumentTitle,
  { deep: true }
)

// Watch for authentication state and manage subscription data + announcements
function onVisibilityChange() {
  if (document.visibilityState === 'visible' && authStore.isAuthenticated) {
    announcementStore.fetchAnnouncements()
  }
}

function onAdminComplianceRequired(event: Event) {
  const detail = (event as CustomEvent<Record<string, string>>).detail || {}
  adminComplianceStore.requireAcknowledgement(detail)
}

watch(
  () => authStore.isAuthenticated,
  (isAuthenticated, oldValue) => {
    if (isAuthenticated) {
      if (authStore.isAdmin) {
        adminComplianceStore.fetchStatus().catch((error) => {
          console.error('Failed to fetch admin compliance status:', error)
        })
      }

      // User logged in: preload subscriptions and start polling
      subscriptionStore.fetchActiveSubscriptions().catch((error) => {
        console.error('Failed to preload subscriptions:', error)
      })
      subscriptionStore.startPolling()

      // Announcements: new login vs page refresh restore
      if (oldValue === false) {
        // New login: delay 3s then force fetch
        setTimeout(() => announcementStore.fetchAnnouncements(true), 3000)
      } else {
        // Page refresh restore (oldValue was undefined)
        announcementStore.fetchAnnouncements()
      }

      // Register visibility change listener
      document.addEventListener('visibilitychange', onVisibilityChange)
    } else {
      // User logged out: clear data and stop polling
      subscriptionStore.clear()
      announcementStore.reset()
      adminComplianceStore.reset()
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  },
  { immediate: true }
)

// Route change trigger (throttled by store)
router.afterEach(() => {
  if (authStore.isAuthenticated) {
    announcementStore.fetchAnnouncements()
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', onVisibilityChange)
  window.removeEventListener('admin-compliance-required', onAdminComplianceRequired)
})

onMounted(async () => {
  window.addEventListener('admin-compliance-required', onAdminComplianceRequired)

  if (isOpaqueDocument()) return

  // Check if setup is needed
  try {
    const status = await getSetupStatus()
    if (status.needs_setup && route.path !== '/setup') {
      await router.replace('/setup')
      return
    }
  } catch {
    // If setup endpoint fails, assume normal mode and continue
  }

  // Load public settings into appStore (will be cached for other components)
  await appStore.fetchPublicSettings()

  // Re-resolve document title now that site settings are available
  updateDocumentTitle()
})
</script>

<template>
  <NavigationProgress />
  <RouterView />
  <Toast />
  <AnnouncementPopup v-if="hasAnnouncementPopup" />
  <AdminComplianceDialog v-if="needsAdminCompliance" />
</template>
