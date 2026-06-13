<script setup>
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router';
import { computed, onMounted, onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from 'vue-i18n';
import { authStore } from "./stores/auth";
import { securityStore } from "./stores/security";
import { settingsStore } from "@/stores/settings";

const { t, locale } = useI18n()
const auth = authStore()
const sec = securityStore()
const settings = settingsStore()
const route = useRoute()
const router = useRouter()

const userMenuOpen = ref(false)
const currentLang = ref(localStorage.getItem('wgLang') || 'ru')

// Responsive sidebar state — single unified adaptive layout, no separate mobile branch.
// Breakpoints (in CSS px): >1200 = full sidebar, 768-1200 = compact icons, <768 = hidden behind burger
const BREAKPOINTS = { compact: 1200, mobile: 768 }
const viewportWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1440)
const mobileSidebarOpen = ref(false)
const updateViewport = () => {
  viewportWidth.value = window.innerWidth
  if (window.innerWidth >= BREAKPOINTS.mobile) {
    mobileSidebarOpen.value = false
  }
}
onMounted(() => {
  window.addEventListener('resize', updateViewport)
  updateViewport()
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', updateViewport)
})
const isCompact = computed(() => viewportWidth.value < BREAKPOINTS.compact && viewportWidth.value >= BREAKPOINTS.mobile)
const isMobile = computed(() => viewportWidth.value < BREAKPOINTS.mobile)
const showSidebar = computed(() => !isMobile.value || mobileSidebarOpen.value)
const sidebarIsCompact = computed(() => isCompact.value)
const toggleMobileSidebar = () => { mobileSidebarOpen.value = !mobileSidebarOpen.value }
const closeMobileSidebar = () => { mobileSidebarOpen.value = false }
// Close drawer on route change (mobile)
watch(() => route.fullPath, () => {
  if (isMobile.value) mobileSidebarOpen.value = false
})

const routerViewKey = computed(() => `${route.fullPath}:${auth.IsAuthenticated ? 'auth' : 'guest'}`)

onMounted(async () => {
  console.log("Starting AWG-PORTAL frontend...");

  document.documentElement.setAttribute('data-bs-theme', 'dark');

  // sync language
  currentLang.value = locale.value

  await sec.LoadSecurityProperties();
  await auth.LoadProviders();

  let wasLoggedIn = auth.IsAuthenticated;
  try {
    await auth.LoadSession();
    await settings.LoadSettings(); // only logs errors, does not throw

    console.log("AWG-PORTAL session is valid");
  } catch (e) {
    if (wasLoggedIn) {
      console.log("AWG-PORTAL invalid - logging out");
      await auth.Logout();
    }
  }

  console.log("AWG-PORTAL ready!");
})

const switchLanguage = function (lang) {
  if (locale.value !== lang) {
    localStorage.setItem('wgLang', lang);
    locale.value = lang;
    currentLang.value = lang;
  }
}

const toggleUserMenu = () => {
  userMenuOpen.value = !userMenuOpen.value
}

const closeUserMenu = () => {
  userMenuOpen.value = false
}

// Close user menu on outside click
const handleDocumentClick = (e) => {
  const menu = document.getElementById('userMenu')
  if (menu && !menu.contains(e.target) && userMenuOpen.value) {
    userMenuOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleDocumentClick)
})

const companyName = ref(WGPORTAL_SITE_COMPANY_NAME);
const wgVersion = ref(WGPORTAL_VERSION);
const webBasePath = ref(WGPORTAL_BASE_PATH);

const userDisplayName = computed(() => {
  let displayName = "Unknown";
  if (auth.IsAuthenticated) {
    if (auth.User.Firstname === "" && auth.User.Lastname === "") {
      displayName = auth.User.Identifier;
    } else if (auth.User.Firstname === "" && auth.User.Lastname !== "") {
      displayName = auth.User.Lastname;
    } else if (auth.User.Firstname !== "" && auth.User.Lastname === "") {
      displayName = auth.User.Firstname;
    } else if (auth.User.Firstname !== "" && auth.User.Lastname !== "") {
      displayName = auth.User.Firstname + " " + auth.User.Lastname;
    }
  }
  return displayName || 'admin'
})

const userInitial = computed(() => {
  const name = userDisplayName.value
  return (name && name[0]) ? name[0].toUpperCase() : 'A'
})

// Sidebar compact state is driven by viewport (compact width breakpoint)
const isCollapsed = computed(() => sidebarIsCompact.value)
const isLoginRoute = computed(() => route.name === 'login')
const isGuestHomeRoute = computed(() => route.name === 'home' && !auth.IsAuthenticated)
const isAuthLayout = computed(() => isLoginRoute.value || isGuestHomeRoute.value)

const availableLanguages = [
  { code: 'en', flag: 'us', label: 'English' },
  { code: 'ru', flag: 'ru', label: 'Русский' },
  { code: 'de', flag: 'de', label: 'Deutsch' },
  { code: 'fr', flag: 'fr', label: 'Français' },
  { code: 'pt', flag: 'pt', label: 'Português' },
  { code: 'uk', flag: 'ua', label: 'Українська' },
  { code: 'vi', flag: 'vi', label: 'Tiếng Việt' },
  { code: 'zh', flag: 'cn', label: '中文' },
  { code: 'es', flag: 'es', label: 'Español' },
  { code: 'ja', flag: 'jp', label: '日本語' },
  { code: 'ko', flag: 'kr', label: '한국어' },
]

// Page title in topbar
const topbarTitle = computed(() => {
  const titles = {
    home: t('menu.home'),
    interfaces: t('menu.interfaces'),
    users: t('menu.users'),
    profile: t('menu.profile'),
    settings: t('menu.settings'),
    audit: t('menu.audit'),
    'key-generator': t('menu.keygen'),
    'ip-calculator': t('menu.calculator'),
  }
  return titles[route.name] || ''
})
</script>

<template>
  <div class="app-shell" :class="{ 'sidebar-collapsed': isCollapsed, 'login-layout': isAuthLayout, 'sidebar-mobile-open': isMobile && mobileSidebarOpen }">
    <!-- Mobile drawer backdrop — only when the mobile drawer is actually open -->
    <div v-if="!isAuthLayout && isMobile && mobileSidebarOpen" class="sidebar-backdrop" @click="closeMobileSidebar"></div>

    <!-- === SIDEBAR === -->
    <aside v-if="!isAuthLayout" class="sidebar" v-show="showSidebar">
      <RouterLink :to="{ name: 'home' }" class="sidebar-logo" :title="companyName">
        <img v-if="auth.IsAuthenticated" src="@/assets/wg-logo.webp" alt="AWG Portal" class="sidebar-logo-icon" />
        <span class="sidebar-logo-text">AWG Portal</span>
        <span class="sidebar-logo-badge">v2</span>
      </RouterLink>

      <div class="nav-section">
        <div class="nav-section-label">{{ $t('menu.nav_main') }}</div>

        <RouterLink :to="{ name: 'home' }" class="nav-item" :class="{ active: route.name === 'home' }">
          <svg class="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>
            <polyline points="9 22 9 12 15 12 15 22"/>
          </svg>
          <span class="nav-item-label">{{ $t('menu.home') }}</span>
        </RouterLink>

        <RouterLink
          v-if="auth.IsAuthenticated && auth.IsAdmin"
          :to="{ name: 'interfaces' }"
          class="nav-item"
          :class="{ active: route.name === 'interfaces' }"
        >
          <svg class="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <rect x="2" y="2" width="20" height="8" rx="2" ry="2"/>
            <rect x="2" y="14" width="20" height="8" rx="2" ry="2"/>
            <line x1="6" y1="6" x2="6.01" y2="6"/>
            <line x1="6" y1="18" x2="6.01" y2="18"/>
          </svg>
          <span class="nav-item-label">{{ $t('menu.interfaces') }}</span>
        </RouterLink>

        <RouterLink
          v-if="auth.IsAuthenticated && auth.IsAdmin"
          :to="{ name: 'users' }"
          class="nav-item"
          :class="{ active: route.name === 'users' }"
        >
          <svg class="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/>
            <circle cx="9" cy="7" r="4"/>
            <path d="M23 21v-2a4 4 0 0 0-3-3.87"/>
            <path d="M16 3.13a4 4 0 0 1 0 7.75"/>
          </svg>
          <span class="nav-item-label">{{ $t('menu.users') }}</span>
        </RouterLink>

        <RouterLink :to="{ name: 'key-generator' }" class="nav-item" :class="{ active: route.name === 'key-generator' }">
          <svg class="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 20h9"/>
            <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/>
          </svg>
          <span class="nav-item-label">{{ $t('menu.keygen') }}</span>
        </RouterLink>

        <RouterLink :to="{ name: 'ip-calculator' }" class="nav-item" :class="{ active: route.name === 'ip-calculator' }">
          <svg class="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10"/>
            <polyline points="12 6 12 12 16 14"/>
          </svg>
          <span class="nav-item-label">{{ $t('menu.calculator') }}</span>
        </RouterLink>
      </div>

      <div v-if="auth.IsAuthenticated" class="nav-section">
        <div class="nav-section-label">{{ $t('menu.nav_account') }}</div>
        <RouterLink :to="{ name: 'profile' }" class="nav-item" :class="{ active: route.name === 'profile' }">
          <svg class="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/>
            <circle cx="12" cy="7" r="4"/>
          </svg>
          <span class="nav-item-label">{{ $t('menu.profile') }}</span>
        </RouterLink>

        <RouterLink
          v-if="auth.IsAdmin || !settings.Setting('ApiAdminOnly') || settings.Setting('WebAuthnEnabled')"
          :to="{ name: 'settings' }"
          class="nav-item"
          :class="{ active: route.name === 'settings' }"
        >
          <svg class="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="3"/>
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
          </svg>
          <span class="nav-item-label">{{ $t('menu.settings') }}</span>
        </RouterLink>

        <RouterLink
          v-if="auth.IsAdmin"
          :to="{ name: 'audit' }"
          class="nav-item"
          :class="{ active: route.name === 'audit' }"
        >
          <svg class="nav-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
            <polyline points="14 2 14 8 20 8"/>
            <line x1="16" y1="13" x2="8" y2="13"/>
            <line x1="16" y1="17" x2="8" y2="17"/>
          </svg>
          <span class="nav-item-label">{{ $t('menu.audit') }}</span>
        </RouterLink>
      </div>

      <div class="sidebar-footer">
        <div class="sidebar-version">v{{ wgVersion }} · AWG Portal</div>
      </div>
    </aside>

    <!-- === TOPBAR === -->
    <header v-if="!isAuthLayout" class="topbar">
      <button v-if="isMobile" class="topbar-burger" @click="toggleMobileSidebar" aria-label="Toggle sidebar" :aria-expanded="mobileSidebarOpen">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="3" y1="6" x2="21" y2="6"/>
          <line x1="3" y1="12" x2="21" y2="12"/>
          <line x1="3" y1="18" x2="21" y2="18"/>
        </svg>
      </button>

      <div class="topbar-search">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="11" cy="11" r="8"/>
          <line x1="21" y1="21" x2="16.65" y2="16.65"/>
        </svg>
        <input type="text" :placeholder="$t('menu.search_placeholder')" />
      </div>

      <div class="topbar-right">
        <span class="topbar-page-title" v-if="topbarTitle">{{ topbarTitle }}</span>

        <!-- User menu -->
        <div v-if="auth.IsAuthenticated" class="user-menu" :class="{ open: userMenuOpen }" id="userMenu">
          <button class="user-btn" @click.prevent="toggleUserMenu" :aria-expanded="userMenuOpen">
            <div class="user-avatar">{{ userInitial }}</div>
            <span class="user-name">{{ userDisplayName }}</span>
            <svg class="user-caret" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="6 9 12 15 18 9"/>
            </svg>
          </button>
          <div class="user-dropdown" @click="closeUserMenu">
            <RouterLink :to="{ name: 'profile' }" class="dropdown-item">
              <i class="fa-solid fa-user"></i> {{ $t('menu.profile') }}
            </RouterLink>
            <RouterLink
              v-if="auth.IsAdmin || !settings.Setting('ApiAdminOnly') || settings.Setting('WebAuthnEnabled')"
              :to="{ name: 'settings' }"
              class="dropdown-item"
            >
              <i class="fa-solid fa-gear"></i> {{ $t('menu.settings') }}
            </RouterLink>
            <RouterLink
              v-if="auth.IsAdmin"
              :to="{ name: 'audit' }"
              class="dropdown-item"
            >
              <i class="fa-solid fa-file-shield"></i> {{ $t('menu.audit') }}
            </RouterLink>
            <div class="dropdown-divider"></div>
            <!-- Language submenu -->
            <div class="dropdown-item" style="position:relative;cursor:default;">
              <i class="fa-solid fa-globe"></i>
              <span style="flex:1">{{ $t('menu.lang') }}</span>
              <span class="nav-item-badge">{{ currentLang.toUpperCase() }}</span>
            </div>
            <div style="display:flex;flex-wrap:wrap;gap:4px;padding:0 var(--space-2) var(--space-2);">
              <button
                v-for="lang in availableLanguages"
                :key="lang.code"
                class="btn btn-sm"
                :class="currentLang === lang.code ? 'btn-primary' : 'btn-ghost'"
                style="padding:2px 8px;font-size:0.7rem;"
                @click.prevent="switchLanguage(lang.code)"
              >{{ lang.label }}</button>
            </div>
            <div class="dropdown-divider"></div>
            <a class="dropdown-item dropdown-item--danger" href="#" @click.prevent="auth.Logout">
              <i class="fa-solid fa-right-from-bracket"></i> {{ $t('menu.logout') }}
            </a>
          </div>
        </div>

        <RouterLink v-else :to="{ name: 'login' }" class="btn btn-primary btn-sm">
          <i class="fa-solid fa-right-to-bracket"></i> {{ $t('menu.login') }}
        </RouterLink>
      </div>
    </header>

    <!-- === MAIN === -->
    <main class="main">
      <RouterView :key="routerViewKey" />
    </main>
  </div>
</template>
