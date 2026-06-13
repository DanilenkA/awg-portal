<script setup>
import {computed, onMounted, ref} from "vue";
import { profileStore } from "@/stores/profile";
import { settingsStore } from "@/stores/settings";
import { authStore } from "../stores/auth";
import {notify} from "@kyvg/vue3-notification";
import { useI18n } from "vue-i18n";

const { locale } = useI18n()
const profile = profileStore()
const settings = settingsStore()
const auth = authStore()

const webBasePath = ref(WGPORTAL_BASE_PATH);
const currentLang = ref(locale.value)

const switchLanguage = (lang) => {
  if (locale.value !== lang) {
    localStorage.setItem('wgLang', lang);
    locale.value = lang;
    currentLang.value = lang;
  }
}

const availableLanguages = [
  { code: 'en', label: 'English' },
  { code: 'ru', label: 'Русский' },
  { code: 'de', label: 'Deutsch' },
  { code: 'fr', label: 'Français' },
  { code: 'pt', label: 'Português' },
  { code: 'uk', label: 'Українська' },
  { code: 'vi', label: 'Tiếng Việt' },
  { code: 'zh', label: '中文' },
  { code: 'es', label: 'Español' },
  { code: 'ja', label: '日本語' },
  { code: 'ko', label: '한국어' },
]

onMounted(async () => {
  await profile.LoadUser()
  await auth.LoadWebAuthnCredentials()
})

const selectedCredential = ref({})

function enableRename(credential) {
  credential.renameMode = true;
  credential.tempName = credential.Name; // Store the original name
}

function cancelRename(credential) {
  credential.renameMode = false;
  credential.tempName = null; // Discard changes
}

async function saveRename(credential) {
  try {
    await auth.RenameWebAuthnCredential({ ...credential, Name: credential.tempName });
    credential.Name = credential.tempName; // Update the name
    credential.renameMode = false;
  } catch (error) {
    console.error("Failed to rename credential:", error);
  }
}

const pwFormData = ref({
  OldPassword: '',
  Password: '',
  PasswordRepeat: '',
})

const passwordWeak = computed(() => {
  return pwFormData.value.Password && pwFormData.value.Password.length > 0 && pwFormData.value.Password.length < settings.Setting('MinPasswordLength')
})

const passwordChangeAllowed = computed(() => {
  return pwFormData.value.Password && pwFormData.value.Password.length >= settings.Setting('MinPasswordLength') &&
      pwFormData.value.Password === pwFormData.value.PasswordRepeat &&
      pwFormData.value.OldPassword && pwFormData.value.OldPassword.length > 0 && pwFormData.value.OldPassword !== pwFormData.value.Password;
})

const updatePassword = async () => {
  try {
    await profile.changePassword(pwFormData.value);

    pwFormData.value.OldPassword = '';
    pwFormData.value.Password = '';
    pwFormData.value.PasswordRepeat = '';
    notify({
      title: "Password changed!",
      text: "Your password has been changed successfully.",
      type: 'success',
    });
  } catch (e) {
    notify({
      title: "Failed to update password!",
      text: e.toString(),
      type: 'error',
    })
  }
}

</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ $t('settings.headline') }}</h1>
      <p>{{ $t('settings.abstract') }}</p>
    </div>
  </div>

  <!-- Language settings -->
  <div class="card" v-if="auth.IsAuthenticated">
    <div class="card-header">
      <h3><i class="fa-solid fa-globe me-2"></i>{{ $t('menu.lang') }}</h3>
    </div>
    <div class="card-body">
      <p class="text-muted-sm mb-3">{{ $t('settings.abstract') }}</p>
      <div class="d-flex flex-wrap gap-2">
        <button
          v-for="lang in availableLanguages"
          :key="lang.code"
          class="btn"
          :class="currentLang === lang.code ? 'btn-primary' : 'btn-secondary'"
          @click.prevent="switchLanguage(lang.code)"
        >
          <i v-if="currentLang === lang.code" class="fa-solid fa-check me-1"></i>
          {{ lang.label }}
        </button>
      </div>
    </div>
  </div>

  <!-- Password change -->
  <div class="card" v-if="profile.user.Source === 'db'">
    <div class="card-header">
      <h3><i class="fa-solid fa-lock me-2"></i>{{ $t('settings.password.headline') }}</h3>
    </div>
    <div class="card-body">
      <p class="text-muted-sm mb-4">{{ $t('settings.password.abstract') }}</p>

      <div class="form-group">
        <label class="form-label" for="oldpw">{{ $t('settings.password.current-label') }}</label>
        <input id="oldpw" v-model="pwFormData.OldPassword" class="form-control" :class="{ 'is-invalid': pwFormData.Password && !pwFormData.OldPassword }" type="password">
      </div>

      <div class="form-row">
        <div class="form-group">
          <label class="form-label" for="newpw">{{ $t('settings.password.new-label') }}</label>
          <input id="newpw" v-model="pwFormData.Password" class="form-control" :class="{ 'is-invalid': passwordWeak,  'is-valid': pwFormData.Password !== '' && !passwordWeak }" type="password">
          <div class="invalid-feedback" v-if="passwordWeak">{{ $t('settings.password.weak-label') }}</div>
        </div>
        <div class="form-group">
          <label class="form-label" for="confirmnewpw">{{ $t('settings.password.new-confirm-label') }}</label>
          <input id="confirmnewpw" v-model="pwFormData.PasswordRepeat" class="form-control" :class="{ 'is-invalid': pwFormData.PasswordRepeat !== ''&& pwFormData.Password !== pwFormData.PasswordRepeat,  'is-valid': pwFormData.PasswordRepeat !== '' && pwFormData.Password === pwFormData.PasswordRepeat && !passwordWeak }" type="password">
          <div class="invalid-feedback" v-if="pwFormData.PasswordRepeat !== ''&& pwFormData.Password !== pwFormData.PasswordRepeat">{{ $t('settings.password.invalid-confirm-label') }}</div>
        </div>
      </div>
      <div class="form-actions">
        <button class="btn btn-primary" :title="$t('settings.password.change-button-text')" @click.prevent="updatePassword" :disabled="profile.isFetching || !passwordChangeAllowed">
          <i class="fa-solid fa-floppy-disk me-1"></i> {{ $t('settings.password.change-button-text') }}
        </button>
      </div>
    </div>
  </div>

  <!-- WebAuthn -->
  <div class="card" v-if="settings.Setting('WebAuthnEnabled')">
    <div class="card-header">
      <h3><i class="fa-solid fa-fingerprint me-2"></i>{{ $t('settings.webauthn.headline') }}</h3>
      <button class="btn btn-primary btn-sm" :title="$t('settings.webauthn.button-register-text')" @click.prevent="auth.RegisterWebAuthn" :disabled="auth.isFetching">
        <i class="fa-solid fa-plus me-1"></i> {{ $t('settings.webauthn.button-register-title') }}
      </button>
    </div>
    <div class="card-body">
      <p class="text-muted-sm mb-4">{{ $t('settings.webauthn.abstract') }}</p>
      <p v-if="auth.IsWebAuthnEnabled" class="text-success"><i class="fa-solid fa-circle-check me-1"></i> {{ $t('settings.webauthn.active-description') }}</p>
      <p v-else class="text-muted"><i class="fa-solid fa-circle-info me-1"></i> {{ $t('settings.webauthn.inactive-description') }}</p>

      <div v-if="auth.WebAuthnCredentials.length > 0" class="mt-4">
        <h4>{{ $t('settings.webauthn.credentials-list') }}</h4>
        <table class="data-table mt-3">
          <thead>
          <tr>
            <th style="width: 50%;">{{ $t('settings.webauthn.table.name') }}</th>
            <th style="width: 20%;">{{ $t('settings.webauthn.table.created') }}</th>
            <th class="text-end" style="width: 30%;">{{ $t('settings.webauthn.table.actions') }}</th>
          </tr>
          </thead>
          <tbody>
          <tr v-for="credential in auth.webAuthnCredentials" :key="credential.ID">
            <td>
              <div v-if="credential.renameMode">
                <input v-model="credential.tempName" class="form-control" type="text">
              </div>
              <div v-else>
                <i class="fa-solid fa-key me-2 text-muted"></i>{{ credential.Name }}
              </div>
            </td>
            <td class="text-mono-sm">{{ credential.CreatedAt }}</td>
            <td>
              <div class="cell-actions" v-if="credential.renameMode">
                <button class="btn btn-primary btn-sm" :title="$t('settings.webauthn.button-save-text')" @click.prevent="saveRename(credential)" :disabled="auth.isFetching">
                  <i class="fa-solid fa-check"></i>
                </button>
                <button class="btn btn-ghost btn-icon" :title="$t('settings.webauthn.button-cancel-text')" @click.prevent="cancelRename(credential)">
                  <i class="fa-solid fa-xmark"></i>
                </button>
              </div>
              <div class="cell-actions" v-else>
                <button class="btn btn-ghost btn-icon" :title="$t('settings.webauthn.button-rename-text')" @click.prevent="enableRename(credential)">
                  <i class="fa-solid fa-pen"></i>
                </button>
                <button class="btn btn-ghost btn-icon text-danger" :title="$t('settings.webauthn.button-delete-text')" data-bs-toggle="modal" data-bs-target="#webAuthnDeleteModal" :disabled="auth.isFetching" @click="selectedCredential=credential">
                  <i class="fa-solid fa-trash"></i>
                </button>
              </div>
            </td>
          </tr>
          </tbody>
        </table>
      </div>

      <div class="modal fade" id="webAuthnDeleteModal" tabindex="-1" aria-labelledby="webAuthnDeleteModalLabel" aria-hidden="true">
        <div class="modal-dialog modal-dialog-centered">
          <div class="modal-content">
            <div class="modal-header" style="background: var(--danger-subtle);">
              <h5 class="modal-title text-danger" id="webAuthnDeleteModalLabel">{{ $t('settings.webauthn.modal-delete.headline') }}</h5>
              <button type="button" class="btn-close" data-bs-dismiss="modal" :aria-label="$t('settings.webauthn.modal-delete.button-cancel')"></button>
            </div>
            <div class="modal-body">
              <h5 class="mb-3">{{ selectedCredential.Name }} <small class="text-muted">({{ $t('settings.webauthn.modal-delete.created') }} {{ selectedCredential.CreatedAt }})</small></h5>
              <p class="mb-0">{{ $t('settings.webauthn.modal-delete.abstract') }}</p>
            </div>
            <div class="modal-footer">
              <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">{{ $t('settings.webauthn.modal-delete.button-cancel') }}</button>
              <button type="button" class="btn btn-danger" id="confirmWebAuthnDelete" @click="auth.DeleteWebAuthnCredential(selectedCredential.ID)" :disabled="auth.isFetching" data-bs-dismiss="modal">{{ $t('settings.webauthn.modal-delete.button-delete') }}</button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- API -->
  <div v-if="auth.IsAdmin || !settings.Setting('ApiAdminOnly')">
    <div class="card" v-if="profile.user.ApiToken">
      <div class="card-header">
        <h3><i class="fa-solid fa-code me-2"></i>{{ $t('settings.api.headline') }}</h3>
        <button class="btn btn-danger btn-sm" :title="$t('settings.api.button-disable-title')" @click.prevent="profile.disableApi()" :disabled="profile.isFetching">
          <i class="fa-solid fa-circle-xmark me-1"></i> {{ $t('settings.api.button-disable-text') }}
        </button>
      </div>
      <div class="card-body">
        <p class="text-muted-sm mb-4">{{ $t('settings.api.abstract') }}</p>
        <p>{{ $t('settings.api.active-description') }}</p>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">{{ $t('settings.api.user-label') }}</label>
            <input v-model="profile.user.Identifier" class="form-control" :placeholder="$t('settings.api.user-placeholder')" type="text" readonly>
          </div>
          <div class="form-group">
            <label class="form-label">{{ $t('settings.api.token-label') }}</label>
            <input v-model="profile.user.ApiToken" class="form-control text-mono-sm" :placeholder="$t('settings.api.token-placeholder')" type="text" readonly>
          </div>
        </div>
        <p class="text-muted-sm mt-2">{{ $t('settings.api.token-created-label') }} {{ profile.user.ApiTokenCreated }}</p>
        <a :href="webBasePath + '/api/v1/doc.html'" target="_blank" :alt="$t('settings.api.api-link')" class="btn btn-secondary btn-sm mt-3">
          <i class="fa-solid fa-book me-1"></i> {{ $t('settings.api.api-link') }}
        </a>
      </div>
    </div>
    <div class="card" v-else>
      <div class="card-header">
        <h3><i class="fa-solid fa-code me-2"></i>{{ $t('settings.api.headline') }}</h3>
      </div>
      <div class="card-body">
        <p class="text-muted-sm mb-4">{{ $t('settings.api.abstract') }}</p>
        <p>{{ $t('settings.api.inactive-description') }}</p>
        <button class="btn btn-primary" :title="$t('settings.api.button-enable-title')" @click.prevent="profile.enableApi()" :disabled="profile.isFetching">
          <i class="fa-solid fa-plus me-1"></i> {{ $t('settings.api.button-enable-text') }}
        </button>
      </div>
    </div>
  </div>
</template>