<!-- SPDX-License-Identifier: LicenseRef-WorkMesh-Pending -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { CircleCheck, CircleX, LoaderCircle, RefreshCw, ShieldCheck } from 'lucide-vue-next'
import { getGatewayStatus, loginGateway } from './api/client'
import type { CapabilityRoute, GatewayLoginRequest, GatewayStatus } from './api/contracts'

const status = ref<GatewayStatus | null>(null)
const loading = ref(true)
const submitting = ref(false)
const error = ref('')
const form = ref<GatewayLoginRequest>({ username: '', password: '' })
const selectedRoute = ref<CapabilityRoute>('local')

const registrationLabel = computed(() => {
  if (!status.value) return '未读取'
  return {
    registered: '已注册',
    pending: '等待授权',
    revoked: '授权已撤销',
    unregistered: '未注册',
  }[status.value.registration]
})

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    status.value = await getGatewayStatus()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '无法读取 Gateway 状态'
  } finally {
    loading.value = false
  }
}

async function submitLogin() {
  submitting.value = true
  error.value = ''
  try {
    const response = await loginGateway(form.value)
    await refresh()
    if (response.authorizationUrl) window.location.assign(response.authorizationUrl)
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Gateway 授权失败'
  } finally {
    submitting.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <main class="shell">
    <header class="topbar">
      <div>
        <p class="eyebrow">WORKMESH SERVER</p>
        <h1>节点控制台</h1>
      </div>
      <button class="icon-button" type="button" title="刷新 Gateway 状态" :disabled="loading" @click="refresh">
        <RefreshCw :size="18" :class="{ spin: loading }" />
      </button>
    </header>

    <section class="grid">
      <article class="panel status-panel">
        <div class="panel-title"><ShieldCheck :size="18" /><span>Gateway 使用授权</span></div>
        <div v-if="loading" class="loading"><LoaderCircle class="spin" :size="20" /> 正在读取状态</div>
        <div v-else-if="status" class="status-content">
          <div class="status-line">
            <component :is="status.registration === 'registered' ? CircleCheck : CircleX" :size="22" :class="status.registration === 'registered' ? 'ok' : 'warn'" />
            <strong>{{ registrationLabel }}</strong>
            <span class="muted">{{ status.connected ? 'Gateway 在线' : 'Gateway 离线' }}</span>
          </div>
          <dl>
            <div><dt>节点 ID</dt><dd>{{ status.nodeId || '未分配' }}</dd></div>
            <div><dt>当前角色</dt><dd>{{ status.role === 'primary' ? '主节点' : '次节点' }}</dd></div>
            <div v-if="status.reason" class="reason"><dt>说明</dt><dd>{{ status.reason }}</dd></div>
          </dl>
        </div>
        <p v-else class="empty">尚未获得 Gateway 状态。请确认服务端接口已启用。</p>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
      </article>

      <article class="panel login-panel">
        <div class="panel-title"><ShieldCheck :size="18" /><span>Gateway 登录与注册</span></div>
        <form @submit.prevent="submitLogin">
          <label>Gateway 账号<input v-model="form.username" autocomplete="username" required /></label>
          <label>密码<input v-model="form.password" type="password" autocomplete="current-password" required /></label>
          <label>Gateway 地址（可选）<input v-model="form.gatewayUrl" placeholder="使用服务端默认地址" /></label>
          <button class="primary" type="submit" :disabled="submitting">
            <LoaderCircle v-if="submitting" class="spin" :size="17" />
            {{ submitting ? '正在授权' : '登录并注册当前节点' }}
          </button>
        </form>
      </article>
    </section>

    <section class="panel capability-panel">
      <div class="panel-title"><ShieldCheck :size="18" /><span>能力调用路由</span></div>
      <p class="muted">路由选择会随能力请求发送到服务端；此页面不执行任务。</p>
      <div class="route-options" role="radiogroup" aria-label="能力调用路由">
        <label v-for="route in (['local', 'gateway', 'auto'] as CapabilityRoute[])" :key="route" class="route-option">
          <input v-model="selectedRoute" type="radio" :value="route" />
          <span>{{ route === 'local' ? '本机执行' : route === 'gateway' ? 'Gateway 授权' : '自动选择' }}</span>
        </label>
      </div>
      <p class="selection">当前选择：<strong>{{ selectedRoute }}</strong></p>
    </section>
  </main>
</template>
