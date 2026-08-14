<template>
  <div class="login">
    <div class="login-stage">
      <div class="login-brand">
        <img class="brand-mark" src="/favicon.svg" width="40" height="40" alt="" />
        <h1 class="brand-name">Woops</h1>
        <p class="brand-tagline">{{ t('login.tagline') }}</p>
      </div>

      <div class="login-panel">
        <div v-if="loadingOptions" class="hint">{{ t('login.loadingOptions') }}</div>
        <template v-else>
          <el-form
            v-if="localLoginEnabled && step === 'credentials'"
            class="login-form"
            label-position="top"
            require-asterisk-position="right"
            @submit.prevent="onSubmit"
          >
            <el-form-item :label="t('login.username')">
              <el-input v-model="username" autocomplete="username" size="large" />
            </el-form-item>
            <el-form-item :label="t('login.password')">
              <el-input v-model="password" type="password" autocomplete="current-password" size="large" show-password />
            </el-form-item>
            <el-button type="primary" native-type="submit" size="large" :loading="loading" class="full">
              {{ t('login.submit') }}
            </el-button>
          </el-form>

          <el-form
            v-else-if="localLoginEnabled && step === 'setup'"
            class="login-form"
            label-position="top"
            @submit.prevent="onSetup"
          >
            <p class="hint top">{{ t('login.totpSetupHint') }}</p>
            <div v-if="qrCodeDataUrl" class="qr-wrap">
              <img :src="qrCodeDataUrl" :alt="t('login.totpQrAlt')" class="qr" />
            </div>
            <el-form-item :label="t('login.totpSecret')">
              <el-input :model-value="totpSecret" readonly class="secret-input">
                <template #append>
                  <el-button @click="copySecret">{{ t('login.copy') }}</el-button>
                </template>
              </el-input>
            </el-form-item>
            <el-form-item :label="t('login.totpCode')">
              <el-input
                v-model="totpCode"
                maxlength="6"
                inputmode="numeric"
                autocomplete="one-time-code"
                size="large"
              />
            </el-form-item>
            <el-button type="primary" native-type="submit" size="large" :loading="loading" class="full">
              {{ t('login.totpConfirm') }}
            </el-button>
            <el-button link class="back" @click="backToCredentials">{{ t('login.back') }}</el-button>
          </el-form>

          <el-form
            v-else-if="localLoginEnabled && step === 'verify'"
            class="login-form"
            label-position="top"
            @submit.prevent="onVerify"
          >
            <p class="hint top">{{ t('login.totpVerifyHint') }}</p>
            <el-form-item :label="t('login.totpCode')">
              <el-input
                ref="totpInputRef"
                v-model="totpCode"
                maxlength="6"
                inputmode="numeric"
                autocomplete="one-time-code"
                size="large"
              />
            </el-form-item>
            <el-button type="primary" native-type="submit" size="large" :loading="loading" class="full">
              {{ t('login.totpConfirm') }}
            </el-button>
            <el-button link class="back" @click="backToCredentials">{{ t('login.back') }}</el-button>
          </el-form>

          <el-button
            v-if="gitlabEnabled && step === 'credentials'"
            type="primary"
            size="large"
            :plain="localLoginEnabled"
            class="full gitlab"
            @click="onGitLab"
          >
            {{ t('login.gitlab') }}
          </el-button>

          <p v-if="errorMsg" class="error">{{ errorMsg }}</p>
          <p v-if="step === 'credentials'" class="hint foot">
            <template v-if="gitlabEnabled && !localLoginEnabled">{{ t('login.hintGitlabOnly') }}</template>
            <template v-else-if="localLoginEnabled">{{ t('login.hintLocalDefault') }}</template>
          </p>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup>
import { nextTick, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../../shared/api'
import { clearSession, refreshMe, setSession } from '../../shared/auth'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const username = ref('')
const password = ref('')
const loading = ref(false)
const loadingOptions = ref(true)
const localLoginEnabled = ref(true)
const gitlabEnabled = ref(false)
const errorMsg = ref('')
const step = ref('credentials')
const pendingToken = ref('')
const totpSecret = ref('')
const qrCodeDataUrl = ref('')
const totpCode = ref('')
const totpInputRef = ref(null)

async function applySession(data) {
  setSession({
    token: data.accessToken,
    username: data.user.username,
    nickname: data.user.nickname || '',
    role: data.user.role
  })
  await refreshMe()
  router.push('/home')
}

async function applyToken(token) {
  setSession({ token })
  await refreshMe()
  router.replace('/home')
}

function resetChallenge() {
  pendingToken.value = ''
  totpSecret.value = ''
  qrCodeDataUrl.value = ''
  totpCode.value = ''
}

function backToCredentials() {
  step.value = 'credentials'
  resetChallenge()
  errorMsg.value = ''
}

async function copySecret() {
  try {
    await navigator.clipboard.writeText(totpSecret.value)
    ElMessage.success(t('login.copied'))
  } catch {
    ElMessage.error(t('login.copyFailed'))
  }
}

async function onSubmit() {
  loading.value = true
  errorMsg.value = ''
  try {
    const { data } = await api.post('/auth/login', {
      username: username.value,
      password: password.value
    })
    if (data.accessToken) {
      await applySession(data)
      return
    }
    if (data.requiresTotpSetup) {
      pendingToken.value = data.pendingToken
      totpSecret.value = data.secret || ''
      qrCodeDataUrl.value = data.qrCodeDataUrl || ''
      totpCode.value = ''
      step.value = 'setup'
      return
    }
    if (data.requiresTotp) {
      pendingToken.value = data.pendingToken
      totpCode.value = ''
      step.value = 'verify'
      await nextTick()
      totpInputRef.value?.focus?.()
      return
    }
    ElMessage.error(t('login.failed'))
  } catch (e) {
    ElMessage.error(e.response?.data?.error || t('login.failed'))
  } finally {
    loading.value = false
  }
}

async function onSetup() {
  if (loading.value) return
  loading.value = true
  errorMsg.value = ''
  try {
    const { data } = await api.post('/auth/login/totp-setup', {
      pendingToken: pendingToken.value,
      code: totpCode.value
    })
    await applySession(data)
  } catch (e) {
    ElMessage.error(totpErrorMessage(e))
  } finally {
    loading.value = false
  }
}

async function onVerify() {
  if (loading.value) return
  loading.value = true
  errorMsg.value = ''
  try {
    const { data } = await api.post('/auth/login/totp', {
      pendingToken: pendingToken.value,
      code: totpCode.value
    })
    await applySession(data)
  } catch (e) {
    ElMessage.error(totpErrorMessage(e))
  } finally {
    loading.value = false
  }
}

function totpErrorMessage(e) {
  const raw = e.response?.data?.error
  if (raw === 'invalid totp code' || raw === 'totp not enabled') {
    return t('login.totpFailed')
  }
  return raw || t('login.totpFailed')
}

function onGitLab() {
  window.location.href = '/api/auth/gitlab/authorize'
}

onMounted(async () => {
  const qToken = route.query.token
  const qError = route.query.error
  if (qError) {
    errorMsg.value = String(qError)
    clearSession()
  }
  try {
    const { data } = await api.get('/auth/login-options')
    localLoginEnabled.value = !!data.localLoginEnabled
    gitlabEnabled.value = !!data.gitlabEnabled
  } catch {
    localLoginEnabled.value = true
  } finally {
    loadingOptions.value = false
  }
  if (qToken && typeof qToken === 'string') {
    try {
      await applyToken(qToken)
      return
    } catch (e) {
      clearSession()
      errorMsg.value = e.response?.data?.error || t('login.gitlabFailed')
    }
  }
})
</script>

<style scoped>
.login {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 32px 16px;
  background:
    radial-gradient(ellipse 80% 60% at 10% 0%, rgba(47, 93, 159, 0.12), transparent 55%),
    radial-gradient(ellipse 70% 50% at 100% 100%, rgba(47, 93, 159, 0.08), transparent 50%),
    linear-gradient(165deg, #eef2f6 0%, var(--ops-bg-page) 45%, #e8edf3 100%);
  background-attachment: fixed;
}

.login::before {
  content: '';
  position: fixed;
  inset: 0;
  pointer-events: none;
  opacity: 0.35;
  background-image:
    linear-gradient(var(--ops-border) 1px, transparent 1px),
    linear-gradient(90deg, var(--ops-border) 1px, transparent 1px);
  background-size: 48px 48px;
  mask-image: radial-gradient(ellipse 70% 60% at 50% 40%, #000 20%, transparent 75%);
}

.login-stage {
  position: relative;
  z-index: 1;
  width: min(420px, 100%);
  display: flex;
  flex-direction: column;
  gap: 28px;
}

.login-brand {
  text-align: center;
}

.brand-mark {
  width: 40px;
  height: 40px;
  margin: 0 auto 16px;
  border-radius: var(--ops-radius);
  display: block;
  box-shadow: var(--ops-shadow-sm);
}

.brand-name {
  margin: 0;
  font-size: clamp(40px, 8vw, 52px);
  font-weight: 600;
  letter-spacing: -0.04em;
  line-height: 1.05;
  color: var(--ops-text);
}

.brand-tagline {
  margin: 10px 0 0;
  font-size: 14px;
  color: var(--ops-text-secondary);
}

.login-panel {
  background: var(--ops-bg-surface);
  border: 1px solid var(--ops-border);
  border-radius: var(--ops-radius);
  padding: 28px 24px 24px;
  box-shadow: var(--ops-shadow-sm);
}

.login-form :deep(.el-form-item) {
  margin-bottom: 16px;
}

.login-form :deep(.el-form-item__label) {
  color: var(--ops-text-secondary);
  font-weight: 500;
  margin-bottom: 6px !important;
  line-height: 1.2;
  justify-content: flex-start;
}

.full {
  width: 100%;
}

.gitlab {
  margin-top: 12px;
}

.back {
  width: 100%;
  margin-top: 8px;
}

.hint.top {
  margin: 0 0 16px;
}

.hint.foot {
  margin: 16px 0 0;
}

.error {
  color: var(--ops-danger);
  font-size: 13px;
  margin: 12px 0 0;
}

.qr-wrap {
  display: flex;
  justify-content: center;
  margin: 0 0 16px;
}

.qr {
  width: 180px;
  height: 180px;
  border: 1px solid var(--ops-border);
  border-radius: var(--ops-radius-sm);
  background: #fff;
}

.secret-input :deep(.el-input__inner) {
  font-family: var(--ops-font-mono);
  font-size: 12px;
  letter-spacing: 0.02em;
}
</style>
