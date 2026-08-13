<template>
<div class="space-y-6">
    <!-- Gateway Forwarding Behavior -->
    <div class="card">
      <div
        class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
      >
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t("admin.settings.gatewayForwarding.title") }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.gatewayForwarding.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div class="flex items-center justify-between gap-4">
          <div>
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.gatewayForwarding.openAIWSModeRouterV2") }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.gatewayForwarding.openAIWSModeRouterV2Hint") }}
            </p>
          </div>
          <Toggle
            v-model="form.openai_ws_mode_router_v2_enabled"
            data-testid="openai-ws-mode-router-v2-toggle"
          />
        </div>

        <div class="flex items-center justify-between border-t border-gray-100 pt-5 dark:border-dark-700">
          <div>
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.gatewayForwarding.openAIVisibleOutputTTFT") }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.gatewayForwarding.openAIVisibleOutputTTFTHint") }}
            </p>
          </div>
          <Toggle
            v-model="form.openai_visible_output_ttft_enabled"
            data-testid="openai-visible-output-ttft-toggle"
          />
        </div>

        <!-- Fingerprint Unification -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.fingerprintUnification",
                )
              }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                t(
                  "admin.settings.gatewayForwarding.fingerprintUnificationHint",
                )
              }}
            </p>
          </div>
          <Toggle v-model="form.enable_fingerprint_unification" />
        </div>

        <!-- Metadata Passthrough -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{
                t("admin.settings.gatewayForwarding.metadataPassthrough")
              }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                t(
                  "admin.settings.gatewayForwarding.metadataPassthroughHint",
                )
              }}
            </p>
          </div>
          <Toggle v-model="form.enable_metadata_passthrough" />
        </div>

        <!-- CCH Signing -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t("admin.settings.gatewayForwarding.cchSigning") }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.gatewayForwarding.cchSigningHint") }}
            </p>
          </div>
          <Toggle v-model="form.enable_cch_signing" />
        </div>

        <!-- Claude OAuth System Prompt Injection -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.claudeOAuthSystemPromptInjection",
                )
              }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                t(
                  "admin.settings.gatewayForwarding.claudeOAuthSystemPromptInjectionHint",
                )
              }}
            </p>
          </div>
          <Toggle
            v-model="form.enable_claude_oauth_system_prompt_injection"
          />
        </div>

        <div>
          <label
            class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{
              t(
                "admin.settings.gatewayForwarding.claudeOAuthSystemPromptBlocks",
              )
            }}
          </label>
          <div class="space-y-3">
            <div
              v-for="(block, index) in claudeOAuthSystemPromptBlocks"
              :key="block.id"
              class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/60"
            >
              <div
                :class="[
                  'flex flex-wrap items-center justify-between gap-3',
                  block.expanded && 'mb-3',
                ]"
              >
                <div class="min-w-0">
                  <div
                    class="text-sm font-medium text-gray-900 dark:text-white"
                  >
                    {{
                      t(
                        "admin.settings.gatewayForwarding.systemBlockTitle",
                        { index: index + 1 },
                      )
                    }}
                  </div>
                  <div
                    class="mt-0.5 text-xs text-gray-500 dark:text-gray-400"
                  >
                    {{ getClaudeOAuthPresetLabel(block.preset) }}
                  </div>
                </div>
                <div class="flex items-center gap-2">
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm px-2"
                    :title="
                      block.expanded
                        ? t(
                            'admin.settings.gatewayForwarding.systemBlockHide',
                          )
                        : t(
                            'admin.settings.gatewayForwarding.systemBlockShow',
                          )
                    "
                    :aria-label="
                      block.expanded
                        ? t(
                            'admin.settings.gatewayForwarding.systemBlockHide',
                          )
                        : t(
                            'admin.settings.gatewayForwarding.systemBlockShow',
                          )
                    "
                    @click="toggleClaudeOAuthSystemPromptBlock(index)"
                  >
                    <Icon
                      :name="block.expanded ? 'eyeOff' : 'eye'"
                      size="xs"
                    />
                  </button>
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm px-2"
                    :disabled="index === 0"
                    @click="moveClaudeOAuthSystemPromptBlock(index, -1)"
                  >
                    <Icon name="arrowUp" size="xs" />
                  </button>
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm px-2"
                    :disabled="
                      index === claudeOAuthSystemPromptBlocks.length - 1
                    "
                    @click="moveClaudeOAuthSystemPromptBlock(index, 1)"
                  >
                    <Icon name="arrowDown" size="xs" />
                  </button>
                  <Toggle v-model="block.enabled" />
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm px-2 text-red-600 hover:text-red-700 dark:text-red-400"
                    @click="removeClaudeOAuthSystemPromptBlock(index)"
                  >
                    <Icon name="trash" size="xs" />
                  </button>
                </div>
              </div>

              <div v-show="block.expanded">
                <div class="grid gap-3 md:grid-cols-2">
                  <div>
                    <label
                      class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300"
                    >
                      {{
                        t(
                          "admin.settings.gatewayForwarding.systemBlockPreset",
                        )
                      }}
                    </label>
                    <Select
                      v-model="block.preset"
                      :options="claudeOAuthSystemPromptPresetOptions"
                      @change="
                        (value) =>
                          applyClaudeOAuthSystemPromptPreset(index, value)
                      "
                    />
                  </div>
                  <div>
                    <label
                      class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300"
                    >
                      {{
                        t(
                          "admin.settings.gatewayForwarding.systemBlockType",
                        )
                      }}
                    </label>
                    <Select
                      v-model="block.type"
                      :options="claudeOAuthSystemPromptBlockTypeOptions"
                    />
                  </div>
                </div>

                <div class="mt-3">
                  <label
                    class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300"
                  >
                    {{ t("admin.settings.gatewayForwarding.systemBlockText") }}
                  </label>
                  <textarea
                    v-model="block.text"
                    rows="6"
                    class="input w-full resize-y font-mono text-xs leading-5"
                    @input="markClaudeOAuthSystemPromptBlockCustom(block)"
                  />
                </div>

                <div
                  class="mt-3 grid gap-3 md:grid-cols-[minmax(0,1fr)_160px]"
                >
                  <div class="flex items-center justify-between gap-4">
                    <div>
                      <label
                        class="text-xs font-medium text-gray-600 dark:text-gray-300"
                      >
                        {{
                          t(
                            "admin.settings.gatewayForwarding.systemBlockCacheControl",
                          )
                        }}
                      </label>
                    </div>
                    <Toggle v-model="block.cacheControlEnabled" />
                  </div>
                  <div v-if="block.cacheControlEnabled">
                    <Select
                      v-model="block.cacheControlTTL"
                      :options="claudeOAuthSystemPromptCacheTTLOptions"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div class="mt-3 flex flex-wrap gap-2">
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              @click="addClaudeOAuthSystemPromptBlock"
            >
              <Icon name="plus" size="xs" />
              {{ t("admin.settings.gatewayForwarding.addSystemBlock") }}
            </button>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              @click="resetClaudeOAuthSystemPromptBlocks"
            >
              <Icon name="refresh" size="xs" />
              {{
                t("admin.settings.gatewayForwarding.resetSystemBlocks")
              }}
            </button>
          </div>
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
            {{
              t(
                "admin.settings.gatewayForwarding.claudeOAuthSystemPromptBlocksHint",
              )
            }}
          </p>
        </div>

        <!-- Anthropic Cache TTL 1h Injection -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.anthropicCacheTTL1hInjection",
                )
              }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                t(
                  "admin.settings.gatewayForwarding.anthropicCacheTTL1hInjectionHint",
                )
              }}
            </p>
          </div>
          <Toggle
            v-model="form.enable_anthropic_cache_ttl_1h_injection"
          />
        </div>

        <!-- messages cache_control 改写 -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.rewriteMessageCacheControl",
                )
              }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                t(
                  "admin.settings.gatewayForwarding.rewriteMessageCacheControlHint",
                )
              }}
            </p>
          </div>
          <Toggle v-model="form.rewrite_message_cache_control" />
        </div>

        <!-- 客户端 dateline 归一化（仅 Anthropic OAuth/SetupToken） -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.clientDatelineNormalization",
                )
              }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                t(
                  "admin.settings.gatewayForwarding.clientDatelineNormalizationHint",
                )
              }}
            </p>
          </div>
          <Toggle
            v-model="form.enable_client_dateline_normalization"
          />
        </div>

        <!-- Antigravity UA 版本 -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{
              t(
                "admin.settings.gatewayForwarding.antigravityUserAgentVersion",
              )
            }}
          </label>
          <input
            v-model="form.antigravity_user_agent_version"
            type="text"
            class="input max-w-xs font-mono text-sm"
            :placeholder="
              t(
                'admin.settings.gatewayForwarding.antigravityUserAgentVersionPlaceholder',
              )
            "
          />
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
            {{
              t(
                "admin.settings.gatewayForwarding.antigravityUserAgentVersionHint",
              )
            }}
          </p>
        </div>

        <!-- OpenAI Codex identity -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{
              t(
                "admin.settings.gatewayForwarding.openaiCodexUserAgent",
              )
            }}
          </label>
          <input
            v-model="form.openai_codex_user_agent"
            type="text"
            class="input w-full font-mono text-sm"
            :placeholder="
              t(
                'admin.settings.gatewayForwarding.openaiCodexUserAgentPlaceholder',
              )
            "
          />
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
            {{
              t(
                "admin.settings.gatewayForwarding.openaiCodexUserAgentHint",
              )
            }}
          </p>
        </div>

        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.openaiCodexClientVersion",
                )
              }}
            </label>
            <input
              v-model="form.openai_codex_client_version"
              data-testid="openai-codex-client-version"
              type="text"
              class="input max-w-xs font-mono text-sm"
              :placeholder="
                t(
                  'admin.settings.gatewayForwarding.openaiCodexClientVersionPlaceholder',
                )
              "
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                t(
                  "admin.settings.gatewayForwarding.openaiCodexClientVersionHint",
                )
              }}
            </p>
          </div>

          <div>
            <label
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.openaiCodexSyncedVersion",
                )
              }}
            </label>
            <div
              data-testid="openai-codex-synced-version"
              class="input flex max-w-xs items-center font-mono text-sm text-gray-600 dark:text-gray-300"
            >
              {{
                form.openai_codex_client_version_synced ||
                t(
                  "admin.settings.gatewayForwarding.openaiCodexSyncedVersionEmpty",
                )
              }}
            </div>
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                t(
                  "admin.settings.gatewayForwarding.openaiCodexSyncedVersionHint",
                )
              }}
            </p>
          </div>
        </div>

        <div class="flex items-center justify-between gap-4">
          <div>
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{
                t(
                  "admin.settings.gatewayForwarding.openaiCodexVersionAutoSync",
                )
              }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                t(
                  "admin.settings.gatewayForwarding.openaiCodexVersionAutoSyncHint",
                )
              }}
            </p>
          </div>
          <Toggle
            v-model="form.openai_codex_version_auto_sync_enabled"
            data-testid="openai-codex-version-auto-sync"
          />
        </div>

      </div>
    </div>

    <!-- Web Search Emulation -->
    <div class="card">
      <div
        class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
      >
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t("admin.settings.webSearchEmulation.title") }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.webSearchEmulation.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <!-- Global Toggle -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t("admin.settings.webSearchEmulation.enabled") }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.webSearchEmulation.enabledHint") }}
            </p>
          </div>
          <Toggle v-model="webSearchConfig.enabled" />
        </div>

        <!-- Providers -->
        <div v-if="webSearchConfig.enabled" class="space-y-4">
          <div class="flex items-center justify-between">
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t("admin.settings.webSearchEmulation.providers") }}
            </label>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              @click="addWebSearchProvider"
            >
              {{ t("admin.settings.webSearchEmulation.addProvider") }}
            </button>
          </div>

          <div
            v-if="webSearchConfig.providers.length === 0"
            class="rounded-lg border border-dashed border-gray-300 p-4 text-center text-sm text-gray-400 dark:border-dark-600"
          >
            {{ t("admin.settings.webSearchEmulation.noProviders") }}
          </div>

          <div
            v-for="(provider, pIdx) in webSearchConfig.providers"
            :key="pIdx"
            class="rounded-lg border border-gray-200 dark:border-dark-600"
          >
            <!-- Collapsible header -->
            <div
              class="flex cursor-pointer items-center justify-between px-4 py-3"
              @click="toggleProviderExpand(pIdx)"
            >
              <div class="flex items-center gap-3">
                <svg
                  class="h-4 w-4 text-gray-400 transition-transform"
                  :class="{ 'rotate-90': expandedProviders[pIdx] }"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M9 5l7 7-7 7"
                  />
                </svg>
                <Select
                  v-model="provider.type"
                  :options="[
                    { value: 'brave', label: 'Brave Search' },
                    { value: 'tavily', label: 'Tavily' },
                  ]"
                  class="w-36"
                  @click.stop
                />
                <!-- Quota summary (always visible) -->
                <span class="text-xs text-gray-400">
                  {{ provider.quota_used ?? 0 }} /
                  {{
                    provider.quota_limit != null &&
                    provider.quota_limit > 0
                      ? provider.quota_limit
                      : "∞"
                  }}
                </span>
                <span
                  v-if="
                    !expandedProviders[pIdx] &&
                    provider.api_key_configured
                  "
                  class="text-xs text-green-500"
                >
                  {{
                    t(
                      "admin.settings.webSearchEmulation.apiKeyConfigured",
                    )
                  }}
                </span>
              </div>
              <button
                type="button"
                class="text-red-500 hover:text-red-700 text-xs"
                @click.stop="removeWebSearchProvider(pIdx)"
              >
                {{
                  t("admin.settings.webSearchEmulation.removeProvider")
                }}
              </button>
            </div>

            <!-- Expanded content -->
            <div
              v-if="expandedProviders[pIdx]"
              class="space-y-3 border-t border-gray-100 px-4 pb-4 pt-3 dark:border-dark-700"
            >
              <!-- API Key with inline show/copy -->
              <div>
                <label class="text-xs text-gray-500">{{
                  t("admin.settings.webSearchEmulation.apiKey")
                }}</label>
                <div class="relative">
                  <input
                    v-model="provider.api_key"
                    :type="apiKeyVisible[pIdx] ? 'text' : 'password'"
                    class="input w-full text-sm"
                    :class="
                      provider.api_key || provider.api_key_configured
                        ? 'pr-16'
                        : ''
                    "
                    :placeholder="
                      provider.api_key_configured
                        ? '••••••••'
                        : t(
                            'admin.settings.webSearchEmulation.apiKeyPlaceholder',
                          )
                    "
                  />
                  <div
                    v-if="provider.api_key || provider.api_key_configured"
                    class="absolute inset-y-0 right-0 flex items-center pr-1.5"
                  >
                    <button
                      type="button"
                      class="rounded p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                      :title="
                        apiKeyVisible[pIdx]
                          ? t(
                              'admin.settings.webSearchEmulation.hideApiKey',
                            )
                          : t(
                              'admin.settings.webSearchEmulation.showApiKey',
                            )
                      "
                      @click="apiKeyVisible[pIdx] = !apiKeyVisible[pIdx]"
                    >
                      <svg
                        v-if="!apiKeyVisible[pIdx]"
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                        />
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"
                        />
                      </svg>
                      <svg
                        v-else
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21"
                        />
                      </svg>
                    </button>
                    <button
                      type="button"
                      class="rounded p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                      :class="{
                        'opacity-30 cursor-not-allowed':
                          !provider.api_key,
                      }"
                      :title="
                        t('admin.settings.webSearchEmulation.copyApiKey')
                      "
                      :disabled="!provider.api_key"
                      @click="copyApiKey(pIdx)"
                    >
                      <svg
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                        />
                      </svg>
                    </button>
                  </div>
                </div>
              </div>

              <!-- Quota + Subscription in compact row -->
              <div class="grid grid-cols-2 gap-3">
                <div>
                  <label class="text-xs text-gray-500">{{
                    t("admin.settings.webSearchEmulation.quotaLimit")
                  }}</label>
                  <input
                    v-model="provider.quota_limit"
                    type="number"
                    min="1"
                    class="input text-sm"
                    :placeholder="'∞'"
                  />
                  <p class="mt-0.5 text-xs text-gray-400">
                    {{
                      t(
                        "admin.settings.webSearchEmulation.quotaLimitHint",
                      )
                    }}
                  </p>
                </div>
                <div>
                  <label class="text-xs text-gray-500">{{
                    t("admin.settings.webSearchEmulation.subscribedAt")
                  }}</label>
                  <input
                    :value="formatSubscribedAt(provider.subscribed_at)"
                    type="date"
                    class="input text-sm"
                    @input="
                      provider.subscribed_at = parseSubscribedAt(
                        ($event.target as HTMLInputElement).value,
                      )
                    "
                  />
                  <p class="mt-0.5 text-xs text-gray-400">
                    {{
                      t(
                        "admin.settings.webSearchEmulation.subscribedAtHint",
                      )
                    }}
                  </p>
                </div>
              </div>

              <!-- Usage display -->
              <div class="flex items-center gap-2">
                <span class="text-xs text-gray-500"
                  >{{
                    t("admin.settings.webSearchEmulation.quotaUsage")
                  }}:</span
                >
                <div
                  v-if="
                    provider.quota_limit != null &&
                    provider.quota_limit > 0
                  "
                  class="flex-1 rounded-full bg-gray-200 dark:bg-dark-600"
                  style="height: 6px"
                >
                  <div
                    class="h-full rounded-full transition-all"
                    :class="
                      quotaPercentage(provider) > 90
                        ? 'bg-red-500'
                        : quotaPercentage(provider) > 70
                          ? 'bg-yellow-500'
                          : 'bg-green-500'
                    "
                    :style="{
                      width:
                        Math.min(quotaPercentage(provider), 100) + '%',
                    }"
                  />
                </div>
                <div v-else class="flex-1" />
                <span class="text-xs text-gray-500"
                  >{{ provider.quota_used ?? 0 }} /
                  {{
                    provider.quota_limit != null &&
                    provider.quota_limit > 0
                      ? provider.quota_limit
                      : "∞"
                  }}</span
                >
                <button
                  v-if="(provider.quota_used ?? 0) > 0"
                  type="button"
                  class="text-xs text-primary-600 hover:text-primary-700"
                  @click="resetWebSearchUsage(pIdx)"
                >
                  {{ t("admin.settings.webSearchEmulation.resetUsage") }}
                </button>
              </div>

              <!-- Proxy + Test on same row -->
              <div class="flex items-end gap-3">
                <div class="flex-1">
                  <label class="text-xs text-gray-500">{{
                    t("admin.settings.webSearchEmulation.proxy")
                  }}</label>
                  <ProxySelector
                    v-model="provider.proxy_id"
                    :proxies="webSearchProxies"
                  />
                </div>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm whitespace-nowrap"
                  @click="openTestDialog()"
                >
                  {{ t("admin.settings.webSearchEmulation.test") }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Web Search Test Dialog -->
    <div
      v-if="wsTestDialogOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      @click.self="wsTestDialogOpen = false"
    >
      <div
        class="mx-4 w-full max-w-lg rounded-xl bg-white p-6 shadow-xl dark:bg-dark-800"
      >
        <h3
          class="mb-4 text-lg font-semibold text-gray-900 dark:text-white"
        >
          {{ t("admin.settings.webSearchEmulation.testResultTitle") }}
        </h3>
        <div class="flex items-center gap-2">
          <input
            v-model="wsTestQuery"
            type="text"
            class="input flex-1 text-sm"
            :placeholder="
              t('admin.settings.webSearchEmulation.testDefaultQuery')
            "
            @keyup.enter="testWebSearchProvider()"
          />
          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="wsTestLoading"
            @click="testWebSearchProvider()"
          >
            {{
              wsTestLoading
                ? t("admin.settings.webSearchEmulation.testing")
                : t("admin.settings.webSearchEmulation.test")
            }}
          </button>
        </div>
        <!-- Test results -->
        <div
          v-if="wsTestResult"
          class="mt-4 max-h-80 overflow-y-auto rounded-lg bg-gray-50 p-4 dark:bg-dark-700"
        >
          <p
            class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{
              t("admin.settings.webSearchEmulation.testResultProvider")
            }}: {{ wsTestResult.provider }}
          </p>
          <div
            v-if="wsTestResult.results.length === 0"
            class="text-sm text-gray-400"
          >
            {{ t("admin.settings.webSearchEmulation.testNoResults") }}
          </div>
          <div
            v-for="(r, rIdx) in wsTestResult.results"
            :key="rIdx"
            class="mt-2 border-t border-gray-200 pt-2 first:mt-0 first:border-0 first:pt-0 dark:border-dark-600"
          >
            <a
              :href="r.url"
              target="_blank"
              class="text-sm font-medium text-blue-600 hover:underline dark:text-blue-400"
              >{{ r.title }}</a
            >
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ r.snippet }}
            </p>
          </div>
        </div>
        <div class="mt-4 flex justify-end">
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            @click="wsTestDialogOpen = false"
          >
            {{ t("common.close") }}
          </button>
        </div>
      </div>
    </div>

  <!-- Usage Records Settings -->
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.usageRecords.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.usageRecords.description') }}
      </p>
    </div>
    <div class="space-y-4 p-6">
      <!-- User usage details visibility -->
      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.user_usage_detail_view.label') }}
          </label>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.user_usage_detail_view.description') }}
          </p>
        </div>
        <label class="toggle shrink-0">
          <input
            v-model="form.allow_user_view_usage_details"
            data-testid="allow-user-view-usage-details"
            type="checkbox"
          />
          <span class="toggle-slider"></span>
        </label>
      </div>
      <!-- User error requests visibility -->
      <div class="flex items-center justify-between gap-4 border-t border-gray-100 pt-4 dark:border-dark-700">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.user_error_view.label') }}
          </label>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.user_error_view.description') }}
          </p>
        </div>
        <label class="toggle shrink-0">
          <input v-model="form.allow_user_view_error_requests" type="checkbox" />
          <span class="toggle-slider"></span>
        </label>
      </div>
    </div>
  </div>
</div>
</template>

<script setup lang="ts">
import Icon from '@/common/widgets/icons/Icon.vue'
import ProxySelector from '@/common/widgets/data/ProxySelector.vue'
import Select from '@/common/widgets/forms/Select.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'

const { addClaudeOAuthSystemPromptBlock, addWebSearchProvider, apiKeyVisible, applyClaudeOAuthSystemPromptPreset, claudeOAuthSystemPromptBlockTypeOptions, claudeOAuthSystemPromptBlocks, claudeOAuthSystemPromptCacheTTLOptions, claudeOAuthSystemPromptPresetOptions, copyApiKey, expandedProviders, form, formatSubscribedAt, getClaudeOAuthPresetLabel, markClaudeOAuthSystemPromptBlockCustom, moveClaudeOAuthSystemPromptBlock, openTestDialog, parseSubscribedAt, quotaPercentage, removeClaudeOAuthSystemPromptBlock, removeWebSearchProvider, resetClaudeOAuthSystemPromptBlocks, resetWebSearchUsage, t, testWebSearchProvider, toggleClaudeOAuthSystemPromptBlock, toggleProviderExpand, webSearchConfig, webSearchProxies, wsTestDialogOpen, wsTestLoading, wsTestQuery, wsTestResult } = useSettingsPageContext()
</script>
