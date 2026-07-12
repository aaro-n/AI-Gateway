<template>
  <div class="main-layout">
    <el-container>
      <el-aside :width="sidebarCollapsed ? '64px' : '220px'" class="sidebar">
        <div class="logo">
          <span v-if="!sidebarCollapsed">{{ t('app.title') }}</span>
          <span v-else>{{ t('app.shortTitle') }}</span>
        </div>
        <el-menu
          :default-active="$route.path"
          :collapse="sidebarCollapsed"
          router
          class="sidebar-menu"
        >
          <el-menu-item index="/">
            <el-icon><Monitor /></el-icon>
            <template #title>{{ t('menu.dashboard') }}</template>
          </el-menu-item>
          <el-menu-item index="/providers" v-if="isAdmin">
            <el-icon><Connection /></el-icon>
            <template #title>{{ t('menu.providers') }}</template>
          </el-menu-item>
          <el-menu-item index="/models" v-if="isAdmin">
            <el-icon><Grid /></el-icon>
            <template #title>{{ t('menu.models') }}</template>
          </el-menu-item>
          <el-menu-item index="/mcps" v-if="isAdmin">
            <el-icon><Platform /></el-icon>
            <template #title>{{ t('menu.mcps') }}</template>
          </el-menu-item>
          <el-menu-item index="/keys">
            <el-icon><Key /></el-icon>
            <template #title>{{ t('menu.keys') }}</template>
          </el-menu-item>
          <el-menu-item index="/model_usage">
            <el-icon><TrendCharts /></el-icon>
            <template #title>{{ t('menu.model_usage') }}</template>
          </el-menu-item>
          <el-menu-item index="/mcp_usage">
            <el-icon><DataAnalysis /></el-icon>
            <template #title>{{ t('menu.mcp_usage') }}</template>
          </el-menu-item>
          <el-menu-item index="/protocol_compare">
            <el-icon><Sort /></el-icon>
            <template #title>{{ t('menu.protocol_compare') }}</template>
          </el-menu-item>
          <el-menu-item index="/debug">
            <el-icon><Cpu /></el-icon>
            <template #title>{{ t('menu.debug') }}</template>
          </el-menu-item>
          <el-menu-item index="/settings" v-if="isAdmin">
            <el-icon><Tools /></el-icon>
            <template #title>{{ t('menu.settings') }}</template>
          </el-menu-item>
        </el-menu>
        <div class="sidebar-footer" v-if="!sidebarCollapsed">
          <span class="version">{{ version }}</span>
        </div>
      </el-aside>
      <el-container>
        <el-header class="header">
          <div class="header-left">
            <el-icon class="collapse-btn" @click="toggleSidebar">
              <Fold v-if="!sidebarCollapsed" />
              <Expand v-else />
            </el-icon>
          </div>
          <div class="header-right">
            <el-icon class="theme-btn" @click="toggleTheme">
              <Moon v-if="!isDark" />
              <Sunny v-else />
            </el-icon>
            <el-dropdown @command="handleUserCommand">
              <span class="user-btn">
                <el-icon><User /></el-icon>
                {{ username }}
              </span>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item @click="router.push('/profile')">
                    <el-icon><Setting /></el-icon>
                    {{ t('settings.title') }}
                  </el-dropdown-item>
                  <el-dropdown-item command="logout" divided>{{ t('login.logout') }}</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </el-header>
        <el-main class="main">
          <router-view />
        </el-main>
      </el-container>
    </el-container>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { useUserStore } from '@/stores/user'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const userStore = useUserStore()

const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isDark = computed(() => appStore.isDark)
const username = computed(() => userStore.username)
const isAdmin = computed(() => userStore.isAdmin)
const version = import.meta.env.VITE_APP_VERSION || 'dev'

function toggleSidebar() {
  appStore.toggleSidebar()
}

function toggleTheme() {
  appStore.toggleTheme()
}

async function handleUserCommand(command: string) {
  if (command === 'logout') {
    await userStore.logout()
    router.push('/login')
  }
}
</script>

<style scoped>
.main-layout {
  height: 100vh;
  display: flex;
  flex-direction: column;
}

.sidebar {
  background-color: var(--el-bg-color);
  border-right: 1px solid var(--el-border-color);
  transition: width 0.3s;
  height: 100vh;
  overflow: hidden;
  position: relative;
}

.logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  font-weight: bold;
  border-bottom: 1px solid var(--el-border-color);
}

.sidebar-menu {
  border-right: none;
  height: calc(100vh - 60px - 45px);
  overflow-y: auto;
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background-color: var(--el-bg-color);
  border-bottom: 1px solid var(--el-border-color);
  padding: 0 20px;
}

.header-left {
  display: flex;
  align-items: center;
}

.collapse-btn {
  font-size: 20px;
  cursor: pointer;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 20px;
}

.theme-btn, .user-btn {
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 14px;
}

.theme-btn:hover, .user-btn:hover {
  color: var(--el-color-primary);
}

.main {
  background-color: var(--el-bg-color-page);
  padding: 0;
  height: calc(100vh - 60px);
  overflow-y: auto;
}

.sidebar-footer {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  padding: 12px;
  text-align: center;
  border-top: 1px solid var(--el-border-color);
}

.version {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
