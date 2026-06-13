<script setup>
import { onMounted } from "vue";
import {auditStore} from "@/stores/audit";
import Pagination from "@/components/Pagination.vue";

const audit = auditStore()

onMounted(async () => {
  await audit.LoadEntries()
})

</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ $t('audit.headline') }}</h1>
      <p>{{ $t('audit.abstract') }}</p>
    </div>
    <div class="page-header__actions">
      <input v-model="audit.filter" class="form-control" :placeholder="$t('general.search.placeholder')" type="text" @keyup="audit.afterPageSizeChange" style="max-width:280px;">
    </div>
  </div>

  <div v-if="audit.Count===0" class="empty-state card">
    <div class="empty-state-icon"><i class="fa-solid fa-file-shield"></i></div>
    <h3>{{ $t('audit.no-entries.headline') }}</h3>
    <p>{{ $t('audit.no-entries.abstract') }}</p>
  </div>

  <div v-else class="card">
    <div class="card-header">
      <h3>{{ $t('audit.entries-headline') }}</h3>
      <span class="text-muted-sm">{{ audit.Count }} {{ $t('audit.entries-headline').toLowerCase() }}</span>
    </div>
    <table class="data-table">
      <thead>
      <tr>
        <th style="width: 50px;">#</th>
        <th>{{ $t('audit.table-heading.time') }}</th>
        <th>{{ $t('audit.table-heading.severity') }}</th>
        <th>{{ $t('audit.table-heading.user') }}</th>
        <th>{{ $t('audit.table-heading.origin') }}</th>
        <th>{{ $t('audit.table-heading.message') }}</th>
      </tr>
      </thead>
      <tbody>
      <tr v-for="entry in audit.FilteredAndPaged" :key="entry.Id">
        <td class="text-muted-sm">{{ entry.Id }}</td>
        <td class="text-mono-sm">{{ entry.Timestamp }}</td>
        <td>
          <span class="tag" :class="entry.Severity === 'low' ? 'tag-info' : entry.Severity === 'medium' ? 'tag-warning' : 'tag-danger'">
            {{ entry.Severity }}
          </span>
        </td>
        <td>{{ entry.ContextUser }}</td>
        <td>{{ entry.Origin }}</td>
        <td>{{ entry.Message }}</td>
      </tr>
      </tbody>
    </table>
  </div>

  <Pagination
    v-if="audit.Count!==0"
    class="mt-4"
    :currentPage="audit.currentPage"
    :totalCount="audit.FilteredCount"
    :pageSize="audit.pageSize"
    :hasNextPage="audit.hasNextPage"
    :hasPrevPage="audit.hasPrevPage"
    :onGotoPage="audit.gotoPage"
    :onNextPage="audit.nextPage"
    :onPrevPage="audit.previousPage"
  />

  <div class="d-flex justify-content-end align-items-center gap-3 mt-3" v-if="audit.Count !== 0">
    <label class="text-muted-sm" for="paginationSelector">{{ $t('general.pagination.size') }}:</label>
    <select id="paginationSelector" v-model.number="audit.pageSize" class="form-select" style="max-width: 120px;" @change="audit.afterPageSizeChange()">
      <option value="10">10</option>
      <option value="25">25</option>
      <option value="50">50</option>
      <option value="100">100</option>
      <option value="999999999">{{ $t('general.pagination.all') }}</option>
    </select>
  </div>
</template>