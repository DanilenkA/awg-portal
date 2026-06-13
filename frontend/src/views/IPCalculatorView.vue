<script setup>

import {ref, watch, computed} from "vue";
import isCidr from "is-cidr";
import {isIP} from "is-ip";
import {excludeCidr} from "cidr-tools";
import {useI18n} from 'vue-i18n';

const allowedIp = ref("0.0.0.0/0, ::/0")
const dissallowedIp = ref("")
const privateIP = ref("10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16")

const {t} = useI18n()

const errorAllowed = ref("")
const errorDissallowed = ref("")

function validateIpAndCidrList(value) {
  const list = value.split(",").map(v => v.trim()).filter(Boolean);
  if (list.length === 0) {
    return t('calculator.allowed-ip.empty');
  }

  for (const addr of list) {
    if (!isIP(addr) && !isCidr(addr)) {
      return t('calculator.dissallowed-ip.invalid', {addr});
    }
  }
  return true;
}

watch(allowedIp, (newValue) => {
  const result = validateIpAndCidrList(newValue);
  errorAllowed.value = result === true ? "" : result;
});

watch(dissallowedIp, (newValue) => {
  if (!allowedIp.value || allowedIp.value.trim() === "") {
    allowedIp.value = "0.0.0.0/0";
  }
  const result = validateIpAndCidrList(newValue);
  errorDissallowed.value = result === true ? "" : result;
});

const newAllowedIp = computed(() => {
  if (errorAllowed.value || errorDissallowed.value) return "";

  try {
    const allowedList = allowedIp.value.split(",").map(v => v.trim()).filter(Boolean);
    const disallowedList = dissallowedIp.value.split(",").map(v => v.trim()).filter(Boolean);

    const result = excludeCidr(allowedList, disallowedList);

    return result.join(", ");
  } catch (e) {
    console.error("Allowed IPs calculation error:", e);
    return "";
  }
});

function addPrivateIPs() {
  const privateList = privateIP.value.split(",").map(v => v.trim());
  const currentList = dissallowedIp.value.split(",").map(v => v.trim()).filter(Boolean);

  const combined = Array.from(new Set([...currentList, ...privateList]));
  dissallowedIp.value = combined.join(", ");
}

const copyStatus = ref(false)
async function copyResult() {
  if (!newAllowedIp.value) return
  try {
    await navigator.clipboard.writeText(newAllowedIp.value)
    copyStatus.value = true
    setTimeout(() => { copyStatus.value = false }, 1500)
  } catch (e) {
    // ignore
  }
}
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ $t('calculator.headline') }}</h1>
      <p>{{ $t('calculator.abstract') }}</p>
    </div>
  </div>

  <div class="row g-4">
    <div class="col-12 col-lg-6">
      <div class="card">
        <div class="card-header">
          <h3>{{ $t('calculator.allowed-ip.label') }}</h3>
        </div>
        <div class="card-body">
          <div class="form-group">
            <label class="form-label">{{ $t('calculator.allowed-ip.label') }}</label>
            <input class="form-control" v-model="allowedIp" :placeholder="$t('calculator.allowed-ip.placeholder')" :class="{ 'is-invalid': errorAllowed }">
            <div v-if="errorAllowed" class="text-danger mt-1">{{ errorAllowed }}</div>
            <div class="form-hint">{{ $t('calculator.allowed-ip.placeholder') }}</div>
          </div>
          <div class="form-group">
            <label class="form-label">{{ $t('calculator.dissallowed-ip.label') }}</label>
            <input class="form-control" v-model="dissallowedIp" :placeholder="$t('calculator.dissallowed-ip.placeholder')" :class="{ 'is-invalid': errorDissallowed }">
            <div v-if="errorDissallowed" class="text-danger mt-1">{{ errorDissallowed }}</div>
            <div class="form-hint">{{ $t('calculator.dissallowed-ip.placeholder') }}</div>
          </div>
          <div class="form-actions">
            <button class="btn btn-secondary" type="button" @click="addPrivateIPs">
              <i class="fa-solid fa-shield-halved me-1"></i>
              {{ $t('calculator.button-exclude-private') }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <div class="col-12 col-lg-6">
      <div class="card">
        <div class="card-header">
          <h3>{{ $t('calculator.headline-allowed-ip') }}</h3>
          <button class="btn btn-ghost btn-sm" @click="copyResult" :disabled="!newAllowedIp">
            <i class="fa-solid" :class="copyStatus ? 'fa-check' : 'fa-copy'"></i>
            {{ copyStatus ? $t('general.copied') : $t('general.copy') }}
          </button>
        </div>
        <div class="card-body">
          <textarea class="form-control" :value="newAllowedIp" rows="8" :placeholder="$t('calculator.new-allowed-ip.placeholder')" readonly style="font-family:var(--font-mono);font-size:0.8125rem;resize:vertical;"></textarea>
        </div>
      </div>
    </div>
  </div>
</template>