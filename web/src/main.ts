import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'

import App from './App.vue'
import router from './router'
import messages from './locales'
import { useAppStore } from './stores/app'

import './style.css'

const pinia = createPinia()
const app = createApp(App)

// 语言优先级：本地存储 > 浏览器语言 > 环境变量 VITE_DEFAULT_LANGUAGE > 'zh'
function detectLocale(): string {
  const stored = localStorage.getItem('locale')
  if (stored) return stored

  const browserLang = navigator.language?.split('-')[0]
  if (browserLang && ['zh', 'en'].includes(browserLang)) return browserLang

  const envDefault = import.meta.env.VITE_DEFAULT_LANGUAGE
  if (envDefault && ['zh', 'en'].includes(envDefault)) return envDefault

  return 'zh'
}

const i18n = createI18n({
  legacy: false,
  locale: detectLocale(),
  messages
})

for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(pinia)
app.use(router)
app.use(i18n)
app.use(ElementPlus)

const appStore = useAppStore()
if (appStore.isDark) {
  document.documentElement.classList.add('dark')
}

app.mount('#app')
