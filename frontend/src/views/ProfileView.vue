<script setup>
import PeerViewModal from "../components/PeerViewModal.vue";

import { onMounted, ref, computed } from "vue";
import { useI18n } from "vue-i18n";
import { profileStore } from "@/stores/profile";
import { peerStore } from "@/stores/peers";
import UserPeerEditModal from "@/components/UserPeerEditModal.vue";
import Pagination from "@/components/Pagination.vue";
import { settingsStore } from "@/stores/settings";
import { humanFileSize } from "@/helpers/utils";

const settings = settingsStore()
const profile = profileStore()
const peers = peerStore()

const { t } = useI18n()

const viewedPeerId = ref("")
const editPeerId = ref("")

const sortKey = ref("")
const sortOrder = ref(1)
const selectAll = ref(false)

const selectedPeers = computed(() => {
  return profile.Peers.filter(peer => peer.IsSelected).map(peer => peer.Identifier);
})

function sortBy(key) {
  if (sortKey.value === key) {
    sortOrder.value = sortOrder.value * -1; // Toggle sort order
  } else {
    sortKey.value = key;
    sortOrder.value = 1; // Default to ascending
  }
  profile.sortKey = sortKey.value;
  profile.sortOrder = sortOrder.value;
}

function friendlyInterfaceName(id, name) {
  if (name) {
    return name
  }
  return id
}

async function bulkDelete() {
  if (confirm(t('interfaces.confirm-bulk-delete', {count: selectedPeers.value.length}))) {
    try {
      await profile.BulkDelete(selectedPeers.value)
      selectAll.value = false // reset selection
    } catch (e) {
      // notification is handled in store
    }
  }
}

function toggleSelectAll() {
  profile.FilteredAndPagedPeers.forEach(peer => {
    peer.IsSelected = selectAll.value;
  });
}

const userInitial = computed(() => {
  let n = 'U'
  if (profile.user?.Firstname) n = profile.user.Firstname[0]
  else if (profile.user?.Identifier) n = profile.user.Identifier[0]
  return (n || 'U').toUpperCase()
})

const memberSince = computed(() => {
  if (!profile.user?.CreatedAt) return '—'
  try {
    return new Date(profile.user.CreatedAt).toLocaleDateString()
  } catch {
    return profile.user.CreatedAt
  }
})

onMounted(async () => {
  await profile.LoadUser()
  await profile.LoadPeers()
  await profile.LoadStats()
  await profile.LoadInterfaces()
})

</script>

<template>
  <PeerViewModal :peerId="viewedPeerId" :visible="viewedPeerId !== ''" @close="viewedPeerId = ''"></PeerViewModal>
  <UserPeerEditModal :peerId="editPeerId" :visible="editPeerId !== ''" @close="editPeerId = ''; profile.LoadPeers()"></UserPeerEditModal>

  <!-- Page header -->
  <div class="page-header">
    <div>
      <h1>{{ $t('profile.headline') }}</h1>
      <p>{{ $t('home.abstract') }}</p>
    </div>
    <div class="page-header__actions">
      <input
        v-model="profile.filter"
        class="form-control"
        :placeholder="$t('general.search.placeholder')"
        type="text"
        @keyup="profile.afterPageSizeChange"
        style="max-width: 240px;"
      >
      <button
        v-if="settings.Setting('SelfProvisioning') && profile.CountInterfaces>0"
        class="btn btn-primary"
        :title="$t('interfaces.button-add-peer')"
        @click.prevent="editPeerId = '#NEW#'"
      >
        <i class="fa-solid fa-plus me-1"></i> {{ $t('interfaces.button-add-peer') }}
      </button>
    </div>
  </div>

  <div class="profile-grid">
    <!-- Sidebar: user card -->
    <div class="profile-sidebar">
      <div class="profile-sidebar__info">
        <div class="profile-sidebar__avatar">{{ userInitial }}</div>
        <div class="profile-sidebar__name">{{ profile.user?.Identifier || '—' }}</div>
        <div class="profile-sidebar__email">{{ profile.user?.Email || '' }}</div>
        <span class="tag tag-admin" v-if="profile.user?.IsAdmin">{{ $t('users.admin') }}</span>
        <span class="tag tag-user" v-else>{{ $t('users.no-admin') }}</span>
      </div>
      <div class="profile-sidebar__stats">
        <div class="profile-sidebar__row">
          <span class="label">{{ $t('users.table-heading.peers') }}</span>
          <span class="value">{{ profile.CountPeers }}</span>
        </div>
        <div class="profile-sidebar__row">
          <span class="label">{{ $t('users.table-heading.sources') }}</span>
          <span class="value">{{ profile.user?.Source || '—' }}</span>
        </div>
      </div>
    </div>

    <!-- Main: peer list -->
    <div>
      <div class="card">
        <div class="card-header">
          <h3>{{ $t('profile.headline') }}</h3>
          <span class="text-muted-sm">{{ profile.CountPeers }} {{ $t('interfaces.headline-peers') }}</span>
        </div>

        <div v-if="selectedPeers.length > 0" class="d-flex gap-2 px-6 pt-3">
          <button class="btn btn-outline-danger btn-sm" :title="$t('interfaces.button-bulk-delete')" @click.prevent="bulkDelete">
            <i class="fa fa-trash-can"></i>
          </button>
          <span class="text-muted-sm ms-2 align-self-center">{{ selectedPeers.length }} {{ $t('general.selected') || 'selected' }}</span>
        </div>

        <div v-if="profile.CountPeers === 0" class="empty-state">
          <div class="empty-state-icon"><i class="fa-solid fa-user-slash"></i></div>
          <h3>{{ $t('profile.no-peer.headline') }}</h3>
          <p>{{ $t('profile.no-peer.abstract') }}</p>
        </div>

        <table v-else class="data-table">
          <thead>
            <tr>
              <th style="width: 32px;">
                <input class="form-check-input" :title="$t('general.select-all')" type="checkbox" v-model="selectAll" @change="toggleSelectAll">
              </th>
              <th style="width: 50px;"></th>
              <th class="th-sort" @click="sortBy('DisplayName')">
                {{ $t("profile.table-heading.name") }}
                <i v-if="sortKey === 'DisplayName'" :class="sortOrder === 1 ? 'asc' : 'desc'"></i>
              </th>
              <th class="th-sort" @click="sortBy('Addresses')">
                {{ $t("profile.table-heading.ip") }}
                <i v-if="sortKey === 'Addresses'" :class="sortOrder === 1 ? 'asc' : 'desc'"></i>
              </th>
              <th v-if="profile.hasStatistics" class="th-sort" @click="sortBy('IsConnected')">
                {{ $t("profile.table-heading.stats") }}
                <i v-if="sortKey === 'IsConnected'" :class="sortOrder === 1 ? 'asc' : 'desc'"></i>
              </th>
              <th v-if="profile.hasStatistics" class="th-sort" @click="sortBy('Traffic')">
                RX/TX
                <i v-if="sortKey === 'Traffic'" :class="sortOrder === 1 ? 'asc' : 'desc'"></i>
              </th>
              <th>{{ $t('profile.table-heading.interface') }}</th>
              <th class="text-end" style="width: 100px;"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="peer in profile.FilteredAndPagedPeers" :key="peer.Identifier">
              <td>
                <input class="form-check-input" type="checkbox" v-model="peer.IsSelected">
              </td>
              <td class="text-center">
                <span v-if="peer.Disabled" class="text-danger" :title="peer.DisabledReason">
                  <i class="fa fa-circle-xmark"></i>
                </span>
                <span v-else-if="peer.ExpiresAt" class="text-warning" :title="peer.ExpiresAt">
                  <i class="fas fa-hourglass-end"></i>
                </span>
                <span v-else-if="profile.Statistics(peer.Identifier).IsConnected" class="text-success" :title="$t('interfaces.peer-connected')">
                  <i class="fa-solid fa-circle-check"></i>
                </span>
                <span v-else class="text-muted">
                  <i class="fa-solid fa-circle"></i>
                </span>
              </td>
              <td class="peer-name">
                <span v-if="peer.DisplayName">{{ peer.DisplayName }}</span>
                <span v-else class="text-mono-sm">{{ $filters.truncate(peer.Identifier, 12) }}</span>
              </td>
              <td>
                <span class="tag tag-info me-1" v-for="ip in peer.Addresses" :key="ip">{{ ip }}</span>
              </td>
              <td v-if="profile.hasStatistics">
                <div v-if="profile.Statistics(peer.Identifier).IsConnected" class="d-flex align-items-center gap-2">
                  <span class="status-dot active"></span>
                  <span class="text-success fw-500">{{ $t('interfaces.peer-connected') }}</span>
                  <span class="text-muted-sm" :title="$t('interfaces.peer-handshake') + ' ' + profile.Statistics(peer.Identifier).LastHandshake">
                    <i class="fa-regular fa-clock"></i> {{ profile.Statistics(peer.Identifier).LastHandshake }}
                  </span>
                </div>
                <div v-else class="d-flex align-items-center gap-2">
                  <span class="status-dot inactive"></span>
                  <span class="text-muted">{{ $t('interfaces.peer-not-connected') }}</span>
                </div>
              </td>
              <td v-if="profile.hasStatistics" class="text-mono-sm">
                <div :title="humanFileSize(profile.Statistics(peer.Identifier).BytesReceived) + ' / ' + humanFileSize(profile.Statistics(peer.Identifier).BytesTransmitted)">
                  <div><i class="fa-solid fa-arrow-down text-muted me-1"></i>{{ humanFileSize(profile.Statistics(peer.Identifier).BytesReceived) }}</div>
                  <div><i class="fa-solid fa-arrow-up text-muted me-1"></i>{{ humanFileSize(profile.Statistics(peer.Identifier).BytesTransmitted) }}</div>
                </div>
              </td>
              <td>{{ friendlyInterfaceName(peer.InterfaceIdentifier, peer.InterfaceDisplayName) }}</td>
              <td>
                <div class="cell-actions">
                  <button class="btn btn-ghost btn-icon" :title="$t('profile.button-show-peer')" @click.prevent="viewedPeerId = peer.Identifier">
                    <i class="fas fa-eye"></i>
                  </button>
                  <button
                    v-if="settings.Setting('SelfProvisioning') && profile.HasInterface(peer.InterfaceIdentifier)"
                    class="btn btn-ghost btn-icon"
                    :title="$t('profile.button-edit-peer')"
                    @click.prevent="editPeerId = peer.Identifier"
                  >
                    <i class="fas fa-cog"></i>
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <Pagination
        class="mt-4"
        :currentPage="profile.currentPage"
        :totalCount="profile.FilteredPeerCount"
        :pageSize="profile.pageSize"
        :hasNextPage="profile.hasNextPage"
        :hasPrevPage="profile.hasPrevPage"
        :onGotoPage="profile.gotoPage"
        :onNextPage="profile.nextPage"
        :onPrevPage="profile.previousPage"
      />

      <div class="d-flex justify-content-end align-items-center gap-3 mt-3" v-if="profile.CountPeers !== 0">
        <label class="text-muted-sm" for="paginationSelector">{{ $t('general.pagination.size') }}:</label>
        <select id="paginationSelector" v-model.number="profile.pageSize" class="form-select" style="max-width: 120px;" @change="profile.afterPageSizeChange()">
          <option value="10">10</option>
          <option value="25">25</option>
          <option value="50">50</option>
          <option value="100">100</option>
          <option value="999999999">{{ $t('general.pagination.all') }}</option>
        </select>
      </div>
    </div>
  </div>
</template>