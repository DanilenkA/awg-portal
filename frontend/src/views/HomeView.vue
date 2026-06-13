<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { authStore } from "@/stores/auth";
import { interfaceStore } from "@/stores/interfaces";
import { peerStore } from "@/stores/peers";
import { RouterLink } from "vue-router";
import { humanFileSize } from "@/helpers/utils";
import { notify } from "@kyvg/vue3-notification";
import { settingsStore } from "@/stores/settings";
import { useI18n } from "vue-i18n";

const auth = authStore()
const interfaces = interfaceStore()
const peers = peerStore()
const settings = settingsStore()
const { locale } = useI18n()

const supportedLanguages = [
  { code: 'en', flag: 'us', label: 'English' },
  { code: 'ru', flag: 'ru', label: 'Русский' },
  { code: 'de', flag: 'de', label: 'Deutsch' },
  { code: 'fr', flag: 'fr', label: 'Français' },
  { code: 'pt', flag: 'pt', label: 'Português' },
  { code: 'uk', flag: 'ua', label: 'Українська' },
  { code: 'vi', flag: 'vn', label: 'Tiếng Việt' },
  { code: 'zh', flag: 'cn', label: '中文' },
  { code: 'es', flag: 'es', label: 'Español' },
  { code: 'ja', flag: 'jp', label: '日本語' },
  { code: 'ko', flag: 'kr', label: '한국어' },
]
const supportedCodes = supportedLanguages.map(l => l.code)
const currentLang = ref(locale.value || localStorage.getItem('wgLang') || 'en')

const switchLanguage = (lang) => {
  if (locale.value !== lang) {
    localStorage.setItem('wgLang', lang)
    locale.value = lang
    currentLang.value = lang
  }
}

// Auto-detect browser language on first visit (only if user has not chosen yet).
const detectBrowserLanguage = () => {
  if (localStorage.getItem('wgLang')) return
  const navLang = (navigator && (navigator.language || navigator.userLanguage)) || ''
  const guess = navLang.split('-')[0]
  if (guess && supportedCodes.includes(guess)) {
    switchLanguage(guess)
  }
}

const loading = ref(false)
const loginExpanded = ref(false)
const loggingIn = ref(false)
const username = ref("")
const password = ref("")

const usernameInvalid = computed(() => username.value === "")
const passwordInvalid = computed(() => password.value === "")
const disableLoginBtn = computed(() => usernameInvalid.value || passwordInvalid.value || loggingIn.value)

onMounted(async () => {
  detectBrowserLanguage()
  if (!auth.IsAuthenticated) {
    await settings.LoadSettings()
    return
  }
  loading.value = true
  try {
    await Promise.all([
      interfaces.LoadInterfaces(),
      peers.LoadPeers(undefined),
      peers.LoadStats(undefined),
    ])
  } catch (e) {
    console.warn("Dashboard data load partial:", e)
  } finally {
    loading.value = false
  }
  startStatsRefreshLoop()
})

// Periodic refresh of peer stats on the home dashboard, so the
// "Connected peers" count and traffic totals stay live. Mirrors the
// pattern used in InterfaceView.vue.
let statsRefreshInterval = null
const STATS_REFRESH_INTERVAL_MS = 30000
let statsRefreshInFlight = false

async function refreshStatsOnce() {
  if (statsRefreshInFlight) return
  if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
  statsRefreshInFlight = true
  try {
    await peers.LoadStats(undefined)
  } catch (e) {
    console.debug('periodic stats refresh failed', e)
  } finally {
    statsRefreshInFlight = false
  }
}

function startStatsRefreshLoop() {
  stopStatsRefreshLoop()
  statsRefreshInterval = setInterval(refreshStatsOnce, STATS_REFRESH_INTERVAL_MS)
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', refreshStatsOnce)
  }
}

function stopStatsRefreshLoop() {
  if (statsRefreshInterval) {
    clearInterval(statsRefreshInterval)
    statsRefreshInterval = null
  }
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', refreshStatsOnce)
  }
}

onBeforeUnmount(() => {
  stopStatsRefreshLoop()
})

const interfaceCount = computed(() => interfaces.Count || 0)
const awgCount = computed(() => interfaces.All.filter(i => i.AWGEnabled).length)
const wgCount = computed(() => interfaces.All.filter(i => !i.AWGEnabled).length)

const totalPeers = computed(() => peers.All.length)
const connectedPeers = computed(() => {
  let count = 0
  peers.All.forEach(p => {
    if (peers.Statistics(p.Identifier).IsConnected) count++
  })
  return count
})

const totalTrafficReceived = computed(() => {
  let total = 0
  for (const key in peers.stats || {}) {
    total += peers.stats[key].BytesReceived || 0
  }
  return total
})

const totalTrafficTransmitted = computed(() => {
  let total = 0
  for (const key in peers.stats || {}) {
    total += peers.stats[key].BytesTransmitted || 0
  }
  return total
})

const traffic24h = computed(() => {
  return humanFileSize(totalTrafficReceived.value + totalTrafficTransmitted.value)
})

const expiringPeers = computed(() => {
  if (!peers.All) return 0
  const now = new Date()
  return peers.All.filter(p => {
    if (!p.ExpiresAt || p.Disabled) return false
    const exp = new Date(p.ExpiresAt)
    const diffDays = (exp - now) / (1000 * 60 * 60 * 24)
    return diffDays >= 0 && diffDays < 7
  }).length
})

const statsEnabled = computed(() => peers.hasStatistics)

const userInitial = computed(() => {
  let n = 'A'
  if (auth.User.Firstname) n = auth.User.Firstname[0]
  else if (auth.User.Identifier) n = auth.User.Identifier[0]
  return (n || 'A').toUpperCase()
})

const revealLogin = () => {
  loginExpanded.value = true
}

const postLoginRoute = () => {
  const target = auth.ReturnUrl || '/'
  return !target || target === '/login' ? '/' : target
}

const login = async () => {
  if (disableLoginBtn.value) return

  const target = postLoginRoute()
  loggingIn.value = true
  try {
    await auth.Login(username.value, password.value)
    await settings.LoadSettings()
    notify({
      title: "Logged in",
      text: "Authentication succeeded!",
      type: 'success',
    })

    // Full-page reload — see LoginView for rationale (avoids auth-layout race).
    const targetHash = target.startsWith('/') ? `#${target}` : target
    const targetUrl = `${window.location.origin}${window.location.pathname}${window.location.search}${targetHash}`
    window.location.assign(targetUrl)
  } catch (e) {
    notify({
      title: "Login failed!",
      text: "Authentication failed!",
      type: 'error',
    })
    setTimeout(() => loggingIn.value = false, 1000)
    return
  }

  loggingIn.value = false
}
</script>

<template>
  <!-- Hero / Not authed -->
  <div v-if="!auth.IsAuthenticated" class="root-auth-shell">
    <div class="login-card root-auth-card" :class="{ 'is-expanded': loginExpanded }">
      <div class="login-logo">
        <img src="@/assets/wg-logo.webp" alt="AWG Portal" class="login-logo-icon" />
        <span class="login-logo-text">AWG Portal</span>
      </div>
      <h1>{{ $t('home.headline') }}</h1>
      <p class="root-auth-copy">{{ $t('home.abstract') }}</p>

      <Transition name="login-reveal" mode="out-in">
        <button
          v-if="!loginExpanded"
          key="reveal"
          class="btn btn-primary btn-lg root-auth-action"
          type="button"
          @click.prevent="revealLogin"
        >
          <i class="fa-solid fa-right-to-bracket"></i> {{ $t('menu.login') }}
        </button>

        <form v-else key="form" class="login-form root-auth-form" method="post" @submit.prevent="login">
          <div class="form-group">
            <label class="form-label" for="homeInputUsername">{{ $t('login.username.label') }}</label>
            <div class="input-group">
              <span class="input-group-text"><span class="far fa-user p-2"></span></span>
              <input
                id="homeInputUsername"
                v-model="username"
                :class="{'is-invalid': usernameInvalid, 'is-valid': !usernameInvalid}"
                :placeholder="$t('login.username.placeholder')"
                class="form-control"
                name="username"
                type="text"
                autocomplete="username"
                autofocus
              >
            </div>
          </div>
          <div class="form-group">
            <label class="form-label" for="homeInputPassword">{{ $t('login.password.label') }}</label>
            <div class="input-group">
              <span class="input-group-text"><span class="fas fa-lock p-2"></span></span>
              <input
                id="homeInputPassword"
                v-model="password"
                :class="{'is-invalid': passwordInvalid, 'is-valid': !passwordInvalid}"
                :placeholder="$t('login.password.placeholder')"
                class="form-control"
                name="password"
                type="password"
                autocomplete="current-password"
              >
            </div>
          </div>
          <button :disabled="disableLoginBtn" class="btn btn-primary btn-lg root-auth-action" type="submit">
            {{ $t('login.button') }}
            <span v-if="loggingIn" class="d-inline"><i class="ms-2 fa-solid fa-circle-notch fa-spin"></i></span>
          </button>
        </form>
      </Transition>

      <div class="login-footer lang-switcher" role="group" aria-label="Language">
        <button
          v-for="lang in supportedLanguages"
          :key="lang.code"
          type="button"
          class="lang-btn"
          :class="{ active: currentLang === lang.code }"
          :title="lang.label"
          :aria-label="lang.label"
          :aria-pressed="currentLang === lang.code"
          @click.prevent="switchLanguage(lang.code)"
        >
          <span class="fi" :class="'fi-' + lang.flag"></span>
        </button>
      </div>
    </div>
  </div>

  <template v-else>
    <!-- Page header -->
    <div class="page-header">
      <div>
        <h1>{{ $t('home.headline') }}</h1>
        <p>{{ $t('home.abstract') }}</p>
      </div>
      <div class="page-header__actions">
        <RouterLink v-if="auth.IsAdmin" :to="{ name: 'interfaces' }" class="btn btn-secondary">
          <i class="fa-solid fa-network-wired"></i> {{ $t('menu.interfaces') }}
        </RouterLink>
        <RouterLink v-if="auth.IsAdmin" :to="{ name: 'interfaces' }" class="btn btn-primary">
          <i class="fa-solid fa-plus"></i> {{ $t('home.admin.button-admin') }}
        </RouterLink>
      </div>
    </div>

    <!-- Stats grid -->
    <div class="stats-grid">
      <RouterLink :to="{ name: 'interfaces' }" class="stat-card stat-card-link" v-if="auth.IsAdmin">
        <div class="stat-card-icon accent">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <rect x="2" y="2" width="20" height="8" rx="2" ry="2"/>
            <rect x="2" y="14" width="20" height="8" rx="2" ry="2"/>
            <line x1="6" y1="6" x2="6.01" y2="6"/>
            <line x1="6" y1="18" x2="6.01" y2="18"/>
          </svg>
        </div>
        <div class="stat-card-label">{{ $t('menu.interfaces') }}</div>
        <div class="stat-card-value">{{ interfaceCount }}</div>
        <div class="stat-card-change">{{ awgCount }} AmneziaWG · {{ wgCount }} WireGuard</div>
      </RouterLink>

      <RouterLink :to="{ name: 'interfaces' }" class="stat-card stat-card-link" v-if="statsEnabled">
        <div class="stat-card-icon success">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/>
            <circle cx="9" cy="7" r="4"/>
          </svg>
        </div>
        <div class="stat-card-label">{{ $t('interfaces.headline-peers') }}</div>
        <div class="stat-card-value">{{ connectedPeers }} / {{ totalPeers }}</div>
        <div class="stat-card-change">{{ $t('interfaces.peer-connected') }}</div>
      </RouterLink>

      <RouterLink :to="{ name: 'traffic' }" class="stat-card stat-card-link" v-if="statsEnabled">
        <div class="stat-card-icon warning">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>
          </svg>
        </div>
        <div class="stat-card-label">{{ $t('modals.peer-view.traffic') }}</div>
        <div class="stat-card-value">{{ traffic24h }}</div>
        <div class="stat-card-change">
          <i class="fa-solid fa-arrow-down"></i> {{ humanFileSize(totalTrafficReceived) }}
          · <i class="fa-solid fa-arrow-up"></i> {{ humanFileSize(totalTrafficTransmitted) }}
        </div>
      </RouterLink>

      <div class="stat-card" v-if="expiringPeers > 0">
        <div class="stat-card-icon danger">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
            <line x1="12" y1="9" x2="12" y2="13"/>
            <line x1="12" y1="17" x2="12.01" y2="17"/>
          </svg>
        </div>
        <div class="stat-card-label">{{ $t('interfaces.peer-expiring') }}</div>
        <div class="stat-card-value">{{ expiringPeers }}</div>
        <div class="stat-card-change">{{ $t('home.dashboard.alerts_subtitle') }}</div>
      </div>
    </div>

    <!-- Quick actions: cards -->
    <div class="section-header">
      <h2>{{ $t('home.info-headline') }}</h2>
    </div>

    <div class="stats-grid" style="grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));">
      <RouterLink :to="{ name: 'interfaces' }" class="stat-card stat-card-link" v-if="auth.IsAdmin">
        <div class="stat-card-icon accent" style="margin-bottom: 0.75rem;">
          <i class="fa-solid fa-network-wired"></i>
        </div>
        <div style="font-weight:600;font-size:0.9375rem;color:var(--text-primary);">{{ $t('home.admin.button-admin') }}</div>
        <div class="stat-card-change">{{ interfaceCount }} {{ $t('menu.interfaces') }}</div>
      </RouterLink>

      <RouterLink :to="{ name: 'users' }" class="stat-card stat-card-link" v-if="auth.IsAdmin">
        <div class="stat-card-icon purple" style="margin-bottom: 0.75rem;">
          <i class="fa-solid fa-users"></i>
        </div>
        <div style="font-weight:600;font-size:0.9375rem;color:var(--text-primary);">{{ $t('home.admin.button-user') }}</div>
        <div class="stat-card-change">{{ $t('menu.users') }}</div>
      </RouterLink>

      <RouterLink :to="{ name: 'key-generator' }" class="stat-card stat-card-link">
        <div class="stat-card-icon success" style="margin-bottom: 0.75rem;">
          <i class="fa-solid fa-key"></i>
        </div>
        <div style="font-weight:600;font-size:0.9375rem;color:var(--text-primary);">{{ $t('menu.keygen') }}</div>
        <div class="stat-card-change">{{ $t('keygen.abstract') }}</div>
      </RouterLink>

      <RouterLink :to="{ name: 'ip-calculator' }" class="stat-card stat-card-link">
        <div class="stat-card-icon warning" style="margin-bottom: 0.75rem;">
          <i class="fa-solid fa-calculator"></i>
        </div>
        <div style="font-weight:600;font-size:0.9375rem;color:var(--text-primary);">{{ $t('menu.calculator') }}</div>
        <div class="stat-card-change">{{ $t('calculator.abstract') }}</div>
      </RouterLink>

      <RouterLink :to="{ name: 'profile' }" class="stat-card stat-card-link">
        <div class="stat-card-icon accent" style="margin-bottom: 0.75rem;">
          <i class="fa-solid fa-user"></i>
        </div>
        <div style="font-weight:600;font-size:0.9375rem;color:var(--text-primary);">{{ $t('home.profiles.button') }}</div>
        <div class="stat-card-change">{{ $t('profile.headline') }}</div>
      </RouterLink>
    </div>

    <!-- Profile shortcut (admins) -->
    <div class="card mt-6" v-if="auth.IsAuthenticated && auth.User.Identifier">
      <div class="card-header">
        <h3><i class="fa-solid fa-user me-2"></i>{{ auth.User.Identifier }}</h3>
        <RouterLink :to="{ name: 'profile' }" class="btn btn-ghost btn-sm">
          {{ $t('menu.profile') }} <i class="fa-solid fa-arrow-right"></i>
        </RouterLink>
      </div>
      <div class="card-body" style="display:flex;align-items:center;gap:1rem;">
        <div style="width:48px;height:48px;border-radius:8px;background:linear-gradient(135deg, var(--accent), #7c4dff);display:flex;align-items:center;justify-content:center;color:white;font-weight:700;font-size:1.125rem;flex-shrink:0;">
          {{ userInitial }}
        </div>
        <div>
          <div style="font-weight:600;color:var(--text-primary);">{{ auth.User.Email || auth.User.Identifier }}</div>
          <div class="text-muted text-muted-sm">{{ auth.IsAdmin ? $t('users.admin') : $t('users.no-admin') }}</div>
        </div>
      </div>
    </div>
  </template>
</template>
