<script setup>

import {ref} from "vue";

const privateKey = ref("")
const publicKey = ref("")
const presharedKey = ref("")
const keygenError = ref("")
const copyStatus = ref({ priv: false, pub: false, psk: false })

async function generateKeypair() {
  const webCrypto = globalThis.crypto
  if (webCrypto?.subtle) {
    try {
      const keyPair = await webCrypto.subtle.generateKey(
          { name: 'X25519' },
          true,
          ['deriveBits']
      );

      const pubJwk  = await webCrypto.subtle.exportKey('jwk', keyPair.publicKey);
      const privJwk = await webCrypto.subtle.exportKey('jwk', keyPair.privateKey);

      return {
        publicKey:  b64urlToB64(pubJwk.x),
        privateKey: b64urlToB64(privJwk.d)
      };
    } catch (e) {
      console.debug("Web Crypto X25519 is unavailable, using local fallback", e)
    }
  }

  return generateKeypairFallback()
}

function generatePresharedKey() {
  return randomBytes(32);
}

function generateKeypairFallback() {
  const privateBytes = clampPrivateKey(randomBytes(32))
  const publicBytes = x25519(privateBytes)

  return {
    privateKey: arrayBufferToBase64(privateBytes),
    publicKey: arrayBufferToBase64(publicBytes)
  }
}

function randomBytes(length) {
  const webCrypto = globalThis.crypto
  if (!webCrypto?.getRandomValues) {
    throw new Error("Web Crypto random generator is not available")
  }

  const bytes = new Uint8Array(length);
  webCrypto.getRandomValues(bytes);
  return bytes;
}

function b64urlToB64(input) {
  let b64 = input.replace(/-/g, '+').replace(/_/g, '/');
  while (b64.length % 4) {
    b64 += '=';
  }
  return b64;
}

function arrayBufferToBase64(buffer) {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (let i = 0; i < bytes.byteLength; ++i) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}

function clampPrivateKey(bytes) {
  const clamped = new Uint8Array(bytes)
  clamped[0] &= 248
  clamped[31] &= 127
  clamped[31] |= 64
  return clamped
}

const X25519_P = (1n << 255n) - 19n
const X25519_A24 = 121665n
const X25519_BASEPOINT = new Uint8Array(32)
X25519_BASEPOINT[0] = 9

function x25519(privateBytes, basepoint = X25519_BASEPOINT) {
  const scalar = bytesToBigIntLE(clampPrivateKey(privateBytes))
  const x1 = bytesToBigIntLE(basepoint)
  let x2 = 1n
  let z2 = 0n
  let x3 = x1
  let z3 = 1n
  let swap = 0

  for (let t = 254; t >= 0; t--) {
    const kT = Number((scalar >> BigInt(t)) & 1n)
    swap ^= kT
    if (swap) {
      ;[x2, x3] = [x3, x2]
      ;[z2, z3] = [z3, z2]
    }
    swap = kT

    const a = mod(x2 + z2)
    const aa = mod(a * a)
    const b = mod(x2 - z2)
    const bb = mod(b * b)
    const e = mod(aa - bb)
    const c = mod(x3 + z3)
    const d = mod(x3 - z3)
    const da = mod(d * a)
    const cb = mod(c * b)

    x3 = mod((da + cb) ** 2n)
    z3 = mod(x1 * mod((da - cb) ** 2n))
    x2 = mod(aa * bb)
    z2 = mod(e * mod(aa + X25519_A24 * e))
  }

  if (swap) {
    ;[x2, x3] = [x3, x2]
    ;[z2, z3] = [z3, z2]
  }

  return bigIntToBytesLE(mod(x2 * modInv(z2)))
}

function bytesToBigIntLE(bytes) {
  let value = 0n
  for (let i = bytes.length - 1; i >= 0; i--) {
    value = (value << 8n) + BigInt(bytes[i])
  }
  return value
}

function bigIntToBytesLE(value) {
  const bytes = new Uint8Array(32)
  let remaining = value
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = Number(remaining & 255n)
    remaining >>= 8n
  }
  return bytes
}

function mod(value) {
  const result = value % X25519_P
  return result >= 0n ? result : result + X25519_P
}

function modInv(value) {
  return powMod(value, X25519_P - 2n)
}

function powMod(base, exponent) {
  let result = 1n
  let x = mod(base)
  let n = exponent

  while (n > 0n) {
    if (n & 1n) {
      result = mod(result * x)
    }
    x = mod(x * x)
    n >>= 1n
  }

  return result
}

async function generateNewKeyPair() {
  keygenError.value = ""
  try {
    const keypair = await generateKeypair();
    privateKey.value = keypair.privateKey;
    publicKey.value = keypair.publicKey;
  } catch (e) {
    keygenError.value = e?.message || String(e)
  }
}

function generateNewPresharedKey() {
  keygenError.value = ""
  try {
    const rawPsk = generatePresharedKey();
    presharedKey.value = arrayBufferToBase64(rawPsk);
  } catch (e) {
    keygenError.value = e?.message || String(e)
  }
}

async function copyToClipboard(value, field) {
  if (!value) return
  try {
    await navigator.clipboard.writeText(value)
    copyStatus.value[field] = true
    setTimeout(() => { copyStatus.value[field] = false }, 1500)
  } catch (e) {
    // fallback
    const el = document.createElement('textarea')
    el.value = value
    document.body.appendChild(el)
    el.select()
    document.execCommand('copy')
    document.body.removeChild(el)
    copyStatus.value[field] = true
    setTimeout(() => { copyStatus.value[field] = false }, 1500)
  }
}
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ $t('keygen.headline') }}</h1>
      <p>{{ $t('keygen.abstract') }}</p>
    </div>
  </div>

  <p v-if="keygenError" class="form-hint text-danger">{{ keygenError }}</p>

  <div class="keygen-grid">
    <div class="keygen-card">
      <h3>
        <svg viewBox="0 0 24 24" fill="none" stroke="var(--accent)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="width:18px;height:18px;">
          <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/>
        </svg>
        {{ $t('keygen.headline-keypair') }}
      </h3>
      <div class="keygen-output">
        <div class="key-field">
          <label>{{ $t('keygen.private-key.label') }}</label>
          <div class="key-value">
            <input type="text" v-model="privateKey" readonly :placeholder="$t('keygen.private-key.placeholder')">
            <button class="btn btn-secondary btn-sm" @click.prevent="copyToClipboard(privateKey, 'priv')">
              <i class="fa-solid" :class="copyStatus.priv ? 'fa-check' : 'fa-copy'"></i>
            </button>
          </div>
        </div>
        <div class="key-field">
          <label>{{ $t('keygen.public-key.label') }}</label>
          <div class="key-value">
            <input type="text" v-model="publicKey" readonly :placeholder="$t('keygen.public-key.placeholder')">
            <button class="btn btn-secondary btn-sm" @click.prevent="copyToClipboard(publicKey, 'pub')">
              <i class="fa-solid" :class="copyStatus.pub ? 'fa-check' : 'fa-copy'"></i>
            </button>
          </div>
        </div>
      </div>
      <button class="btn btn-primary" type="button" @click.prevent="generateNewKeyPair">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="width:16px;height:16px;">
          <polyline points="23 4 23 10 17 10"/>
          <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
        </svg>
        {{ $t('keygen.button-generate') }}
      </button>
    </div>

    <div class="keygen-card">
      <h3>
        <svg viewBox="0 0 24 24" fill="none" stroke="var(--purple)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="width:18px;height:18px;">
          <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
          <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
        </svg>
        {{ $t('keygen.headline-preshared-key') }}
      </h3>
      <div class="keygen-output">
        <div class="key-field">
          <label>{{ $t('keygen.preshared-key.label') }}</label>
          <div class="key-value">
            <input type="text" v-model="presharedKey" readonly :placeholder="$t('keygen.preshared-key.placeholder')">
            <button class="btn btn-secondary btn-sm" @click.prevent="copyToClipboard(presharedKey, 'psk')">
              <i class="fa-solid" :class="copyStatus.psk ? 'fa-check' : 'fa-copy'"></i>
            </button>
          </div>
        </div>
      </div>
      <button class="btn btn-primary" type="button" @click.prevent="generateNewPresharedKey" style="background: var(--purple); border-color: var(--purple);">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="width:16px;height:16px;">
          <polyline points="23 4 23 10 17 10"/>
          <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
        </svg>
        {{ $t('keygen.button-generate') }}
      </button>
    </div>
  </div>
</template>
