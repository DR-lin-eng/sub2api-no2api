<template>
<div class="space-y-6">
  <!-- Registration Settings -->
  <div class="card">
    <div
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.registration.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.registration.description") }}
      </p>
    </div>
    <div class="space-y-5 p-6">
      <!-- Enable Registration -->
      <div class="flex items-center justify-between">
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{
            t("admin.settings.registration.enableRegistration")
          }}</label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{
              t("admin.settings.registration.enableRegistrationHint")
            }}
          </p>
        </div>
        <Toggle v-model="form.registration_enabled" />
      </div>

      <!-- Email Verification -->
      <div
        class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{
            t("admin.settings.registration.emailVerification")
          }}</label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.registration.emailVerificationHint") }}
          </p>
        </div>
        <Toggle v-model="form.email_verify_enabled" />
      </div>

      <!-- Email Suffix Whitelist -->
      <div class="border-t border-gray-100 pt-4 dark:border-dark-700">
        <label class="font-medium text-gray-900 dark:text-white">{{
          t("admin.settings.registration.emailSuffixWhitelist")
        }}</label>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{
            t("admin.settings.registration.emailSuffixWhitelistHint")
          }}
        </p>
        <div
          class="mt-3 rounded-lg border border-gray-300 bg-white p-2 dark:border-dark-500 dark:bg-dark-700"
        >
          <div class="flex flex-wrap items-center gap-2">
            <span
              v-for="suffix in registrationEmailSuffixWhitelistTags"
              :key="suffix"
              class="inline-flex items-center gap-1 rounded bg-gray-100 px-2 py-1 text-xs font-mono text-gray-700 dark:bg-dark-600 dark:text-gray-200"
            >
              <span>{{ suffix }}</span>
              <button
                type="button"
                class="rounded-full text-gray-500 hover:bg-gray-200 hover:text-gray-700 dark:text-gray-300 dark:hover:bg-dark-500 dark:hover:text-white"
                @click="
                  removeRegistrationEmailSuffixWhitelistTag(suffix)
                "
              >
                <Icon
                  name="x"
                  size="xs"
                  class="h-3.5 w-3.5"
                  :stroke-width="2"
                />
              </button>
            </span>

            <div
              class="flex min-w-[220px] flex-1 items-center gap-1 rounded border border-transparent px-2 py-1 focus-within:border-primary-300 dark:focus-within:border-primary-700"
            >
              <input
                v-model="registrationEmailSuffixWhitelistDraft"
                type="text"
                class="w-full bg-transparent text-sm font-mono text-gray-900 outline-none placeholder:text-gray-400 dark:text-white dark:placeholder:text-gray-500"
                :placeholder="
                  t(
                    'admin.settings.registration.emailSuffixWhitelistPlaceholder',
                  )
                "
                @input="
                  handleRegistrationEmailSuffixWhitelistDraftInput
                "
                @keydown="
                  handleRegistrationEmailSuffixWhitelistDraftKeydown
                "
                @blur="commitRegistrationEmailSuffixWhitelistDraft"
                @paste="handleRegistrationEmailSuffixWhitelistPaste"
              />
            </div>
          </div>
        </div>
        <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
          {{
            t(
              "admin.settings.registration.emailSuffixWhitelistInputHint",
            )
          }}
        </p>
      </div>

      <!-- Promo Code -->
      <div
        class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{
            t("admin.settings.registration.promoCode")
          }}</label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.registration.promoCodeHint") }}
          </p>
        </div>
        <Toggle v-model="form.promo_code_enabled" />
      </div>

      <!-- Invitation Code -->
      <div
        class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{
            t("admin.settings.registration.invitationCode")
          }}</label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.registration.invitationCodeHint") }}
          </p>
        </div>
        <Toggle v-model="form.invitation_code_enabled" />
      </div>
      <!-- Password Reset - Only show when email verification is enabled -->
      <div
        v-if="form.email_verify_enabled"
        class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{
            t("admin.settings.registration.passwordReset")
          }}</label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.registration.passwordResetHint") }}
          </p>
        </div>
        <Toggle v-model="form.password_reset_enabled" />
      </div>
      <!-- Frontend URL - Only show when password reset is enabled -->
      <div
        v-if="form.email_verify_enabled && form.password_reset_enabled"
        class="border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <label
          class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          {{ t("admin.settings.registration.frontendUrl") }}
        </label>
        <input
          v-model="form.frontend_url"
          type="url"
          class="input"
          :placeholder="
            t('admin.settings.registration.frontendUrlPlaceholder')
          "
        />
        <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.registration.frontendUrlHint") }}
        </p>
      </div>

      <!-- TOTP 2FA -->
      <div
        class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{
            t("admin.settings.registration.totp")
          }}</label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.registration.totpHint") }}
          </p>
          <!-- Warning when encryption key not configured -->
          <p
            v-if="!form.totp_encryption_key_configured"
            class="mt-2 text-sm text-amber-600 dark:text-amber-400"
          >
            {{ t("admin.settings.registration.totpKeyNotConfigured") }}
          </p>
        </div>
        <Toggle
          v-model="form.totp_enabled"
          :disabled="!form.totp_encryption_key_configured"
        />
      </div>

      <!-- Passkey sign-in -->
      <div
        class="border-t border-gray-100 pt-4 dark:border-dark-700"
        data-testid="passkey-settings"
      >
        <div class="flex items-start justify-between gap-4">
          <div>
            <label class="font-medium text-gray-900 dark:text-white">
              {{ t("admin.settings.security.passkey") }}
            </label>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.security.passkeyHint") }}
            </p>
          </div>
          <Toggle
            v-model="form.passkey_enabled"
            data-testid="passkey-toggle"
            :disabled="!form.passkey_configured"
          />
        </div>
        <div
          class="mt-3 rounded-lg border px-3 py-2 text-sm"
          :class="
            form.passkey_configured
              ? 'border-green-200 bg-green-50 text-green-800 dark:border-green-900 dark:bg-green-950/40 dark:text-green-300'
              : 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-300'
          "
          data-testid="passkey-config-status"
        >
          <p class="font-medium">
            {{
              form.passkey_configured
                ? t("admin.settings.security.passkeyConfigured")
                : t("admin.settings.security.passkeyNotConfigured")
            }}
          </p>
          <p class="mt-1 break-all">
            {{ t("admin.settings.security.passkeyRPID") }}:
            {{
              form.passkey_rp_id ||
              t("admin.settings.security.passkeyValueNotConfigured")
            }}
          </p>
          <p class="mt-1 break-all">
            {{ t("admin.settings.security.passkeyOrigins") }}:
            {{
              form.passkey_rp_origins.length > 0
                ? form.passkey_rp_origins.join(", ")
                : t("admin.settings.security.passkeyValueNotConfigured")
            }}
          </p>
          <p v-if="!form.passkey_configured" class="mt-2">
            {{ t("admin.settings.security.passkeyDeploymentHint") }}
          </p>
        </div>
      </div>

      <!-- 敏感操作 step-up 2FA -->
      <div
        class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{
            t("admin.settings.security.stepUp")
          }}</label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.security.stepUpHint") }}
          </p>
        </div>
        <Toggle v-model="form.step_up_enabled" />
      </div>

      <!-- 会话 IP/UA 绑定 -->
      <div
        class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{
            t("admin.settings.security.sessionBinding")
          }}</label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.security.sessionBindingHint") }}
          </p>
        </div>
        <Toggle v-model="form.session_binding_enabled" />
      </div>

      <!-- 审计日志保留天数 -->
      <div
        class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{
            t("admin.settings.security.auditRetention")
          }}</label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.security.auditRetentionHint") }}
          </p>
        </div>
        <input
          v-model.number="form.audit_log_retention_days"
          type="number"
          min="0"
          class="input w-28 text-right"
        />
      </div>
    </div>
  </div>

  <!-- API Key IP ACL Settings -->
  <div class="card">
    <div
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.apiKeyAcl.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.apiKeyAcl.description") }}
      </p>
    </div>
    <div class="space-y-5 p-6">
      <div class="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(240px,360px)] md:items-center">
        <div>
          <label class="font-medium text-gray-900 dark:text-white">
            {{ t("admin.settings.apiKeyAcl.resolutionMode") }}
          </label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.apiKeyAcl.resolutionModeHint") }}
          </p>
        </div>
        <Select
          :modelValue="form.client_ip_resolution_mode"
          @update:modelValue="form.client_ip_resolution_mode = $event as ClientIPResolutionMode"
          :options="clientIPResolutionModeOptions"
          :searchable="false"
        />
      </div>

      <div v-if="form.client_ip_resolution_mode !== 'direct'">
        <label class="mb-1 block text-sm font-medium text-gray-900 dark:text-white">
          {{ t("admin.settings.apiKeyAcl.trustedProxies") }}
        </label>
        <textarea
          v-model="clientIPTrustedProxiesText"
          class="input min-h-24 font-mono text-sm"
          :placeholder="t('admin.settings.apiKeyAcl.trustedProxiesPlaceholder')"
          spellcheck="false"
        ></textarea>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.apiKeyAcl.trustedProxiesHint") }}
        </p>
      </div>

      <dl class="grid gap-3 border-t border-gray-100 pt-4 text-sm dark:border-dark-700 sm:grid-cols-2 lg:grid-cols-4">
        <div>
          <dt class="text-gray-500 dark:text-gray-400">{{ t("admin.settings.apiKeyAcl.activeMode") }}</dt>
          <dd class="mt-1 font-medium text-gray-900 dark:text-white">
            {{ t(`admin.settings.apiKeyAcl.modes.${form.client_ip_resolution_status.mode || form.client_ip_resolution_mode}`) }}
          </dd>
        </div>
        <div>
          <dt class="text-gray-500 dark:text-gray-400">{{ t("admin.settings.apiKeyAcl.customRules") }}</dt>
          <dd class="mt-1 font-medium text-gray-900 dark:text-white">
            {{ form.client_ip_resolution_status.custom_prefix_count }}
          </dd>
        </div>
        <div>
          <dt class="text-gray-500 dark:text-gray-400">{{ t("admin.settings.apiKeyAcl.cloudflareRules") }}</dt>
          <dd class="mt-1 font-medium text-gray-900 dark:text-white">
            {{ form.client_ip_resolution_status.cloudflare_prefix_count }}
          </dd>
        </div>
        <div>
          <dt class="text-gray-500 dark:text-gray-400">{{ t("admin.settings.apiKeyAcl.cloudflareSource") }}</dt>
          <dd class="mt-1 font-medium text-gray-900 dark:text-white">
            {{ t(`admin.settings.apiKeyAcl.sources.${form.client_ip_resolution_status.cloudflare_ranges_source || 'embedded'}`) }}
          </dd>
        </div>
      </dl>
      <p
        v-if="form.client_ip_resolution_status.cloudflare_last_success_at"
        class="text-xs text-gray-500 dark:text-gray-400"
      >
        {{ t("admin.settings.apiKeyAcl.lastRefresh", { time: clientIPLastRefreshText }) }}
      </p>
    </div>
  </div>

  <PanelRateLimitSettingsCard v-if="panelRateLimitSettingsMounted" />

  <!-- Authentication bot protection settings -->
  <div class="card">
    <div
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.turnstile.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.turnstile.description") }}
      </p>
    </div>
    <div class="space-y-5 p-6">
      <div
        v-for="provider in humanVerificationProviders"
        :key="provider.key"
        class="flex items-center justify-between gap-4 border-b border-gray-100 pb-4 last:border-0 last:pb-0 dark:border-dark-700"
      >
        <div>
          <label class="font-medium text-gray-900 dark:text-white">
            {{ t(provider.label) }}
          </label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t(provider.hint) }}
          </p>
        </div>
        <Toggle
          :model-value="form[provider.key]"
          @update:model-value="setHumanVerificationProvider(provider.key, $event)"
        />
      </div>

      <!-- Turnstile Keys - Only show when enabled -->
      <div
        v-if="form.turnstile_enabled"
        class="border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <div class="grid grid-cols-1 gap-6">
          <div>
            <label
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t("admin.settings.turnstile.siteKey") }}
            </label>
            <input
              v-model="form.turnstile_site_key"
              type="text"
              class="input font-mono text-sm"
              placeholder="0x4AAAAAAA..."
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.turnstile.siteKeyHint") }}
              <a
                href="https://dash.cloudflare.com/"
                target="_blank"
                class="text-primary-600 hover:text-primary-500"
                >{{
                  t("admin.settings.turnstile.cloudflareDashboard")
                }}</a
              >
            </p>
          </div>
          <div>
            <label
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t("admin.settings.turnstile.secretKey") }}
            </label>
            <input
              v-model="form.turnstile_secret_key"
              type="password"
              class="input font-mono text-sm"
              placeholder="0x4AAAAAAA..."
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                form.turnstile_secret_key_configured
                  ? t(
                      "admin.settings.turnstile.secretKeyConfiguredHint",
                    )
                  : t("admin.settings.turnstile.secretKeyHint")
              }}
            </p>
          </div>
        </div>
      </div>

      <div
        v-if="form.recaptcha_enabled"
        class="border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <div class="grid grid-cols-1 gap-6">
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.turnstile.siteKey") }}
            </label>
            <input
              v-model="form.recaptcha_site_key"
              type="text"
              class="input font-mono text-sm"
              placeholder="6Lc..."
            />
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.turnstile.secretKey") }}
            </label>
            <input
              v-model="form.recaptcha_secret_key"
              type="password"
              class="input font-mono text-sm"
              placeholder="6Lc..."
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                form.recaptcha_secret_key_configured
                  ? t("admin.settings.turnstile.secretKeyConfiguredHint")
                  : t("admin.settings.turnstile.secretKeyHint")
              }}
            </p>
          </div>
        </div>
      </div>

      <div
        v-if="form.cap_enabled"
        class="border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <div class="grid grid-cols-1 gap-6">
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.turnstile.capEndpoint") }}
            </label>
            <input
              v-model="form.cap_api_endpoint"
              type="url"
              class="input font-mono text-sm"
              placeholder="https://cap.example.com/site-key"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.turnstile.capEndpointHint") }}
            </p>
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.turnstile.secretKey") }}
            </label>
            <input
              v-model="form.cap_secret_key"
              type="password"
              class="input font-mono text-sm"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                form.cap_secret_key_configured
                  ? t("admin.settings.turnstile.secretKeyConfiguredHint")
                  : t("admin.settings.turnstile.secretKeyHint")
              }}
            </p>
          </div>
        </div>
      </div>

      <div
        v-if="form.tencent_captcha_enabled"
        class="border-t border-gray-100 pt-4 dark:border-dark-700"
        data-testid="tencent-captcha-settings"
      >
        <div class="grid grid-cols-1 gap-6">
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.turnstile.tencentRegion") }}
            </label>
            <Select
              :model-value="form.tencent_captcha_region"
              :options="tencentCaptchaRegionOptions"
              :searchable="false"
              data-testid="tencent-captcha-region"
              @update:model-value="form.tencent_captcha_region = $event as string"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.turnstile.tencentRegionHint") }}
            </p>
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.turnstile.tencentAppId") }}
            </label>
            <input
              v-model="form.tencent_captcha_app_id"
              type="text"
              inputmode="numeric"
              class="input font-mono text-sm"
              placeholder="123456789"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.turnstile.tencentAppIdHint") }}
            </p>
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.turnstile.tencentAppSecretKey") }}
            </label>
            <input
              v-model="form.tencent_captcha_app_secret_key"
              type="password"
              class="input font-mono text-sm"
              autocomplete="new-password"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                form.tencent_captcha_app_secret_key_configured
                  ? t("admin.settings.turnstile.secretKeyConfiguredHint")
                  : t("admin.settings.turnstile.tencentAppSecretKeyHint")
              }}
            </p>
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.turnstile.tencentCloudSecretId") }}
            </label>
            <input
              v-model="form.tencent_captcha_cloud_secret_id"
              type="password"
              class="input font-mono text-sm"
              autocomplete="new-password"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                form.tencent_captcha_cloud_secret_id_configured
                  ? t("admin.settings.turnstile.secretKeyConfiguredHint")
                  : t("admin.settings.turnstile.tencentCloudSecretIdHint")
              }}
            </p>
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.turnstile.tencentCloudSecretKey") }}
            </label>
            <input
              v-model="form.tencent_captcha_cloud_secret_key"
              type="password"
              class="input font-mono text-sm"
              autocomplete="new-password"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                form.tencent_captcha_cloud_secret_key_configured
                  ? t("admin.settings.turnstile.secretKeyConfiguredHint")
                  : t("admin.settings.turnstile.tencentCloudSecretKeyHint")
              }}
            </p>
          </div>
        </div>
      </div>

      <div
        v-if="form.aliyun_captcha_enabled"
        class="border-t border-gray-100 pt-4 dark:border-dark-700"
        data-testid="aliyun-captcha-settings"
      >
        <div class="grid grid-cols-1 gap-6 sm:grid-cols-2">
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.aliyunCaptcha.region") }}
            </label>
            <Select
              :model-value="form.aliyun_captcha_region"
              :options="aliyunCaptchaRegionOptions"
              :searchable="false"
              data-testid="aliyun-captcha-region"
              @update:model-value="form.aliyun_captcha_region = $event as string"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.aliyunCaptcha.regionHint") }}
            </p>
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.aliyunCaptcha.prefix") }}
            </label>
            <input
              v-model="form.aliyun_captcha_prefix"
              type="text"
              class="input font-mono text-sm"
              placeholder="14xxxxx"
              data-testid="aliyun-captcha-prefix"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.aliyunCaptcha.prefixHint") }}
            </p>
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.aliyunCaptcha.sceneId") }}
            </label>
            <input
              v-model="form.aliyun_captcha_scene_id"
              type="text"
              class="input font-mono text-sm"
              placeholder="1cxxxxxx"
              data-testid="aliyun-captcha-scene-id"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.aliyunCaptcha.sceneIdHint") }}
            </p>
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.aliyunCaptcha.accessKeyId") }}
            </label>
            <input
              v-model="form.aliyun_captcha_access_key_id"
              type="text"
              class="input font-mono text-sm"
              placeholder="LTAI..."
              autocomplete="off"
              data-testid="aliyun-captcha-access-key-id"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.aliyunCaptcha.accessKeyIdHint") }}
            </p>
          </div>
          <div class="sm:col-span-2">
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.aliyunCaptcha.accessKeySecret") }}
            </label>
            <input
              v-model="form.aliyun_captcha_access_key_secret"
              type="password"
              class="input font-mono text-sm"
              autocomplete="new-password"
              data-testid="aliyun-captcha-access-key-secret"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                form.aliyun_captcha_access_key_secret_configured
                  ? t("admin.settings.aliyunCaptcha.accessKeySecretConfiguredHint")
                  : t("admin.settings.aliyunCaptcha.accessKeySecretHint")
              }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>
</div>
</template>

<script setup lang="ts">
import Icon from '@/common/widgets/icons/Icon.vue'
import PanelRateLimitSettingsCard from '@/features/admin-settings/presentation/widgets/PanelRateLimitSettingsCard.vue'
import Select from '@/common/widgets/forms/Select.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'
import type { ClientIPResolutionMode } from '@/features/admin-settings/data/dtos/systemSettingsDtos'

const { aliyunCaptchaRegionOptions, clientIPLastRefreshText, clientIPResolutionModeOptions, clientIPTrustedProxiesText, commitRegistrationEmailSuffixWhitelistDraft, form, handleRegistrationEmailSuffixWhitelistDraftInput, handleRegistrationEmailSuffixWhitelistDraftKeydown, handleRegistrationEmailSuffixWhitelistPaste, humanVerificationProviders, panelRateLimitSettingsMounted, registrationEmailSuffixWhitelistDraft, registrationEmailSuffixWhitelistTags, removeRegistrationEmailSuffixWhitelistTag, setHumanVerificationProvider, t, tencentCaptchaRegionOptions } = useSettingsPageContext()
</script>
