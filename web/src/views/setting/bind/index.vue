<template>
    <div class="gateway-bind-page">
        <div class="gateway-bind-content">
            <h2>绑定 WorkMesh Gateway</h2>
            <p class="gateway-bind-description">登录独立的 Gateway 账号以启用节点上的 WorkMesh 功能。</p>
            <el-form label-position="top" @submit.prevent="bindGateway">
                <el-form-item label="Gateway 用户名">
                    <el-input v-model="form.username" autocomplete="username" placeholder="请输入 Gateway 用户名" />
                </el-form-item>
                <el-form-item label="Gateway 密码">
                    <el-input
                        v-model="form.password"
                        type="password"
                        show-password
                        autocomplete="current-password"
                        placeholder="请输入 Gateway 密码"
                        @keyup.enter="bindGateway"
                    />
                </el-form-item>
                <el-button type="primary" :loading="loading" class="bind-button" @click="bindGateway">
                    绑定并继续
                </el-button>
            </el-form>
            <div class="gateway-register">
                没有 Gateway 账号？
                <a :href="registerURL" target="_blank" rel="noopener">注册账号</a>
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessage } from 'element-plus';
import { getWorkMeshGatewayStatus } from '@/api/modules/workmesh';
import { gatewayLoginApi } from '@/api/modules/auth';

const router = useRouter();
const loading = ref(false);
const gatewayURL = ref('https://work.zoomtk.com');
const form = reactive({ username: '', password: '' });
const registerURL = computed(() => `${gatewayURL.value.replace(/\/$/, '')}/login/register`);

const bindGateway = async () => {
    if (!form.username.trim() || !form.password) {
        ElMessage.error('请输入 Gateway 用户名和密码');
        return;
    }
    loading.value = true;
    try {
        await gatewayLoginApi({ username: form.username.trim(), password: form.password });
        ElMessage.success('Gateway 账号绑定成功');
        await router.replace({ name: 'home' });
    } catch (error: any) {
        ElMessage.error(error?.message || 'Gateway 账号绑定失败');
    } finally {
        loading.value = false;
    }
};

onMounted(async () => {
    try {
        const status = await getWorkMeshGatewayStatus();
        gatewayURL.value = status.data.gatewayUrl || gatewayURL.value;
        if (status.data.configured) await router.replace({ name: 'home' });
    } catch {
        // 页面仍可显示默认 Gateway 地址并允许用户重试。
    }
});
</script>

<style scoped>
.gateway-bind-page {
    display: flex;
    justify-content: center;
    padding: 64px 24px;
}

.gateway-bind-content {
    width: min(100%, 420px);
}

.gateway-bind-content h2 {
    margin: 0 0 12px;
    color: var(--el-text-color-primary);
    font-size: 24px;
    font-weight: 600;
}

.gateway-bind-description {
    margin: 0 0 28px;
    color: var(--el-text-color-secondary);
    line-height: 1.6;
}

.bind-button {
    width: 100%;
}

.gateway-register {
    margin-top: 18px;
    color: var(--el-text-color-secondary);
    font-size: 13px;
    text-align: center;
}

.gateway-register a {
    color: var(--el-color-primary);
}
</style>
