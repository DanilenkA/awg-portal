<script setup>

import {computed, onMounted, ref, nextTick, watch} from "vue";
import {useRoute} from "vue-router";
import {useI18n} from "vue-i18n";
import {authStore} from "@/stores/auth";
import {notify} from "@kyvg/vue3-notification";
import {settingsStore} from "@/stores/settings";

const { t } = useI18n()

const auth = authStore()
const settings = settingsStore()
const route = useRoute()

const loggingIn = ref(false)
const username = ref("")
const password = ref("")
const passwordRef = ref(null)

// Bug 3 fix: track failed-login attempts so we can clear the password
// field's "is-valid" green-check styling. Bootstrap treats a non-empty
// field with no .is-invalid class as valid, so after a 401 the password
// input would otherwise stay green. We force a re-key on the input via
// :key="passwordInputKey" to drop the stale validity state, then focus
// and pre-fill the (now-empty) password field.
const passwordInputKey = ref(0)
const loginFailed = ref(false)

// Reset the failed state as soon as the user starts typing again so the
// red .is-invalid highlight clears and the form looks normal while the
// user is composing a new attempt.
watch(password, (v) => { if (v && loginFailed.value) loginFailed.value = false })
watch(username, (v) => { if (v && loginFailed.value) loginFailed.value = false })

const usernameInvalid = computed(() => username.value === "" && loginFailed.value)
const passwordInvalid = computed(() => password.value === "" && loginFailed.value)
// Only show the green "is-valid" check BEFORE the first failed attempt —
// after a 401 the field must reset to neutral until the user types again.
const passwordValid = computed(() => !passwordInvalid.value && password.value !== "" && !loginFailed.value)
const usernameValid = computed(() => !usernameInvalid.value && username.value !== "" && !loginFailed.value)
const disableLoginBtn = computed(() => username.value === "" || password.value === "" || loggingIn.value)
onMounted(async () => {
  await settings.LoadSettings()
})

const postLoginRoute = () => {
  const target = auth.ReturnUrl || '/'
  return !target || target === '/login' ? '/' : target
}

const finishLogin = async function (loginAction) {
  const target = postLoginRoute()
  loggingIn.value = true
  try {
    await loginAction()
    loginFailed.value = false
    notify({
      title: "Logged in",
      text: "Authentication succeeded!",
      type: 'success',
    });
    await settings.LoadSettings(); // reload full settings before rendering authenticated layout
    // Full-page reload to /: in hash mode router.replace has a race with
    // App.vue's `routerViewKey` recompute (auth.IsAuthenticated flips BEFORE
    // the LoginView is unmounted, which can leave the auth-layout active and
    // render an empty <main>). A hard navigation gives Vue a clean mount
    // cycle and App.vue's onMounted → LoadSession branch runs again with
    // the valid session cookie.
    const targetHash = target.startsWith('/') ? `#${target}` : target
    const targetUrl = `${window.location.origin}${window.location.pathname}${window.location.search}${targetHash}`
    window.location.assign(targetUrl)
  } catch (error) {
    // Bug 3 fix: drop the password field's "valid" green check by clearing
    // the value, bumping the input :key (forces Vue to remount the input
    // element with a clean validity state), and re-focusing it so the user
    // can immediately re-type. The form-input keeps .is-invalid so the
    // field is highlighted red until the user types again.
    loginFailed.value = true
    password.value = ""
    passwordInputKey.value++
    await nextTick()
    if (passwordRef.value && passwordRef.value.focus) {
      passwordRef.value.focus()
    }
    notify({
      title: "Login failed!",
      text: t('login.invalid_credentials'),
      type: 'error',
    });

    // delay the user from logging in for a short amount of time
    setTimeout(() => loggingIn.value = false, 1000);
    return
  }

  loggingIn.value = false;
}

const login = async function () {
  console.log("Performing login for user:", username.value);
  await finishLogin(() => auth.Login(username.value, password.value))
}

const loginWebAuthn = async function () {
  console.log("Performing webauthn login");
  await finishLogin(() => auth.LoginWebAuthn())
}

const externalLogin = function (provider) {
  console.log("Performing external login for provider", provider.Identifier);
  loggingIn.value = true;
  const currentUrl = new URL(`${WGPORTAL_BASE_PATH || ''}${import.meta.env.BASE_URL || '/'}`, window.location.origin);
  currentUrl.hash = route.fullPath;
  let currentUri = currentUrl.toString();
  let redirectUrl = `${WGPORTAL_BACKEND_BASE_URL}${provider.ProviderUrl}`;
  redirectUrl += "?redirect=true";
  redirectUrl += "&return=" + encodeURIComponent(currentUri);
  window.location.href = redirectUrl;
}
</script>

<template>
  <div class="login-shell">
    <div class="card login-card">
      <div class="card-header">
        {{ $t('login.headline') }}
        <div class="float-end">
          <RouterLink :to="{ name: 'home' }" class="nav-link" :title="$t('menu.home')"><i class="fas fa-times-circle"></i></RouterLink>
        </div>
      </div>
      <div class="card-body">
        <div class="login-logo">
          <img src="@/assets/wg-logo.webp" alt="AWG Portal" class="login-logo-icon" />
          <span class="login-logo-text">AWG Portal</span>
        </div>
        <form method="post">
          <fieldset>
            <div class="form-group">
              <label class="form-label" for="inputUsername">{{ $t('login.username.label') }}</label>
              <div class="input-group mb-3">
                <span class="input-group-text"><span class="far fa-user p-2"></span></span>
                <input id="inputUsername" v-model="username" :class="{'is-invalid':usernameInvalid, 'is-valid':usernameValid}" :placeholder="$t('login.username.placeholder')" aria-describedby="usernameHelp"
                       class="form-control"
                       name="username" type="text">
              </div>
            </div>
            <div class="form-group">
              <label class="form-label" for="inputPassword">{{ $t('login.password.label') }}</label>
              <div class="input-group mb-3">
                <span class="input-group-text"><span class="fas fa-lock p-2"></span></span>
                <input id="inputPassword" :key="passwordInputKey" ref="passwordRef" v-model="password" :class="{'is-invalid':passwordInvalid, 'is-valid':passwordValid}" :placeholder="$t('login.password.placeholder')" class="form-control"
                       name="password" type="password">
              </div>
            </div>

            <div class="row mt-5 mb-2">
              <div class="col-sm-4 col-xs-12">
                <button :disabled="disableLoginBtn" class="btn btn-primary mb-2" type="submit" @click.prevent="login">
                  {{ $t('login.button') }} <div v-if="loggingIn" class="d-inline"><i class="ms-2 fa-solid fa-circle-notch fa-spin"></i></div>
                </button>
              </div>
              <div class="col-sm-8 col-xs-12 text-sm-end">
                <button v-if="settings.Setting('WebAuthnEnabled')" class="btn btn-primary" type="submit" @click.prevent="loginWebAuthn">
                  {{ $t('login.button-webauthn') }} <div v-if="loggingIn" class="d-inline"><i class="ms-2 fa-solid fa-circle-notch fa-spin"></i></div>
                </button>
              </div>
            </div>

            <div class="row mt-4 d-flex">
              <div class="col-lg-12 d-flex mb-2">
                <!-- OpenIdConnect / OAUTH providers -->
                <button v-for="(provider, idx) in auth.LoginProviders" :key="provider.Identifier" :class="{'ms-1':idx > 0}"
                        :disabled="loggingIn" :title="provider.Name" class="btn btn-outline-primary flex-fill"
                        v-html="provider.Name" @click.prevent="externalLogin(provider)"></button>
              </div>
            </div>

            <div class="mt-3">
            </div>
          </fieldset>
        </form>
      </div>
    </div>
  </div>
</template>
