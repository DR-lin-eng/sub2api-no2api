<script setup lang="ts">
import { ref, watch } from 'vue'
import { gatewayBaseUrl, PLACEHOLDER_BASE_URL, setGatewayBaseUrl } from '../gateway'

const draft = ref(gatewayBaseUrl.value)
const invalid = ref(false)

watch(gatewayBaseUrl, (value) => {
  draft.value = value
})

function save() {
  // Blurring an untouched field must not persist a value that came from a link.
  if (draft.value.trim() === gatewayBaseUrl.value) {
    invalid.value = false
    return
  }
  invalid.value = !setGatewayBaseUrl(draft.value)
}

function reset() {
  draft.value = ''
  save()
}
</script>

<template>
  <form class="gateway-setting" @submit.prevent="save">
    <label class="gateway-setting__label" for="gateway-base-url">示例中的网关地址</label>
    <div class="gateway-setting__row">
      <input
        id="gateway-base-url"
        v-model="draft"
        class="gateway-setting__input"
        :class="{ 'is-invalid': invalid }"
        type="url"
        inputmode="url"
        autocomplete="off"
        spellcheck="false"
        :placeholder="PLACEHOLDER_BASE_URL"
        @blur="save"
      />
      <button v-if="gatewayBaseUrl" class="gateway-setting__reset" type="button" title="恢复占位符" @click="reset">
        重置
      </button>
    </div>
    <p v-if="invalid" class="gateway-setting__hint is-invalid">请输入 http(s) 开头的完整地址</p>
    <p v-else class="gateway-setting__hint">填入后，全站代码示例会改用你的地址。仅保存在本浏览器。</p>
  </form>
</template>
