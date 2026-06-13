<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue"
import { peerStore } from "@/stores/peers"
import { interfaceStore } from "@/stores/interfaces"
import { humanFileSize } from "@/helpers/utils"
import { useI18n } from "vue-i18n"

const peers = peerStore()
const interfaces = interfaceStore()
const { t } = useI18n()

// Periodic stats refresh — same pattern as the InterfaceView / HomeView.
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

onMounted(async () => {
  await interfaces.LoadInterfaces()
  await peers.LoadPeers(undefined)
  await peers.LoadStats(undefined)
  startStatsRefreshLoop()
})

onBeforeUnmount(() => {
  stopStatsRefreshLoop()
})

// Per-peer rows: name, interface name, uploaded bytes, downloaded bytes,
// connection status, last handshake timestamp.
//
// Only peers that have observed traffic in this session are shown —
// the user requested session totals only, not historical data.
// Sorted by total bytes (rx + tx) descending so the busiest peers are
// at the top.
const peerRows = computed(() => {
  if (!peers.All) return []
  const out = []
  for (const p of peers.All) {
    if (!p.InterfaceIdentifier) continue
    const s = peers.Statistics(p.Identifier)
    const tx = s?.BytesTransmitted || 0
    const rx = s?.BytesReceived || 0
    if (tx + rx === 0) continue
    const iface = interfaces.Find(p.InterfaceIdentifier)
    out.push({
      id: p.Identifier,
      name: p.DisplayName || (p.Identifier ? p.Identifier.slice(0, 12) + '…' : '—'),
      ifaceName: (iface && (iface.DisplayName || iface.Identifier)) || p.InterfaceIdentifier,
      isAWG: !!(iface && iface.AWGEnabled),
      uploaded: tx,
      downloaded: rx,
      isConnected: !!s?.IsConnected,
      lastHandshake: s?.LastHandshake || null,
    })
  }
  return out.sort((a, b) => (b.uploaded + b.downloaded) - (a.uploaded + a.downloaded))
})

const totalUp = computed(() => peerRows.value.reduce((acc, r) => acc + r.uploaded, 0))
const totalDown = computed(() => peerRows.value.reduce((acc, r) => acc + r.downloaded, 0))
const activePeerCount = computed(() => peerRows.value.length)
const connectedCount = computed(() => peerRows.value.filter(r => r.isConnected).length)

function friendlyLastHandshake(ts) {
  if (!ts) return '—'
  try {
    const d = new Date(ts)
    if (isNaN(d.getTime())) return ts
    return d.toLocaleString()
  } catch {
    return ts
  }
}
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('home.traffic.headline', 'Traffic') }}</h1>
      <p>{{ t('home.traffic.abstract', 'Per-peer session traffic — totals since this portal was started.') }}</p>
    </div>
  </div>

  <div v-if="!peers.hasStatistics" class="empty-state card">
    <div class="empty-state-icon"><i class="fa-solid fa-chart-line"></i></div>
    <h3>{{ t('home.traffic.no_stats', 'Statistics are disabled on the server.') }}</h3>
    <p>{{ t('home.traffic.no_stats_help', 'Enable statistics collection to see per-peer traffic.') }}</p>
  </div>

  <template v-else>
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-card-icon accent">
          <i class="fa-solid fa-arrow-down"></i>
        </div>
        <div class="stat-card-label">{{ t('home.traffic.total_downloaded', 'Downloaded (session)') }}</div>
        <div class="stat-card-value">{{ humanFileSize(totalDown) }}</div>
        <div class="stat-card-change">{{ activePeerCount }} {{ t('home.traffic.peers', 'peers') }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-card-icon warning">
          <i class="fa-solid fa-arrow-up"></i>
        </div>
        <div class="stat-card-label">{{ t('home.traffic.total_uploaded', 'Uploaded (session)') }}</div>
        <div class="stat-card-value">{{ humanFileSize(totalUp) }}</div>
        <div class="stat-card-change">{{ activePeerCount }} {{ t('home.traffic.peers', 'peers') }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-card-icon success">
          <i class="fa-solid fa-circle-check"></i>
        </div>
        <div class="stat-card-label">{{ t('interfaces.peer-connected', 'Connected') }}</div>
        <div class="stat-card-value">{{ connectedCount }} / {{ activePeerCount }}</div>
        <div class="stat-card-change">{{ t('interfaces.headline-peers', 'Current VPN Peers') }}</div>
      </div>
    </div>

    <div v-if="peerRows.length === 0" class="empty-state card">
      <div class="empty-state-icon"><i class="fa-solid fa-user-slash"></i></div>
      <h3>{{ t('home.traffic.no_traffic', 'No traffic observed this session.') }}</h3>
      <p>{{ t('home.traffic.no_traffic_help', 'Once a peer sends or receives data, totals will appear here.') }}</p>
    </div>

    <div v-else class="card mt-4">
      <table class="data-table">
        <thead>
          <tr>
            <th>{{ t('interfaces.table-heading.name', 'Name') }}</th>
            <th>{{ t('interfaces.interface.headline', 'Interface') }}</th>
            <th class="text-end"><i class="fa-solid fa-arrow-down me-1"></i>{{ t('home.traffic.downloaded', 'Downloaded') }}</th>
            <th class="text-end"><i class="fa-solid fa-arrow-up me-1"></i>{{ t('home.traffic.uploaded', 'Uploaded') }}</th>
            <th>{{ t('interfaces.table-heading.status', 'Status') }}</th>
            <th>{{ t('home.traffic.last_handshake', 'Last handshake') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in peerRows" :key="row.id">
            <td class="peer-name">{{ row.name }}</td>
            <td>
              <span class="text-mono-sm">{{ row.ifaceName }}</span>
              <span v-if="row.isAWG" class="tag tag-awg ms-2" style="margin-left:0.5rem;">AmneziaWG</span>
              <span v-else class="tag tag-wg ms-2" style="margin-left:0.5rem;">WireGuard</span>
            </td>
            <td class="text-end text-mono-sm">{{ humanFileSize(row.downloaded) }}</td>
            <td class="text-end text-mono-sm">{{ humanFileSize(row.uploaded) }}</td>
            <td>
              <span v-if="row.isConnected" class="d-inline-flex align-items-center gap-2">
                <span class="status-dot active"></span>
                <span class="text-success fw-500">{{ t('interfaces.peer-connected', 'Connected') }}</span>
              </span>
              <span v-else class="d-inline-flex align-items-center gap-2">
                <span class="status-dot inactive"></span>
                <span class="text-muted">{{ t('interfaces.peer-not-connected', 'Not connected') }}</span>
              </span>
            </td>
            <td class="text-mono-sm">{{ friendlyLastHandshake(row.lastHandshake) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </template>
</template>
