<script setup>
import {userStore} from "@/stores/users";
import {ref, onMounted, computed} from "vue";
import UserEditModal from "@/components/UserEditModal.vue";
import UserViewModal from "@/components/UserViewModal.vue";
import Pagination from "@/components/Pagination.vue";
import {useI18n} from "vue-i18n";

const users = userStore()
const { t } = useI18n()

const editUserId = ref("")
const viewedUserId = ref("")

const selectAll = ref(false)

const selectedUsers = computed(() => {
  return users.All.filter(user => user.IsSelected).map(user => user.Identifier);
})

async function bulkDelete() {
  if (confirm(t('users.confirm-bulk-delete', {count: selectedUsers.value.length}))) {
    try {
      await users.BulkDelete(selectedUsers.value)
      selectAll.value = false // reset selection
    } catch (e) {
      // notification is handled in store
    }
  }
}

async function bulkEnable() {
  try {
    await users.BulkEnable(selectedUsers.value)
    selectAll.value = false
    users.All.forEach(u => u.IsSelected = false) // remove selection
  } catch (e) {
    // notification is handled in store
  }
}

async function bulkDisable() {
  if (confirm(t('users.confirm-bulk-disable', {count: selectedUsers.value.length}))) {
    try {
      await users.BulkDisable(selectedUsers.value)
      selectAll.value = false
      users.All.forEach(u => u.IsSelected = false) // remove selection
    } catch (e) {
      // notification is handled in store
    }
  }
}

async function bulkLock() {
  if (confirm(t('users.confirm-bulk-lock', {count: selectedUsers.value.length}))) {
    try {
      await users.BulkLock(selectedUsers.value)
      selectAll.value = false
      users.All.forEach(u => u.IsSelected = false) // remove selection
    } catch (e) {
      // notification is handled in store
    }
  }
}

async function bulkUnlock() {
  try {
    await users.BulkUnlock(selectedUsers.value)
    selectAll.value = false
    users.All.forEach(u => u.IsSelected = false) // remove selection
  } catch (e) {
    // notification is handled in store
  }
}

function toggleSelectAll() {
  users.FilteredAndPaged.forEach(user => {
    user.IsSelected = selectAll.value;
  });
}

onMounted(() => {
  users.LoadUsers()
})
</script>

<template>
  <UserEditModal :userId="editUserId" :visible="editUserId!==''" @close="editUserId=''"></UserEditModal>
  <UserViewModal :userId="viewedUserId" :visible="viewedUserId!==''" @close="viewedUserId=''"></UserViewModal>

  <!-- Page header -->
  <div class="page-header">
    <div>
      <h1>{{ $t('users.headline') }}</h1>
      <p>{{ $t('home.abstract') }}</p>
    </div>
    <div class="page-header__actions">
      <input v-model="users.filter" class="form-control" :placeholder="$t('general.search.placeholder')" type="text" @keyup="users.afterPageSizeChange" style="max-width: 280px;">
      <button class="btn btn-primary" :title="$t('users.button-add-user')" @click.prevent="editUserId='#NEW#'">
        <i class="fa-solid fa-plus me-1"></i> {{ $t('users.button-add-user') }}
      </button>
    </div>
  </div>

  <div class="d-flex gap-2 mb-3" v-if="selectedUsers.length > 0">
    <button class="btn btn-outline-primary btn-sm" :title="$t('users.button-bulk-enable')" @click.prevent="bulkEnable"><i class="fa-regular fa-circle-check"></i></button>
    <button class="btn btn-outline-primary btn-sm" :title="$t('users.button-bulk-disable')" @click.prevent="bulkDisable"><i class="fa fa-ban"></i></button>
    <button class="btn btn-outline-primary btn-sm" :title="$t('users.button-bulk-unlock')" @click.prevent="bulkUnlock"><i class="fa-solid fa-lock-open"></i></button>
    <button class="btn btn-outline-primary btn-sm" :title="$t('users.button-bulk-lock')" @click.prevent="bulkLock"><i class="fa-solid fa-lock"></i></button>
    <button class="btn btn-outline-danger btn-sm" :title="$t('users.button-bulk-delete')" @click.prevent="bulkDelete"><i class="fa fa-trash-can"></i></button>
    <span class="text-muted-sm ms-2 align-self-center">{{ selectedUsers.length }} {{ $t('general.selected') }}</span>
  </div>

  <div v-if="users.Count===0" class="empty-state card">
    <div class="empty-state-icon"><i class="fa-solid fa-users-slash"></i></div>
    <h3>{{ $t('users.no-user.headline') }}</h3>
    <p>{{ $t('users.no-user.abstract') }}</p>
  </div>

  <div v-else class="card">
    <table class="data-table">
      <thead>
        <tr>
          <th style="width: 32px;">
            <input class="form-check-input" :title="$t('general.select-all')" type="checkbox" v-model="selectAll" @change="toggleSelectAll">
          </th>
          <th style="width: 60px;"></th>
          <th>{{ $t('users.table-heading.id') }}</th>
          <th>{{ $t('users.table-heading.email') }}</th>
          <th>{{ $t('users.table-heading.firstname') }}</th>
          <th>{{ $t('users.table-heading.lastname') }}</th>
          <th>{{ $t('users.table-heading.sources') }}</th>
          <th class="text-center">{{ $t('users.table-heading.peers') }}</th>
          <th class="text-center">{{ $t('users.table-heading.admin') }}</th>
          <th class="text-end" style="width: 100px;"></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="user in users.FilteredAndPaged" :key="user.Identifier">
          <td>
            <input class="form-check-input" type="checkbox" v-model="user.IsSelected">
          </td>
          <td class="text-center">
            <span v-if="user.Disabled" class="text-danger" :title="$t('users.user-disabled') + ' ' + user.DisabledReason"><i class="fa fa-circle-xmark"></i></span>
            <span v-else-if="user.Locked" class="text-warning" :title="$t('users.user-locked') + ' ' + user.LockedReason"><i class="fas fa-lock"></i></span>
            <span v-else class="text-success"><i class="fa-solid fa-circle-check"></i></span>
          </td>
          <td class="peer-name">{{ user.Identifier }}</td>
          <td>{{ user.Email }}</td>
          <td>{{ user.Firstname }}</td>
          <td>{{ user.Lastname }}</td>
          <td>
            <span class="tag tag-secondary me-1" v-for="src in user.AuthSources" :key="src">{{ src }}</span>
          </td>
          <td class="text-center text-mono-sm">{{ user.PeerCount }}</td>
          <td class="text-center">
            <span v-if="user.IsAdmin" class="tag tag-success" :title="$t('users.admin')">
              <i class="fa-solid fa-shield-halved"></i> {{ $t('users.admin') }}
            </span>
            <span v-else class="text-muted">—</span>
          </td>
          <td>
            <div class="cell-actions">
              <button class="btn btn-ghost btn-icon" :title="$t('users.button-show-user')" @click.prevent="viewedUserId=user.Identifier">
                <i class="fas fa-eye"></i>
              </button>
              <button class="btn btn-ghost btn-icon" :title="$t('users.button-edit-user')" @click.prevent="editUserId=user.Identifier">
                <i class="fas fa-cog"></i>
              </button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>

  <Pagination
    v-if="users.Count!==0"
    class="mt-4"
    :currentPage="users.currentPage"
    :totalCount="users.FilteredCount"
    :pageSize="users.pageSize"
    :hasNextPage="users.hasNextPage"
    :hasPrevPage="users.hasPrevPage"
    :onGotoPage="users.gotoPage"
    :onNextPage="users.nextPage"
    :onPrevPage="users.previousPage"
  />

  <div class="d-flex justify-content-end align-items-center gap-3 mt-3" v-if="users.Count !== 0">
    <label class="text-muted-sm" for="paginationSelector">{{ $t('general.pagination.size') }}:</label>
    <select id="paginationSelector" v-model.number="users.pageSize" class="form-select" style="max-width: 120px;" @change="users.afterPageSizeChange()">
      <option value="10">10</option>
      <option value="25">25</option>
      <option value="50">50</option>
      <option value="100">100</option>
      <option value="999999999">{{ $t('general.pagination.all') }}</option>
    </select>
  </div>
</template>