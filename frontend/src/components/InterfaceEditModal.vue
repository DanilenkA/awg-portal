<script setup>
import Modal from "./Modal.vue";
import {interfaceStore} from "@/stores/interfaces";
import {computed, ref, watch} from "vue";
import { useI18n } from 'vue-i18n';
import { notify } from "@kyvg/vue3-notification";
import { VueTagsInput } from '@vojtechlanka/vue-tags-input';
import { validateCIDR, validateIP, validateDomain } from '@/helpers/validators';
import isCidr from "is-cidr";
import {isIP} from 'is-ip';
import { freshInterface } from '@/helpers/models';
import {peerStore} from "@/stores/peers";
import {settingsStore} from "@/stores/settings";
import {securityStore} from "@/stores/security";

const { t } = useI18n()

const interfaces = interfaceStore()
const peers = peerStore()
const settings = settingsStore()
const sec = securityStore()

const props = defineProps({
  interfaceId: String,
  visible: Boolean,
})

const emit = defineEmits(['close'])

const selectedInterface = computed(() => {
  return interfaces.Find(props.interfaceId)
})

const title = computed(() => {
  if (!props.visible) {
    return "" // otherwise interfaces.GetSelected will die...
  }

  if (selectedInterface.value) {
    return t("modals.interface-edit.headline-edit") + " " + selectedInterface.value.Identifier
  }
  return t("modals.interface-edit.headline-new")
})

const currentTags = ref({
  Addresses: "",
  Dns: "",
  DnsSearch: "",
  PeerDefNetwork: "",
  PeerDefAllowedIPs: "",
  PeerDefDns: "",
  PeerDefDnsSearch: ""
})

// AWG parameter auto-generation (matches backend GenerateAWGParams)
function generateAWGParams() {
  const secureU32 = () => {
    const values = new Uint32Array(1)
    crypto.getRandomValues(values)
    return values[0]
  }
  const rand = (lo, hi) => {
    const range = hi - lo + 1
    const limit = 0x100000000 - (0x100000000 % range)
    let v
    do {
      v = secureU32()
    } while (v >= limit)
    return lo + (v % range)
  }
  const randU32 = () => { let v; do { v = secureU32() } while (v < 5); return v }
  
  let s1, s2, s3, s4
  for (;;) {
    s1 = rand(15, 63); s2 = rand(15, 63); s3 = rand(10, 63); s4 = rand(1, 15)
    const set = new Set([s1, s2, s3, s4])
    if (set.size !== 4) continue
    if (s1+148 === s2+92 || s3+64 === s1+148 || s3+64 === s2+92) continue
    break
  }
  
  const hs = new Set()
  const h = () => { let v; do { v = randU32() } while (hs.has(v)); hs.add(v); return v }
  
  const jmin = rand(64, 113)
  return {
    AWGJc: rand(3, 6),
    AWGJmin: jmin,
    AWGJmax: rand(jmin+50, jmin+149),
    AWGS1: s1, AWGS2: s2, AWGS3: s3, AWGS4: s4,
    AWGH1: h(), AWGH2: h(), AWGH3: h(), AWGH4: h()
  }
}

function toggleAWG() {
  formData.value.AWGEnabled = !formData.value.AWGEnabled
  if (formData.value.AWGEnabled) {
    const params = generateAWGParams()
    Object.assign(formData.value, params)
  } else {
    formData.value.AWGJc = 0
    formData.value.AWGJmin = 0
    formData.value.AWGJmax = 0
    formData.value.AWGS1 = 0; formData.value.AWGS2 = 0
    formData.value.AWGS3 = 0; formData.value.AWGS4 = 0
    formData.value.AWGH1 = 0; formData.value.AWGH2 = 0
    formData.value.AWGH3 = 0; formData.value.AWGH4 = 0
  }
}

// Bug 2 fix: surface a soft warning when amneziawg-go is not present on
// the host PATH. We rely on the AWGAvailable flag published by
// /config/settings (computed once on settings load) so the modal does
// not need a fresh backend round-trip every time it opens. The flag is
// undefined for older backends → we treat that as "available" to keep
// backward compatibility.
const awgAvailable = computed(() => {
  const v = settings.Setting('AWGAvailable')
  return v === undefined || v === null ? true : !!v
})
const formData = ref(freshInterface())
const isSaving = ref(false)
const isDeleting = ref(false)
const isApplyingDefaults = ref(false)
const isCreatingDefaultPeers = ref(false)

const isBackendValid = computed(() => {
  if (!props.visible || !selectedInterface.value) {
    return true // if modal is not visible or no interface is selected, we don't care about backend validity
  }

  let backendId = selectedInterface.value.Backend

  let valid = false
  let availableBackends = settings.Setting('AvailableBackends') || []
  availableBackends.forEach(backend => {
    if (backend.Id === backendId) {
      valid = true
    }
  })
  return valid
})

// functions

watch(() => props.visible, async (newValue, oldValue) => {
      if (oldValue === false && newValue === true) { // if modal is shown
        // Bug 3: refresh the CSRF token before any mutating call. The
        // server-side CSRF rotates on session changes, and a stale token
        // produces a 403 that the fetch-wrapper treats as an auth failure
        // and auto-logs-out the user. Loading it here (next to the modal
        // open) means the token is fresh for the upcoming save.
        await sec.LoadSecurityProperties()
        console.log(selectedInterface.value)
        if (!selectedInterface.value) {
          await interfaces.PrepareInterface()

          // fill form data
          formData.value.Identifier = interfaces.Prepared.Identifier
          formData.value.DisplayName = interfaces.Prepared.DisplayName
          formData.value.Mode = interfaces.Prepared.Mode
          formData.value.CreateDefaultPeer = interfaces.Prepared.CreateDefaultPeer
          formData.value.Backend = interfaces.Prepared.Backend

          formData.value.PublicKey = interfaces.Prepared.PublicKey
          formData.value.PrivateKey = interfaces.Prepared.PrivateKey

          formData.value.ListenPort = interfaces.Prepared.ListenPort
          formData.value.Addresses = interfaces.Prepared.Addresses
          formData.value.Dns = interfaces.Prepared.Dns
          formData.value.DnsSearch = interfaces.Prepared.DnsSearch

          formData.value.Mtu = interfaces.Prepared.Mtu
          formData.value.FirewallMark = interfaces.Prepared.FirewallMark
          formData.value.RoutingTable = interfaces.Prepared.RoutingTable

          formData.value.PreUp = interfaces.Prepared.PreUp
          formData.value.PostUp = interfaces.Prepared.PostUp
          formData.value.PreDown = interfaces.Prepared.PreDown
          formData.value.PostDown = interfaces.Prepared.PostDown

          formData.value.SaveConfig = interfaces.Prepared.SaveConfig

          // Reset AWG for new interface
          formData.value.AWGEnabled = false
          formData.value.AWGJc = 0
          formData.value.AWGJmin = 0
          formData.value.AWGJmax = 0
          formData.value.AWGS1 = 0
          formData.value.AWGS2 = 0
          formData.value.AWGS3 = 0
          formData.value.AWGS4 = 0
          formData.value.AWGH1 = 0
          formData.value.AWGH2 = 0
          formData.value.AWGH3 = 0
          formData.value.AWGH4 = 0

          formData.value.PeerDefNetwork = interfaces.Prepared.PeerDefNetwork
          formData.value.PeerDefDns = interfaces.Prepared.PeerDefDns
          formData.value.PeerDefDnsSearch = interfaces.Prepared.PeerDefDnsSearch
          formData.value.PeerDefEndpoint = interfaces.Prepared.PeerDefEndpoint
          formData.value.PeerDefAllowedIPs = interfaces.Prepared.PeerDefAllowedIPs
          formData.value.PeerDefMtu = interfaces.Prepared.PeerDefMtu
          formData.value.PeerDefPersistentKeepalive = interfaces.Prepared.PeerDefPersistentKeepalive
          formData.value.PeerDefFirewallMark = interfaces.Prepared.PeerDefFirewallMark
          formData.value.PeerDefRoutingTable = interfaces.Prepared.PeerDefRoutingTable
          formData.value.PeerDefPreUp = interfaces.Prepared.PeerDefPreUp
          formData.value.PeerDefPostUp = interfaces.Prepared.PeerDefPostUp
          formData.value.PeerDefPreDown = interfaces.Prepared.PeerDefPreDown
          formData.value.PeerDefPostDown = interfaces.Prepared.PeerDefPostDown
        } else { // fill existing userdata
          formData.value.Disabled = selectedInterface.value.Disabled
          formData.value.Identifier = selectedInterface.value.Identifier
          formData.value.DisplayName = selectedInterface.value.DisplayName
          formData.value.Mode = selectedInterface.value.Mode
          formData.value.CreateDefaultPeer = selectedInterface.value.CreateDefaultPeer
          formData.value.Backend = selectedInterface.value.Backend

          formData.value.PublicKey = selectedInterface.value.PublicKey
          formData.value.PrivateKey = selectedInterface.value.PrivateKey

          formData.value.ListenPort = selectedInterface.value.ListenPort
          formData.value.Addresses = selectedInterface.value.Addresses
          formData.value.Dns = selectedInterface.value.Dns
          formData.value.DnsSearch = selectedInterface.value.DnsSearch

          formData.value.Mtu = selectedInterface.value.Mtu
          formData.value.FirewallMark = selectedInterface.value.FirewallMark
          formData.value.RoutingTable = selectedInterface.value.RoutingTable

          formData.value.PreUp = selectedInterface.value.PreUp
          formData.value.PostUp = selectedInterface.value.PostUp
          formData.value.PreDown = selectedInterface.value.PreDown
          formData.value.PostDown = selectedInterface.value.PostDown

          formData.value.SaveConfig = selectedInterface.value.SaveConfig

          formData.value.PeerDefNetwork = selectedInterface.value.PeerDefNetwork
          formData.value.PeerDefDns = selectedInterface.value.PeerDefDns
          formData.value.PeerDefDnsSearch = selectedInterface.value.PeerDefDnsSearch
          formData.value.PeerDefEndpoint = selectedInterface.value.PeerDefEndpoint
          formData.value.PeerDefAllowedIPs = selectedInterface.value.PeerDefAllowedIPs
          formData.value.PeerDefMtu = selectedInterface.value.PeerDefMtu
          formData.value.PeerDefPersistentKeepalive = selectedInterface.value.PeerDefPersistentKeepalive
          formData.value.PeerDefFirewallMark = selectedInterface.value.PeerDefFirewallMark
          formData.value.PeerDefRoutingTable = selectedInterface.value.PeerDefRoutingTable
          formData.value.PeerDefPreUp = selectedInterface.value.PeerDefPreUp
          formData.value.PeerDefPostUp = selectedInterface.value.PeerDefPostUp
          formData.value.PeerDefPreDown = selectedInterface.value.PeerDefPreDown
          formData.value.PeerDefPostDown = selectedInterface.value.PeerDefPostDown

          // AWG fields
          formData.value.AWGEnabled = selectedInterface.value.AWGEnabled
          formData.value.AWGJc = selectedInterface.value.AWGJc
          formData.value.AWGJmin = selectedInterface.value.AWGJmin
          formData.value.AWGJmax = selectedInterface.value.AWGJmax
          formData.value.AWGS1 = selectedInterface.value.AWGS1
          formData.value.AWGS2 = selectedInterface.value.AWGS2
          formData.value.AWGS3 = selectedInterface.value.AWGS3
          formData.value.AWGS4 = selectedInterface.value.AWGS4
          formData.value.AWGH1 = selectedInterface.value.AWGH1
          formData.value.AWGH2 = selectedInterface.value.AWGH2
          formData.value.AWGH3 = selectedInterface.value.AWGH3
          formData.value.AWGH4 = selectedInterface.value.AWGH4
        }
      }
    }
)

function close() {
  formData.value = freshInterface()
  emit('close')
}

function handleChangeAddresses(tags) {
  let validInput = true
  tags.forEach(tag => {
    if(isCidr(tag.text) === 0) {
      validInput = false
      notify({
        title: "Invalid CIDR",
        text: tag.text + " is not a valid IP address",
        type: 'error',
      })
    }
  })
  if(validInput) {
    formData.value.Addresses = tags.map(tag => tag.text)
  }
}

function handleChangeDns(tags) {
  let validInput = true
  tags.forEach(tag => {
    if(!isIP(tag.text)) {
      validInput = false
      notify({
        title: "Invalid IP",
        text: tag.text + " is not a valid IP address",
        type: 'error',
      })
    }
  })
  if(validInput) {
    formData.value.Dns = tags.map(tag => tag.text)
  }
}

function handleChangeDnsSearch(tags) {
  formData.value.DnsSearch = tags.map(tag => tag.text)
}

function handleChangePeerDefNetwork(tags) {
  let validInput = true
  tags.forEach(tag => {
    if(isCidr(tag.text) === 0) {
      validInput = false
      notify({
        title: "Invalid CIDR",
        text: tag.text + " is not a valid IP address",
        type: 'error',
      })
    }
  })
  if(validInput) {
    formData.value.PeerDefNetwork = tags.map(tag => tag.text)
  }
}

function handleChangePeerDefAllowedIPs(tags) {
  let validInput = true
  tags.forEach(tag => {
    if(isCidr(tag.text) === 0) {
      validInput = false
      notify({
        title: "Invalid CIDR",
        text: tag.text + " is not a valid IP address",
        type: 'error',
      })
    }
  })
  if(validInput) {
    formData.value.PeerDefAllowedIPs = tags.map(tag => tag.text)
  }
}

function handleChangePeerDefDns(tags) {
  let validInput = true
  tags.forEach(tag => {
    if(!isIP(tag.text)) {
      validInput = false
      notify({
        title: "Invalid IP",
        text: tag.text + " is not a valid IP address",
        type: 'error',
      })
    }
  })
  if(validInput) {
    formData.value.PeerDefDns = tags.map(tag => tag.text)
  }
}

function handleChangePeerDefDnsSearch(tags) {
  formData.value.PeerDefDnsSearch = tags.map(tag => tag.text)
}

async function saveInterface() {
  if (isSaving.value) return
  // Bug 2 fix: short-circuit the create/update path when the user has
  // enabled AmneziaWG obfuscation but the backend reports the binary
  // is missing. We still let the request go through if AWGAvailable is
  // undefined (older backend) — better to surface a backend error than
  // to silently drop the user's intent.
  if (formData.value.AWGEnabled && !awgAvailable.value) {
    notify({
      title: t('modals.interface-edit.awg.binary-missing-title'),
      text: t('modals.interface-edit.awg.binary-missing'),
      type: 'error',
    })
    return
  }
  const savedId = formData.value.Identifier
  isSaving.value = true
  try {
    // Bug 3: visibility for the save path
    console.log("[InterfaceEditModal] saveInterface called", {
      isNew: props.interfaceId === '#NEW#',
      identifier: savedId,
      mode: formData.value.Mode,
      backend: formData.value.Backend,
      addresses: formData.value.Addresses,
      listenPort: formData.value.ListenPort,
      awgEnabled: formData.value.AWGEnabled,
    })
    if (props.interfaceId!=='#NEW#') {
      await interfaces.UpdateInterface(selectedInterface.value.Identifier, formData.value)
    } else {
      await interfaces.CreateInterface(formData.value)
    }
    await interfaces.LoadInterfaces()
    if (savedId && interfaces.Find(savedId)) {
      interfaces.selected = savedId
    }
    notify({
      title: "Interface saved",
      text: "The interface has been saved successfully.",
      type: 'success',
    })
    close()
  } catch (e) {
    console.log("[InterfaceEditModal] saveInterface failed", e)
    const raw = (e && e.message) ? e.message : (e && e.toString) ? e.toString() : String(e)
    const isPerm = /operation not permitted|EPERM|CAP_NET_ADMIN/i.test(raw)
    // Bug 2 fix: also translate the amneziawg-go missing error in case
    // the backend ever returns it (the backend writeCreateError now
    // maps this to 400 with a friendly message, so a 4xx here
    // typically means the operator ran past the warning).
    const isAWGMissing = /amneziawg-go not found|executable file not found/i.test(raw)
    notify({
      title: isPerm
        ? t('errors.perm_denied_title')
        : isAWGMissing
          ? t('modals.interface-edit.awg.binary-missing-title')
          : "Failed to save interface!",
      text: isPerm
        ? t('errors.perm_denied')
        : isAWGMissing
          ? t('modals.interface-edit.awg.binary-missing')
          : raw,
      type: 'error',
    })
  } finally {
    isSaving.value = false
  }
}

async function applyPeerDefaults() {
  if (props.interfaceId==='#NEW#') {
    return; // do nothing for new interfaces
  }

  if (isApplyingDefaults.value) return
  isApplyingDefaults.value = true
  try {
    await interfaces.ApplyPeerDefaults(selectedInterface.value.Identifier, formData.value)

    notify({
      title: "Peer Defaults Applied",
      text: "Applied current peer defaults to all available peers.",
      type: 'success',
    })

    await peers.LoadPeers(selectedInterface.value.Identifier) // reload all peers after applying the defaults
  } catch (e) {
    console.log(e)
    notify({
      title: "Failed to apply peer defaults!",
      text: e.toString(),
      type: 'error',
    })
  } finally {
    isApplyingDefaults.value = false
  }
}

async function createDefaultPeers() {
  if (props.interfaceId==='#NEW#') {
    return; // do nothing for new interfaces
  }

  if (!formData.value.CreateDefaultPeer) {
    return; // only allowed if the interface flag is set
  }

  if (isCreatingDefaultPeers.value) return
  isCreatingDefaultPeers.value = true
  try {
    await interfaces.CreateDefaultPeers(selectedInterface.value.Identifier)

    notify({
      title: "Default Peers Created",
      text: "Created default peers for all users on this interface.",
      type: 'success',
    })

    await peers.LoadPeers(selectedInterface.value.Identifier) // reload peers list
  } catch (e) {
    console.log(e)
    notify({
      title: "Failed to create default peers!",
      text: e.toString(),
      type: 'error',
    })
  } finally {
    isCreatingDefaultPeers.value = false
  }
}

async function del() {
  if (isDeleting.value) return
  if (!confirm(t('modals.interface-edit.confirm-delete', {id: selectedInterface.value.Identifier}))) return
  isDeleting.value = true
  try {
    await interfaces.DeleteInterface(selectedInterface.value.Identifier)

    // reload all interfaces and peers
    await interfaces.LoadInterfaces()
    if (interfaces.Count > 0 && interfaces.GetSelected !== undefined) {
      const selectedInterface = interfaces.GetSelected
      await peers.LoadPeers(selectedInterface.Identifier)
      await peers.LoadStats(selectedInterface.Identifier)
    } else {
      await peers.Reset() // reset peers if no interfaces are available
    }
    close()
  } catch (e) {
    console.log(e)
    notify({
      title: "Failed to delete interface!",
      text: e.toString(),
      type: 'error',
    })
  } finally {
    isDeleting.value = false
  }
}

</script>

<template>
  <Modal :title="title" :visible="visible" @close="close">
    <template #default>
      <form id="interface-edit-form" class="interface-edit-form" @submit.prevent="saveInterface">
        <ul class="nav nav-tabs">
          <li class="nav-item">
            <a class="nav-link active" data-bs-toggle="tab" href="#interface">{{ $t('modals.interface-edit.tab-interface') }}</a>
          </li>
          <li v-if="formData.Mode==='server'" class="nav-item">
            <a class="nav-link" data-bs-toggle="tab" href="#peerdefaults">{{ $t('modals.interface-edit.tab-peerdef') }}</a>
          </li>
        </ul>
        <div id="interfaceTabs" class="tab-content">
        <div id="interface" class="tab-pane fade active show">
          <fieldset class="form-section interface-edit-section">
            <legend class="form-section-title">{{ $t('modals.interface-edit.header-general') }}</legend>
            <div v-if="props.interfaceId==='#NEW#'" class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.identifier.label') }}</label>
                <input v-model="formData.Identifier" class="form-input" :placeholder="$t('modals.interface-edit.identifier.placeholder')" type="text">
              </div>
            </div>
            <div class="form-row form-row--double">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.mode.label') }}</label>
                <select v-model="formData.Mode" class="form-input">
                  <option value="server">{{ $t('modals.interface-edit.mode.server') }}</option>
                  <option value="client">{{ $t('modals.interface-edit.mode.client') }}</option>
                  <option value="any">{{ $t('modals.interface-edit.mode.any') }}</option>
                </select>
              </div>
              <div class="form-group">
                <label class="form-label" for="ifaceBackendSelector">{{ $t('modals.interface-edit.backend.label') }}</label>
                <select id="ifaceBackendSelector" v-model="formData.Backend" class="form-input" aria-describedby="backendHelp">
                  <option v-for="backend in settings.Setting('AvailableBackends')" :value="backend.Id">{{ backend.Id === 'local' ? $t(backend.Name) : backend.Name }}</option>
                </select>
                <small v-if="!isBackendValid" id="backendHelp" class="form-hint form-hint--warning">{{ $t('modals.interface-edit.backend.invalid-label') }}</small>
              </div>
            </div>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.display-name.label') }}</label>
                <input v-model="formData.DisplayName" class="form-input" :placeholder="$t('modals.interface-edit.display-name.placeholder')" type="text">
              </div>
            </div>
          </fieldset>
          <fieldset class="form-section interface-edit-section">
            <legend class="form-section-title">{{ $t('modals.interface-edit.header-crypto') }}</legend>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.private-key.label') }}</label>
                <input v-model="formData.PrivateKey" class="form-input code" :placeholder="$t('modals.interface-edit.private-key.placeholder')" required type="text">
              </div>
            </div>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.public-key.label') }}</label>
                <input v-model="formData.PublicKey" class="form-input code" :placeholder="$t('modals.interface-edit.public-key.placeholder')" required type="text">
              </div>
            </div>
          </fieldset>
          <fieldset class="form-section interface-edit-section">
            <legend class="form-section-title">Protocol</legend>
            <div v-if="!awgAvailable" class="form-hint form-hint--warning awg-binary-warning" role="alert">
              <i class="fas fa-exclamation-triangle" aria-hidden="true"></i>
              {{ t('modals.interface-edit.awg.binary-missing') }}
            </div>
            <div class="form-check form-switch awg-toggle">
              <input :checked="formData.AWGEnabled" :disabled="!awgAvailable" class="form-check-input" type="checkbox" id="awgToggle" @change="toggleAWG">
              <label class="form-check-label" for="awgToggle">
                AmneziaWG (обфускация DPI)
              </label>
            </div>
            <div v-if="formData.AWGEnabled" class="awg-params">
              <p class="form-hint awg-params__hint">Параметры обфускации сгенерированы автоматически.</p>
              <div class="form-row form-row--triple">
                <div class="form-group">
                  <label class="form-label" for="awgJc">Jc (Junk count)</label>
                  <input id="awgJc" v-model="formData.AWGJc" class="form-input" type="number" min="0" max="128">
                </div>
                <div class="form-group">
                  <label class="form-label" for="awgJmin">Jmin</label>
                  <input id="awgJmin" v-model="formData.AWGJmin" class="form-input" type="number" min="0" max="1280">
                </div>
                <div class="form-group">
                  <label class="form-label" for="awgJmax">Jmax</label>
                  <input id="awgJmax" v-model="formData.AWGJmax" class="form-input" type="number" min="0" max="1280">
                </div>
              </div>
              <div class="form-row">
                <div class="form-group">
                  <label class="form-label" for="awgS1">S1</label>
                  <input id="awgS1" v-model="formData.AWGS1" class="form-input" type="number" min="0" max="1280">
                </div>
                <div class="form-group">
                  <label class="form-label" for="awgS2">S2</label>
                  <input id="awgS2" v-model="formData.AWGS2" class="form-input" type="number" min="0" max="1280">
                </div>
              </div>
              <div class="form-row">
                <div class="form-group">
                  <label class="form-label" for="awgS3">S3</label>
                  <input id="awgS3" v-model="formData.AWGS3" class="form-input" type="number" min="0" max="1280">
                </div>
                <div class="form-group">
                  <label class="form-label" for="awgS4">S4</label>
                  <input id="awgS4" v-model="formData.AWGS4" class="form-input" type="number" min="0" max="1280">
                </div>
              </div>
              <div class="form-row">
                <div class="form-group">
                  <label class="form-label" for="awgH1">H1</label>
                  <input id="awgH1" v-model="formData.AWGH1" class="form-input" type="number" min="5">
                </div>
                <div class="form-group">
                  <label class="form-label" for="awgH2">H2</label>
                  <input id="awgH2" v-model="formData.AWGH2" class="form-input" type="number" min="5">
                </div>
              </div>
              <div class="form-row">
                <div class="form-group">
                  <label class="form-label" for="awgH3">H3</label>
                  <input id="awgH3" v-model="formData.AWGH3" class="form-input" type="number" min="5">
                </div>
                <div class="form-group">
                  <label class="form-label" for="awgH4">H4</label>
                  <input id="awgH4" v-model="formData.AWGH4" class="form-input" type="number" min="5">
                </div>
              </div>
            </div>
          </fieldset>
          <fieldset class="form-section interface-edit-section">
            <legend class="form-section-title">{{ $t('modals.interface-edit.header-network') }}</legend>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.ip.label') }}</label>
                <vue-tags-input class="tags-input" v-model="currentTags.Addresses"
                              :tags="formData.Addresses.map(str => ({ text: str }))"
                              :placeholder="$t('modals.interface-edit.ip.placeholder')"
                              :validation="validateCIDR()"
                              :add-on-key="[13, 188, 32, 9]"
                              :save-on-key="[13, 188, 32, 9]"
                              :allow-edit-tags="true"
                              :separators="[',', ';', ' ']"
                              @tags-changed="handleChangeAddresses"/>
              </div>
            </div>
            <div v-if="formData.Mode!=='server'" class="form-row">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.dns.label') }}</label>
                <vue-tags-input class="tags-input" v-model="currentTags.Dns"
                              :tags="formData.Dns.map(str => ({ text: str }))"
                              :placeholder="$t('modals.interface-edit.dns.placeholder')"
                              :validation="validateIP()"
                              :add-on-key="[13, 188, 32, 9]"
                              :save-on-key="[13, 188, 32, 9]"
                              :allow-edit-tags="true"
                              :separators="[',', ';', ' ']"
                              @tags-changed="handleChangeDns"/>
              </div>
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.dns-search.label') }}</label>
                <vue-tags-input class="tags-input" v-model="currentTags.DnsSearch"
                              :tags="formData.DnsSearch.map(str => ({ text: str }))"
                              :placeholder="$t('modals.interface-edit.dns-search.placeholder')"
                              :validation="validateDomain()"
                              :add-on-key="[13, 188, 32, 9]"
                              :save-on-key="[13, 188, 32, 9]"
                              :allow-edit-tags="true"
                              :separators="[',', ';', ' ']"
                              @tags-changed="handleChangeDnsSearch"/>
              </div>
            </div>
            <div class="form-row">
              <div v-if="formData.Mode==='server'" class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.listen-port.label') }}</label>
                <input v-model="formData.ListenPort" class="form-input" :placeholder="$t('modals.interface-edit.listen-port.placeholder')" type="number">
              </div>
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.mtu.label') }}</label>
                <input v-model="formData.Mtu" class="form-input" :placeholder="$t('modals.interface-edit.mtu.placeholder')" type="number">
              </div>
            </div>
            <div class="form-row" v-if="formData.Backend==='local'">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.firewall-mark.label') }}</label>
                <input v-model="formData.FirewallMark" class="form-input" :placeholder="$t('modals.interface-edit.firewall-mark.placeholder')" type="number">
              </div>
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.routing-table.label') }}</label>
                <input v-model="formData.RoutingTable" aria-describedby="routingTableHelp" class="form-input" :placeholder="$t('modals.interface-edit.routing-table.placeholder')" type="text">
                <small id="routingTableHelp" class="form-hint">{{ $t('modals.interface-edit.routing-table.description') }}</small>
              </div>
            </div>
            <div class="form-row form-row--single" v-else>
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.routing-table.label') }}</label>
                <input v-model="formData.RoutingTable" aria-describedby="routingTableHelp" class="form-input" :placeholder="$t('modals.interface-edit.routing-table.placeholder')" type="text">
                <small id="routingTableHelp" class="form-hint">{{ $t('modals.interface-edit.routing-table.description') }}</small>
              </div>
            </div>
          </fieldset>
          <fieldset v-if="formData.Backend==='local'" class="form-section interface-edit-section">
            <legend class="form-section-title">{{ $t('modals.interface-edit.header-hooks') }}</legend>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.pre-up.label') }}</label>
                <textarea v-model="formData.PreUp" class="form-input form-textarea" rows="2" :placeholder="$t('modals.interface-edit.pre-up.placeholder')"></textarea>
              </div>
            </div>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.post-up.label') }}</label>
                <textarea v-model="formData.PostUp" class="form-input form-textarea" rows="2" :placeholder="$t('modals.interface-edit.post-up.placeholder')"></textarea>
              </div>
            </div>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.pre-down.label') }}</label>
                <textarea v-model="formData.PreDown" class="form-input form-textarea" rows="2" :placeholder="$t('modals.interface-edit.pre-down.placeholder')"></textarea>
              </div>
            </div>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.post-down.label') }}</label>
                <textarea v-model="formData.PostDown" class="form-input form-textarea" rows="2" :placeholder="$t('modals.interface-edit.post-down.placeholder')"></textarea>
              </div>
            </div>
          </fieldset>
          <fieldset class="form-section interface-edit-section">
            <legend class="form-section-title">{{ $t('modals.interface-edit.header-state') }}</legend>
            <div class="form-row form-row--single">
              <div class="form-group form-check form-switch">
                <input v-model="formData.Disabled" class="form-check-input" type="checkbox">
                <label class="form-check-label">{{ $t('modals.interface-edit.disabled.label') }}</label>
              </div>
            </div>
            <div class="form-row form-row--single" v-if="formData.Mode==='server' && settings.Setting('CreateDefaultPeer')">
              <div class="form-group d-flex align-items-center justify-content-between" style="gap:var(--space-3);">
                <div class="form-check form-switch" style="margin:0;">
                  <input v-model="formData.CreateDefaultPeer" class="form-check-input" type="checkbox">
                  <label class="form-check-label">{{ $t('modals.interface-edit.create-default-peer.label') }}</label>
                </div>
                <button v-if="props.interfaceId!=='#NEW#'" class="btn btn-primary btn-sm" type="button" @click.prevent="createDefaultPeers" :disabled="!formData.CreateDefaultPeer || isCreatingDefaultPeers">
                  <span v-if="isCreatingDefaultPeers" class="spinner-border spinner-border-sm me-1" role="status" aria-hidden="true"></span>
                  {{ $t('modals.interface-edit.button-create-default-peers') }}
                </button>
              </div>
            </div>
            <div class="form-row form-row--single" v-if="formData.Backend==='local'">
              <div class="form-group form-check form-switch">
                <input v-model="formData.SaveConfig" checked="" class="form-check-input" type="checkbox">
                <label class="form-check-label">{{ $t('modals.interface-edit.save-config.label') }}</label>
              </div>
            </div>
          </fieldset>
        </div>
        <div id="peerdefaults" class="tab-pane fade">
          <fieldset class="form-section interface-edit-section">
            <legend class="form-section-title">{{ $t('modals.interface-edit.header-network') }}</legend>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.defaults.endpoint.label') }}</label>
                <input v-model="formData.PeerDefEndpoint" class="form-input" :placeholder="$t('modals.interface-edit.defaults.endpoint.placeholder')" type="text">
                <small class="form-hint">{{ $t('modals.interface-edit.defaults.endpoint.description') }}</small>
              </div>
            </div>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.defaults.networks.label') }}</label>
                <vue-tags-input class="tags-input" v-model="currentTags.PeerDefNetwork"
                              :tags="formData.PeerDefNetwork.map(str => ({ text: str }))"
                              :placeholder="$t('modals.interface-edit.defaults.networks.placeholder')"
                              :validation="validateCIDR()"
                              :add-on-key="[13, 188, 32, 9]"
                              :save-on-key="[13, 188, 32, 9]"
                              :allow-edit-tags="true"
                              :separators="[',', ';', ' ']"
                              @tags-changed="handleChangePeerDefNetwork"/>
                <small class="form-hint">{{ $t('modals.interface-edit.defaults.networks.description') }}</small>
              </div>
            </div>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.defaults.allowed-ip.label') }}</label>
                <vue-tags-input class="tags-input" v-model="currentTags.PeerDefAllowedIPs"
                              :tags="formData.PeerDefAllowedIPs.map(str => ({ text: str }))"
                              :placeholder="$t('modals.interface-edit.defaults.allowed-ip.placeholder')"
                              :validation="validateCIDR()"
                              :add-on-key="[13, 188, 32, 9]"
                              :save-on-key="[13, 188, 32, 9]"
                              :allow-edit-tags="true"
                              :separators="[',', ';', ' ']"
                              @tags-changed="handleChangePeerDefAllowedIPs"/>
              </div>
            </div>
            <div class="form-row">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.dns.label') }}</label>
                <vue-tags-input class="tags-input" v-model="currentTags.PeerDefDns"
                              :tags="formData.PeerDefDns.map(str => ({ text: str }))"
                              :placeholder="$t('modals.interface-edit.dns.placeholder')"
                              :validation="validateIP()"
                              :add-on-key="[13, 188, 32, 9]"
                              :save-on-key="[13, 188, 32, 9]"
                              :allow-edit-tags="true"
                              :separators="[',', ';', ' ']"
                              @tags-changed="handleChangePeerDefDns"/>
              </div>
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.dns-search.label') }}</label>
                <vue-tags-input class="tags-input" v-model="currentTags.PeerDefDnsSearch"
                              :tags="formData.PeerDefDnsSearch.map(str => ({ text: str }))"
                              :placeholder="$t('modals.interface-edit.dns-search.placeholder')"
                              :validation="validateDomain()"
                              :add-on-key="[13, 188, 32, 9]"
                              :save-on-key="[13, 188, 32, 9]"
                              :allow-edit-tags="true"
                              :separators="[',', ';', ' ']"
                              @tags-changed="handleChangePeerDefDnsSearch"/>
              </div>
            </div>
            <div class="form-row">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.defaults.mtu.label') }}</label>
                <input v-model="formData.PeerDefMtu" class="form-input" :placeholder="$t('modals.interface-edit.defaults.mtu.placeholder')" type="number">
              </div>
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.firewall-mark.label') }}</label>
                <input v-model="formData.PeerDefFirewallMark" class="form-input" :placeholder="$t('modals.interface-edit.firewall-mark.placeholder')" type="number">
              </div>
            </div>
            <div class="form-row">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.routing-table.label') }}</label>
                <input v-model="formData.PeerDefRoutingTable" class="form-input" :placeholder="$t('modals.interface-edit.routing-table.placeholder')" type="number">
              </div>
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.defaults.keep-alive.label') }}</label>
                <input v-model="formData.PeerDefPersistentKeepalive" class="form-input" :placeholder="$t('modals.interface-edit.defaults.keep-alive.placeholder')" type="number">
              </div>
            </div>
          </fieldset>
          <fieldset class="form-section interface-edit-section">
            <legend class="form-section-title">{{ $t('modals.interface-edit.header-peer-hooks') }}</legend>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.pre-up.label') }}</label>
                <textarea v-model="formData.PeerDefPreUp" class="form-input form-textarea" rows="2" :placeholder="$t('modals.interface-edit.pre-up.placeholder')"></textarea>
              </div>
            </div>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.post-up.label') }}</label>
                <textarea v-model="formData.PeerDefPostUp" class="form-input form-textarea" rows="2" :placeholder="$t('modals.interface-edit.post-up.placeholder')"></textarea>
              </div>
            </div>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.pre-down.label') }}</label>
                <textarea v-model="formData.PeerDefPreDown" class="form-input form-textarea" rows="2" :placeholder="$t('modals.interface-edit.pre-down.placeholder')"></textarea>
              </div>
            </div>
            <div class="form-row form-row--single">
              <div class="form-group">
                <label class="form-label">{{ $t('modals.interface-edit.post-down.label') }}</label>
                <textarea v-model="formData.PeerDefPostDown" class="form-input form-textarea" rows="2" :placeholder="$t('modals.interface-edit.post-down.placeholder')"></textarea>
              </div>
            </div>
          </fieldset>
          <fieldset v-if="props.interfaceId!=='#NEW#'" class="text-end">
            <hr>
            <button class="btn btn-primary me-1" type="button" @click.prevent="applyPeerDefaults" :disabled="isApplyingDefaults">
              <span v-if="isApplyingDefaults" class="spinner-border spinner-border-sm me-1" role="status" aria-hidden="true"></span>
              {{ $t('modals.interface-edit.button-apply-defaults') }}
            </button>
          </fieldset>
        </div>
        </div>
      </form>
    </template>
    <template #footer>
      <div class="flex-fill text-start">
        <button v-if="props.interfaceId!=='#NEW#'" class="btn btn-danger me-1" type="button" @click.prevent="del" :disabled="isDeleting">
          <span v-if="isDeleting" class="spinner-border spinner-border-sm me-1" role="status" aria-hidden="true"></span>
          {{ $t('general.delete') }}
        </button>
      </div>
      <button class="btn btn-primary me-1" type="button" @click.prevent="saveInterface" :disabled="isSaving">
        <span v-if="isSaving" class="spinner-border spinner-border-sm me-1" role="status" aria-hidden="true"></span>
        {{ $t('general.save') }}
      </button>
      <button class="btn btn-secondary" type="button" @click.prevent="close">{{ $t('general.close') }}</button>
    </template>
  </Modal>
</template>

<style>

</style>
