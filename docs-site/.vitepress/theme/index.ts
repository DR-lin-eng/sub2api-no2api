import type { Theme } from 'vitepress'
import { onContentUpdated } from 'vitepress'
import DefaultTheme from 'vitepress/theme'
import { h, onMounted } from 'vue'
import Endpoint from './components/Endpoint.vue'
import GatewaySetting from './components/GatewaySetting.vue'
import { applyGatewayBaseUrl, loadGatewayBaseUrl } from './gateway'
import './custom.css'

export default {
  extends: DefaultTheme,
  Layout: () => h(DefaultTheme.Layout, null, { 'sidebar-nav-before': () => h(GatewaySetting) }),
  enhanceApp({ app }) {
    app.component('Endpoint', Endpoint)
  },
  setup() {
    onMounted(loadGatewayBaseUrl)
    onContentUpdated(applyGatewayBaseUrl)
  }
} satisfies Theme
