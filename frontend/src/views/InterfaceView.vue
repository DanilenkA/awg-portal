<script setup>
import PeerViewModal from "@/components/PeerViewModal.vue";
import PeerEditModal from "@/components/PeerEditModal.vue";
import PeerMultiCreateModal from "@/components/PeerMultiCreateModal.vue";
import InterfaceEditModal from "@/components/InterfaceEditModal.vue";
import InterfaceViewModal from "@/components/InterfaceViewModal.vue";
import Pagination from "@/components/Pagination.vue";

import {computed, onBeforeUnmount, onMounted, ref} from "vue";
import {peerStore} from "@/stores/peers";
import {interfaceStore} from "@/stores/interfaces";
import {notify} from "@kyvg/vue3-notification";
import {settingsStore} from "@/stores/settings";
import {humanFileSize} from '@/helpers/utils';
import {useI18n} from "vue-i18n";

const settings = settingsStore()
const interfaces = interfaceStore()
const peers = peerStore()

const { t } = useI18n()

const viewedPeerId = ref("")
const editPeerId = ref("")
const multiCreatePeerId = ref("")
const editInterfaceId = ref("")
const viewedInterfaceId = ref("")

const sortKey = ref("")
const sortOrder = ref(1)
const selectAll = ref(false)

const selectedPeers = computed(() => {
  return peers.All.filter(peer => peer.IsSelected).map(peer => peer.Identifier);
})

function sortBy(key) {
  if (sortKey.value === key) {
    sortOrder.value = sortOrder.value * -1; // Toggle sort order
  } else {
    sortKey.value = key;
    sortOrder.value = 1; // Default to ascending
  }
  peers.sortKey = sortKey.value;
  peers.sortOrder = sortOrder.value;
}

function calculateInterfaceName(id, name) {
  let result = id
  if (name) {
    result += ' (' + name + ')'
  }
  return result
}

const calculateBackendName = computed(() => {
  let backendId = interfaces.GetSelected.Backend

  let backendName = t('interfaces.interface.unknown-backend')
  let availableBackends = settings.Setting('AvailableBackends') || []
  availableBackends.forEach(backend => {
    if (backend.Id === backendId) {
      backendName = backend.Id === 'local' ? t(backend.Name) : backend.Name
    }
  })
  return backendName
})

const isBackendValid = computed(() => {
  let backendId = interfaces.GetSelected.Backend

  let valid = false
  let availableBackends = settings.Setting('AvailableBackends') || []
  availableBackends.forEach(backend => {
    if (backend.Id === backendId) {
      valid = true
    }
  })
  return valid
})

const selectedInterface = computed(() => interfaces.GetSelected || {})
const isAwg = computed(() => !!selectedInterface.value.AWGEnabled)
const isDisabled = computed(() => !!selectedInterface.value.Disabled)

const trafficStats = computed(() => interfaces.TrafficStats || { Received: 0, Transmitted: 0 })

async function download() {
  await interfaces.LoadInterfaceConfig(interfaces.GetSelected.Identifier)

  // credit: https://www.bitdegree.org/learn/javascript-download
  let text = interfaces.configuration

  let element = document.createElement('a')
  element.setAttribute('href', 'data:application/octet-stream;charset=utf-8,' + encodeURIComponent(text))
  element.setAttribute('download', interfaces.GetSelected.Filename)

  element.style.display = 'none'
  document.body.appendChild(element)

  element.click()
  document.body.removeChild(element)
}

async function saveConfig() {
  try {
    await interfaces.SaveConfiguration(interfaces.GetSelected.Identifier)

    notify({
      title: "Configuration persisted",
      text: "The interface configuration has been written to the wg-quick configuration file.",
      type: 'success',
    })
  } catch (e) {
    console.log(e)
    notify({
      title: "Failed to persist configuration!",
      text: e.toString(),
      type: 'error',
    })
  }
}

async function bulkDelete() {
  if (confirm(t('interfaces.confirm-bulk-delete', {count: selectedPeers.value.length}))) {
    try {
      await peers.BulkDelete(selectedPeers.value)
      selectAll.value = false // reset selection
    } catch (e) {
      // notification is handled in store
    }
  }
}

async function bulkEnable() {
  try {
    await peers.BulkEnable(selectedPeers.value)
    selectAll.value = false
    peers.All.forEach(p => p.IsSelected = false) // remove selection
  } catch (e) {
    // notification is handled in store
  }
}

async function bulkDisable() {
  if (confirm(t('interfaces.confirm-bulk-disable', {count: selectedPeers.value.length}))) {
    try {
      await peers.BulkDisable(selectedPeers.value)
      selectAll.value = false
      peers.All.forEach(p => p.IsSelected = false) // remove selection
    } catch (e) {
      // notification is handled in store
    }
  }
}

function toggleSelectAll() {
  peers.FilteredAndPaged.forEach(peer => {
    peer.IsSelected = selectAll.value;
  });
}

onMounted(async () => {
  await interfaces.LoadInterfaces()
  await peers.LoadPeers(undefined) // use default interface
  await peers.LoadStats(undefined) // use default interface
  startStatsRefreshLoop()
})

// Periodic refresh of peer stats. The server only refreshes its own
// peer-status table on the StatisticsCollector interval (default ~25s),
// and the browser only knows about new traffic counters when it asks,
// so we re-poll every 30s (slightly above the 25s keepalive so a missed
// handshake is still caught within one tick).
//
// Cleanup is enforced in onBeforeUnmount; we also guard against
// overlapping calls (in-flight Promise is dropped on the next tick) and
// skip ticks when the tab is hidden so a backgrounded view doesn't
// hammer the backend.
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
    // store already notifies on failure — don't double-toast
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

function friendlyLastHandshake(ts) {
  if (!ts) return '—'
  try {
    const date = new Date(ts)
    if (isNaN(date.getTime())) return ts
    return date.toLocaleString()
  } catch {
    return ts
  }
}
</script>

<template>
  <PeerViewModal :peerId="viewedPeerId" :visible="viewedPeerId!==''" @close="viewedPeerId=''"></PeerViewModal>
  <PeerEditModal :peerId="editPeerId" :visible="editPeerId!==''" @close="editPeerId=''"></PeerEditModal>
  <PeerMultiCreateModal :visible="multiCreatePeerId!==''" @close="multiCreatePeerId=''"></PeerMultiCreateModal>
  <InterfaceEditModal :interfaceId="editInterfaceId" :visible="editInterfaceId!==''" @close="editInterfaceId=''"></InterfaceEditModal>
  <InterfaceViewModal :interfaceId="viewedInterfaceId" :visible="viewedInterfaceId!==''" @close="viewedInterfaceId=''"></InterfaceViewModal>

  <!-- Page header -->
  <div class="page-header">
    <div>
      <h1>{{ $t('interfaces.headline') }}</h1>
      <p>{{ $t('home.abstract') }}</p>
    </div>
    <div class="page-header__actions">
      <select
        v-model="interfaces.selected"
        :disabled="interfaces.Count===0"
        class="form-select"
        style="max-width: 280px;"
        @change="() => { peers.LoadPeers(); peers.LoadStats() }"
      >
        <option v-if="interfaces.Count===0" value="nothing">{{ $t('interfaces.no-interface.default-selection') }}</option>
        <option v-for="iface in interfaces.All" :key="iface.Identifier" :value="iface.Identifier">
          {{ calculateInterfaceName(iface.Identifier, iface.DisplayName) }}
        </option>
      </select>
      <button class="btn btn-primary" :title="$t('interfaces.button-add-interface')" @click.prevent="editInterfaceId='#NEW#'">
        <i class="fa-solid fa-plus"></i> {{ $t('interfaces.button-add-interface') }}
      </button>
    </div>
  </div>

  <!-- No interfaces -->
  <div v-if="interfaces.Count===0" class="empty-state card">
    <div class="empty-state-icon"><i class="fa-solid fa-network-wired"></i></div>
    <h3>{{ $t('interfaces.no-interface.headline') }}</h3>
    <p>{{ $t('interfaces.no-interface.abstract') }}</p>
    <button class="btn btn-primary" @click.prevent="editInterfaceId='#NEW#'">
      <i class="fa-solid fa-plus"></i> {{ $t('interfaces.button-add-interface') }}
    </button>
  </div>

  <!-- Interface overview -->
  <template v-if="interfaces.Count!==0">
    <div class="card">
      <div class="card-header">
        <div>
          <h3>
            <span style="font-family: var(--font-mono);">{{ selectedInterface.Identifier }}</span>
            <span class="tag" :class="isAwg ? 'tag-awg' : 'tag-wg'" style="margin-left:0.5rem;">{{ isAwg ? 'AmneziaWG' : 'WireGuard' }}</span>
            <span v-if="isDisabled" class="tag tag-disabled" style="margin-left:0.5rem;">
              <i class="fa-solid fa-circle-xmark"></i> {{ $t('general.disabled') || 'Disabled' }}
            </span>
          </h3>
          <div class="text-muted text-muted-sm mt-2" v-if="trafficStats.Received > 0 || trafficStats.Transmitted > 0">
            {{ $t('modals.peer-view.traffic') }}:
            <i class="fa-solid fa-arrow-down me-1"></i>{{ humanFileSize(trafficStats.Received) }}/s
            <i class="fa-solid fa-arrow-up ms-2 me-1"></i>{{ humanFileSize(trafficStats.Transmitted) }}/s
          </div>
        </div>
        <div class="d-flex gap-2">
          <button class="btn btn-ghost btn-icon" :title="$t('interfaces.interface.button-show-config')" @click.prevent="viewedInterfaceId=selectedInterface.Identifier">
            <i class="fas fa-eye"></i>
          </button>
          <button class="btn btn-ghost btn-icon" :title="$t('interfaces.interface.button-download-config')" @click.prevent="download">
            <i class="fas fa-download"></i>
          </button>
          <button v-if="settings.Setting('PersistentConfigSupported')" class="btn btn-ghost btn-icon" :title="$t('interfaces.interface.button-store-config')" @click.prevent="saveConfig">
            <i class="fas fa-save"></i>
          </button>
          <button class="btn btn-secondary btn-sm" :title="$t('interfaces.interface.button-edit')" @click.prevent="editInterfaceId=selectedInterface.Identifier">
            <i class="fas fa-cog me-1"></i> {{ $t('general.edit') || 'Edit' }}
          </button>
        </div>
      </div>
      <div class="card-body">
        <div v-if="selectedInterface.Mode==='server'" class="row g-4">
          <div class="col-sm-6">
            <table class="table table-sm">
              <tbody>
                <tr>
                  <td class="text-muted" style="width: 40%;">{{ $t('interfaces.interface.key') }}</td>
                  <td class="text-mono-sm text-wrap">{{ selectedInterface.PublicKey }}</td>
                </tr>
                <tr>
                  <td class="text-muted">{{ $t('interfaces.interface.endpoint') }}</td>
                  <td>{{ selectedInterface.PeerDefEndpoint || '—' }}</td>
                </tr>
                <tr>
                  <td class="text-muted">{{ $t('interfaces.interface.protocol') }}</td>
                  <td><span class="tag" :class="isAwg ? 'tag-awg' : 'tag-wg'">{{ isAwg ? 'AmneziaWG' : 'WireGuard' }}</span></td>
                </tr>
                <tr>
                  <td class="text-muted">{{ $t('interfaces.interface.port') }}</td>
                  <td class="text-mono-sm">{{ selectedInterface.ListenPort }}</td>
                </tr>
                <tr>
                  <td class="text-muted">{{ $t('interfaces.interface.peers') }}</td>
                  <td>{{ selectedInterface.EnabledPeers }}</td>
                </tr>
                <tr>
                  <td class="text-muted">{{ $t('interfaces.interface.total-peers') }}</td>
                  <td>{{ selectedInterface.TotalPeers }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="col-sm-6">
            <table class="table table-sm">
              <tbody>
                <tr>
                  <td class="text-muted" style="width: 40%;">{{ $t('interfaces.interface.ip') }}</td>
                  <td>
                    <span class="tag tag-info me-1" v-for="addr in selectedInterface.Addresses" :key="addr">{{ addr }}</span>
                  </td>
                </tr>
                <tr>
                  <td class="text-muted">{{ $t('interfaces.interface.mtu') }}</td>
                  <td>{{ selectedInterface.Mtu }}</td>
                </tr>
                <tr>
                  <td class="text-muted">{{ $t('interfaces.interface.default-dns') }}</td>
                  <td>
                    <span class="tag tag-secondary me-1" v-for="addr in selectedInterface.PeerDefDns" :key="addr">{{ addr }}</span>
                    <span v-if="!selectedInterface.PeerDefDns?.length" class="text-muted-sm">—</span>
                  </td>
                </tr>
                <tr>
                  <td class="text-muted">{{ $t('interfaces.interface.default-keep-alive') }}</td>
                  <td>{{ selectedInterface.PeerDefPersistentKeepalive }}</td>
                </tr>
                <tr>
                  <td class="text-muted">{{ $t('interfaces.interface.default-allowed-ip') }}</td>
                  <td>
                    <span class="tag tag-secondary me-1" v-for="addr in selectedInterface.PeerDefAllowedIPs" :key="addr">{{ addr }}</span>
                    <span v-if="!selectedInterface.PeerDefAllowedIPs?.length" class="text-muted-sm">—</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <div v-else-if="selectedInterface.Mode==='client'" class="row g-4">
          <div class="col-sm-6">
            <table class="table table-sm">
              <tbody>
                <tr>
                  <td class="text-muted" style="width: 40%;">{{ $t('interfaces.interface.key') }}</td>
                  <td class="text-mono-sm text-wrap">{{ selectedInterface.PublicKey }}</td>
                </tr>
                <tr>
                  <td class="text-muted">{{ $t('interfaces.interface.endpoints') }}</td>
                  <td>{{ selectedInterface.EnabledPeers }}</td>
                </tr>
                <tr>
                  <td class="text-muted">{{ $t('interfaces.interface.total-endpoints') }}</td>
                  <td>{{ selectedInterface.TotalPeers }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="col-sm-6">
            <table class="table table-sm">
              <tbody>
                <tr>
                  <td class="text-muted" style="width: 40%;">{{ $t('interfaces.interface.ip') }}</td>
                  <td>
                    <span class="tag tag-info me-1" v-for="addr in selectedInterface.Addresses" :key="addr">{{ addr }}</span>
                  </td>
                </tr>
                <tr>
                  <td class="text-muted">{{ $t('interfaces.interface.dns') }}</td>
                  <td>
                    <span class="tag tag-secondary me-1" v-for="addr in selectedInterface.Dns" :key="addr">{{ addr }}</span>
                    <span v-if="!selectedInterface.Dns?.length" class="text-muted-sm">—</span>
                  </td>
                </tr>
                <tr>
                  <td class="text-muted">{{ $t('interfaces.interface.mtu') }}</td>
                  <td>{{ selectedInterface.Mtu }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Peer list -->
    <div class="page-header mt-6 mb-4" style="margin-bottom: 1rem;">
      <div>
        <h2 style="font-size: 1.125rem;">
          <span v-if="selectedInterface.Mode==='server'">{{ $t('interfaces.headline-peers') }}</span>
          <span v-else>{{ $t('interfaces.headline-endpoints') }}</span>
        </h2>
      </div>
      <div class="page-header__actions">
        <div class="input-group" style="max-width: 280px;">
          <input v-model="peers.filter" class="form-control" :placeholder="$t('general.search.placeholder')" type="text" @keyup="peers.afterPageSizeChange">
        </div>
        <button class="btn btn-secondary" :title="$t('interfaces.button-add-peers')" @click.prevent="multiCreatePeerId='#NEW#'">
          <i class="fa fa-plus me-1"></i><i class="fa fa-users"></i>
        </button>
        <button class="btn btn-primary" :title="$t('interfaces.button-add-peer')" @click.prevent="editPeerId='#NEW#'">
          <i class="fa-solid fa-plus me-1"></i> {{ $t('interfaces.button-add-peer') }}
        </button>
      </div>
    </div>

    <div class="d-flex gap-2 mb-3" v-if="selectedPeers.length > 0">
      <button class="btn btn-outline-primary btn-sm" :title="$t('interfaces.button-bulk-enable')" @click.prevent="bulkEnable">
        <i class="fa-regular fa-circle-check"></i>
      </button>
      <button class="btn btn-outline-primary btn-sm" :title="$t('interfaces.button-bulk-disable')" @click.prevent="bulkDisable">
        <i class="fa fa-ban"></i>
      </button>
      <button class="btn btn-outline-danger btn-sm" :title="$t('interfaces.button-bulk-delete')" @click.prevent="bulkDelete">
        <i class="fa fa-trash-can"></i>
      </button>
      <span class="text-muted-sm ms-2 align-self-center">{{ selectedPeers.length }} {{ $t('general.selected') || 'selected' }}</span>
    </div>

    <div v-if="peers.Count===0" class="empty-state card">
      <div class="empty-state-icon"><i class="fa-solid fa-user-slash"></i></div>
      <h3>{{ $t('interfaces.no-peer.headline') }}</h3>
      <p>{{ $t('interfaces.no-peer.abstract') }}</p>
    </div>

    <div v-else class="card">
      <table class="data-table">
        <thead>
          <tr>
            <th style="width: 32px;">
              <input class="form-check-input" :title="$t('general.select-all')" type="checkbox" v-model="selectAll" @change="toggleSelectAll">
            </th>
            <th style="width: 50px;"></th>
            <th class="th-sort" @click="sortBy('DisplayName')">
              {{ $t("interfaces.table-heading.name") }}
              <i v-if="sortKey === 'DisplayName'" :class="sortOrder === 1 ? 'asc' : 'desc'"></i>
            </th>
            <th class="th-sort" @click="sortBy('UserIdentifier')">
              {{ $t("interfaces.table-heading.user") }}
              <i v-if="sortKey === 'UserIdentifier'" :class="sortOrder === 1 ? 'asc' : 'desc'"></i>
            </th>
            <th class="th-sort" @click="sortBy('Addresses')">
              {{ $t("interfaces.table-heading.ip") }}
              <i v-if="sortKey === 'Addresses'" :class="sortOrder === 1 ? 'asc' : 'desc'"></i>
            </th>
            <th v-if="selectedInterface.Mode === 'client'">
              {{ $t("interfaces.table-heading.endpoint") }}
            </th>
            <th v-if="peers.hasStatistics" class="th-sort" @click="sortBy('IsConnected')">
              {{ $t("interfaces.table-heading.status") }}
              <i v-if="sortKey === 'IsConnected'" :class="sortOrder === 1 ? 'asc' : 'desc'"></i>
            </th>
            <th v-if="peers.hasStatistics" class="th-sort" @click="sortBy('Traffic')">
              RX/TX
              <i v-if="sortKey === 'Traffic'" :class="sortOrder === 1 ? 'asc' : 'desc'"></i>
            </th>
            <th class="text-end" style="width: 100px;"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="peer in peers.FilteredAndPaged" :key="peer.Identifier">
            <td>
              <input class="form-check-input" type="checkbox" v-model="peer.IsSelected">
            </td>
            <td class="text-center">
              <span v-if="peer.Disabled" class="text-danger" :title="$t('interfaces.peer-disabled') + ' ' + peer.DisabledReason"><i class="fa fa-circle-xmark"></i></span>
              <span v-else-if="peer.ExpiresAt" class="text-warning" :title="$t('interfaces.peer-expiring') + ' ' + peer.ExpiresAt"><i class="fas fa-hourglass-end"></i></span>
              <span v-else class="text-success"><i class="fa-solid fa-circle-check"></i></span>
            </td>
            <td class="peer-name">
              <span v-if="peer.DisplayName">{{ peer.DisplayName }}</span>
              <span v-else class="text-mono-sm">{{ $filters.truncate(peer.Identifier, 12) }}</span>
            </td>
            <td>{{ peer.UserIdentifier || '—' }}</td>
            <td>
              <span class="tag tag-info me-1" v-for="ip in peer.Addresses" :key="ip">{{ ip }}</span>
            </td>
            <td v-if="selectedInterface.Mode==='client'" class="text-mono-sm">{{ peer.Endpoint?.Value || '—' }}</td>
            <td v-if="peers.hasStatistics">
              <span v-if="peers.Statistics(peer.Identifier).IsConnected" class="d-inline-flex align-items-center gap-2">
                <span class="status-dot active"></span>
                <span class="text-success fw-500">{{ $t('interfaces.peer-connected') }}</span>
              </span>
              <span v-else class="d-inline-flex align-items-center gap-2">
                <span class="status-dot inactive"></span>
                <span class="text-muted">{{ $t('interfaces.peer-not-connected') }}</span>
              </span>
            </td>
            <td v-if="peers.hasStatistics" class="text-mono-sm">
              <span :title="humanFileSize(peers.Statistics(peer.Identifier).BytesReceived) + ' / ' + humanFileSize(peers.Statistics(peer.Identifier).BytesTransmitted)">
                <i class="fa-solid fa-arrow-down text-muted me-1"></i>{{ humanFileSize(peers.TrafficStats(peer.Identifier).Received) }}/s
                <i class="fa-solid fa-arrow-up text-muted ms-2 me-1"></i>{{ humanFileSize(peers.TrafficStats(peer.Identifier).Transmitted) }}/s
              </span>
            </td>
            <td>
              <div class="cell-actions">
                <button class="btn btn-ghost btn-icon" :title="$t('interfaces.button-show-peer')" @click.prevent="viewedPeerId=peer.Identifier">
                  <i class="fas fa-eye"></i>
                </button>
                <button class="btn btn-ghost btn-icon" :title="$t('interfaces.button-edit-peer')" @click.prevent="editPeerId=peer.Identifier">
                  <i class="fas fa-cog"></i>
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <Pagination
      v-if="peers.Count!==0"
      class="mt-4"
      :currentPage="peers.currentPage"
      :totalCount="peers.FilteredCount"
      :pageSize="peers.pageSize"
      :hasNextPage="peers.hasNextPage"
      :hasPrevPage="peers.hasPrevPage"
      :onGotoPage="peers.gotoPage"
      :onNextPage="peers.nextPage"
      :onPrevPage="peers.previousPage"
    />

    <div class="d-flex justify-content-end align-items-center gap-3 mt-3" v-if="peers.Count!==0">
      <label class="text-muted-sm" for="paginationSelector">{{ $t('general.pagination.size') }}:</label>
      <select id="paginationSelector" v-model.number="peers.pageSize" class="form-select" style="max-width: 120px;" @change="peers.afterPageSizeChange()">
        <option value="10">10</option>
        <option value="25">25</option>
        <option value="50">50</option>
        <option value="100">100</option>
        <option value="999999999">{{ $t('general.pagination.all') }}</option>
      </select>
    </div>
  </template>
</template>