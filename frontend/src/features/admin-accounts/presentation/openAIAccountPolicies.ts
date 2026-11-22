import type { Ref } from 'vue'
import type { CodexImageToolMode } from './accountEditorContext'

export interface OpenAIQuotaPauseState {
  autoPause5hDisabled: Ref<boolean>
  autoPause7dDisabled: Ref<boolean>
  autoPause5hThreshold: Ref<number | null>
  autoPause7dThreshold: Ref<number | null>
}

export function applyOpenAIAutoPauseExtra(
  extra: Record<string, unknown>,
  context: OpenAIQuotaPauseState,
): void {
  if (
    context.autoPause5hThreshold.value != null &&
    context.autoPause5hThreshold.value > 0
  ) {
    extra.auto_pause_5h_threshold = context.autoPause5hThreshold.value / 100
  } else {
    delete extra.auto_pause_5h_threshold
  }
  if (
    context.autoPause7dThreshold.value != null &&
    context.autoPause7dThreshold.value > 0
  ) {
    extra.auto_pause_7d_threshold = context.autoPause7dThreshold.value / 100
  } else {
    delete extra.auto_pause_7d_threshold
  }
  if (context.autoPause5hDisabled.value) extra.auto_pause_5h_disabled = true
  else delete extra.auto_pause_5h_disabled
  if (context.autoPause7dDisabled.value) extra.auto_pause_7d_disabled = true
  else delete extra.auto_pause_7d_disabled
}

export function applyCodexImageToolExtra(
  extra: Record<string, unknown>,
  context: { codexImageToolMode: Ref<CodexImageToolMode> },
): void {
  delete extra.codex_image_generation_bridge_enabled
  switch (context.codexImageToolMode.value) {
    case 'enabled':
    case 'disabled':
      extra.codex_image_generation_bridge =
        context.codexImageToolMode.value === 'enabled'
      delete extra.codex_image_generation_explicit_tool_policy
      break
    case 'block':
      extra.codex_image_generation_explicit_tool_policy = 'strip'
      delete extra.codex_image_generation_bridge
      break
    default:
      delete extra.codex_image_generation_bridge
      delete extra.codex_image_generation_explicit_tool_policy
  }
}
