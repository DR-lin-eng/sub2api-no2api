<template>
  <div class="space-y-6">
    <!-- Codex OAuth A/B Simulation Settings -->
    <SettingsCodexSimulationCard />

    <!-- Global Temporary Unschedulable Settings -->
    <SettingsGlobalTempUnschedulableCard />

    <!-- Overload Cooldown (529) Settings -->
    <SettingsOverloadCooldownCard />

    <!-- Rate Limit Cooldown (429) Settings -->
    <SettingsRateLimit429CooldownCard />

    <!-- Stream Timeout Settings -->
    <SettingsStreamTimeoutCard />

    <!-- Request Rectifier Settings -->
    <SettingsRequestRectifierCard />

    <!-- Beta Policy Settings -->
    <SettingsBetaPolicyCard />

    <!-- OpenAI Fast/Flex Policy Settings -->
    <SettingsOpenAIFastPolicyCard />
  </div>
</template>

<script setup lang="ts">
import SettingsCodexSimulationCard from './gateway-resilience/SettingsCodexSimulationCard.vue'
import SettingsGlobalTempUnschedulableCard from './gateway-resilience/SettingsGlobalTempUnschedulableCard.vue'
import SettingsOverloadCooldownCard from './gateway-resilience/SettingsOverloadCooldownCard.vue'
import SettingsRateLimit429CooldownCard from './gateway-resilience/SettingsRateLimit429CooldownCard.vue'
import SettingsStreamTimeoutCard from './gateway-resilience/SettingsStreamTimeoutCard.vue'
import SettingsRequestRectifierCard from './gateway-resilience/SettingsRequestRectifierCard.vue'
import SettingsBetaPolicyCard from './gateway-resilience/SettingsBetaPolicyCard.vue'
import SettingsOpenAIFastPolicyCard from './gateway-resilience/SettingsOpenAIFastPolicyCard.vue'
</script>
