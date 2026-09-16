<template>
    <div class="global-page" v-loading="loading">
        <div class="section-tabs">
            <el-radio-group v-model="section">
                <el-radio-button label="frequency">{{ $t('xpack.waf.frequencyLimit') }}</el-radio-button>
                <el-radio-button label="default">{{ $t('xpack.waf.defaultRule') }}</el-radio-button>
                <el-radio-button label="custom">{{ $t('xpack.waf.customRule') }}</el-radio-button>
                <el-radio-button label="config">配置</el-radio-button>
            </el-radio-group>
        </div>

        <section class="main-switch">
            <div>
                <h3>
                    <i />
                    WAF {{ $t('xpack.waf.mainSwitch') }}
                    <el-tag :type="global.enabled ? 'success' : 'info'">
                        {{ global.enabled ? '已启用' : '已关闭' }}
                    </el-tag>
                </h3>
                <p>关闭之后所有网站将失去防护</p>
            </div>
            <el-switch
                v-model="global.enabled"
                inline-prompt
                active-text="开启"
                inactive-text="关闭"
                @change="saveGlobal(false)"
            />
        </section>
        <div class="helper-bar">
            带有「网站」标签的设置，需要在「网站设置」配置生效；全局设置仅为新建网站的默认设置。
            设置生效需要「全局设置」和「网站设置」的开关同时打开。
        </div>

        <div v-if="section === 'frequency'" class="freq-layout">
            <el-menu :default-active="freq" class="freq-menu" @select="freq = String($event)">
                <el-menu-item v-for="item in freqItems" :key="item.key" :index="item.key">
                    {{ item.label }}
                </el-menu-item>
            </el-menu>
            <section class="freq-panel">
                <div class="freq-heading">
                    <h3>{{ currentFreq.label }}</h3>
                    <el-switch v-model="currentLimit.enabled" />
                </div>
                <p>
                    {{ currentLimit.period }} 秒内累计请求任意网站超过 {{ currentLimit.count }} 次，封锁此 IP
                    {{ currentLimit.blockTime }} 分钟
                    <el-tag size="small">网站</el-tag>
                </p>
                <el-form :inline="true" label-width="72px">
                    <el-form-item label="模式" required>
                        <el-select v-model="currentLimit.mode" class="mode-select">
                            <el-option label="全局模式" value="global" />
                            <el-option label="URL 模式" value="uri" />
                        </el-select>
                    </el-form-item>
                    <el-form-item label="周期" required>
                        <el-input-number
                            v-model="currentLimit.period"
                            :min="1"
                            :max="86400"
                            controls-position="right"
                        />
                        <span class="unit">秒</span>
                    </el-form-item>
                    <el-form-item label="频率" required>
                        <el-input-number
                            v-model="currentLimit.count"
                            :min="1"
                            :max="1000000"
                            controls-position="right"
                        />
                        <span class="unit">次</span>
                    </el-form-item>
                    <el-form-item label="封禁时间" required>
                        <el-input-number
                            v-model="currentLimit.blockTime"
                            :min="1"
                            :max="525600"
                            controls-position="right"
                        />
                        <span class="unit">分钟</span>
                    </el-form-item>
                </el-form>
                <div class="buttons">
                    <el-button type="primary" @click="saveGlobal(false)">{{ $t('xpack.waf.saveDefault') }}</el-button>
                    <el-button @click="applyToSites">{{ $t('xpack.waf.saveToWebsite') }}</el-button>
                </div>
                <p class="desc">
                    全局模式：单位时间请求任意 URL 次数之和超过阈值即触发
                    <br />
                    URL 模式：单位时间请求单个 URL 次数超过阈值即触发
                </p>
                <p class="warn">封锁 IP 临时存储在 OpenResty 中，重启 OpenResty 会解封，可以通过拉黑功能永久拉黑</p>
            </section>
        </div>

        <section v-else-if="section === 'default'" class="form-panel">
            <el-form label-width="150px">
                <el-form-item label="标准规则"><el-switch v-model="global.standardRules" /></el-form-item>
                <el-form-item label="执行策略">
                    <el-radio-group v-model="global.mode">
                        <el-radio-button label="block">防护模式</el-radio-button>
                        <el-radio-button label="observe">观察模式</el-radio-button>
                    </el-radio-group>
                </el-form-item>
                <el-form-item label="Paranoia Level">
                    <el-input-number v-model="global.paranoiaLevel" :min="1" :max="4" />
                </el-form-item>
                <el-form-item label="异常评分阈值">
                    <el-input-number v-model="global.inboundThreshold" :min="1" :max="99" />
                </el-form-item>
                <el-form-item>
                    <el-button type="primary" @click="saveGlobal(false)">{{ $t('commons.button.save') }}</el-button>
                </el-form-item>
            </el-form>
            <div class="default-rules-heading">
                <div>
                    <h3>默认规则明细</h3>
                    <p>规则保存到全局 WAF 默认规则文件，并在启用标准规则时参与请求匹配。</p>
                </div>
                <el-button plain @click="appendDefaultRule">新增规则</el-button>
            </div>
            <el-table
                v-if="defaultRules.length || defaultRuleLoading"
                :data="defaultRules"
                border
                v-loading="defaultRuleLoading"
            >
                <el-table-column label="规则名称" min-width="150">
                    <template #default="{ row }"><el-input v-model="row.name" /></template>
                </el-table-column>
                <el-table-column label="位置" width="120">
                    <template #default="{ row }">
                        <el-select v-model="row.location">
                            <el-option label="URL" value="uri" />
                            <el-option label="参数" value="args" />
                            <el-option label="请求头" value="header" />
                            <el-option label="请求体" value="body" />
                            <el-option label="IP 组" value="ip-group" />
                        </el-select>
                    </template>
                </el-table-column>
                <el-table-column label="匹配值" min-width="220">
                    <template #default="{ row }"><el-input v-model="row.value" /></template>
                </el-table-column>
                <el-table-column label="动作" width="110">
                    <template #default="{ row }">
                        <el-select v-model="row.action">
                            <el-option label="放行" value="allow" />
                            <el-option label="记录" value="log" />
                            <el-option label="拦截" value="block" />
                        </el-select>
                    </template>
                </el-table-column>
                <el-table-column label="优先级" width="110">
                    <template #default="{ row }">
                        <el-input-number v-model="row.priority" :min="1" :max="10000" />
                    </template>
                </el-table-column>
                <el-table-column label="启用" width="80">
                    <template #default="{ row }"><el-switch v-model="row.enabled" /></template>
                </el-table-column>
                <el-table-column label="操作" width="80">
                    <template #default="{ $index }">
                        <el-button link type="danger" @click="defaultRules.splice($index, 1)">删除</el-button>
                    </template>
                </el-table-column>
            </el-table>
            <el-empty v-else description="暂无默认规则" :image-size="54" />
            <div class="default-rules-footer">
                <el-button type="primary" @click="saveDefaultRules">保存默认规则</el-button>
            </div>
        </section>

        <section v-else-if="section === 'custom'" class="form-panel">
            <el-form label-width="110px">
                <el-form-item label="自定义规则">
                    <el-input
                        v-model="customText"
                        type="textarea"
                        :rows="14"
                        placeholder="每行一条规则，按 URI 包含匹配保存为阻断规则"
                    />
                </el-form-item>
                <el-form-item>
                    <el-button type="primary" @click="saveCustom">{{ $t('commons.button.save') }}</el-button>
                </el-form-item>
            </el-form>
        </section>

        <section v-else class="form-panel">
            <el-form label-width="150px">
                <el-form-item label="请求体大小(MB)">
                    <el-input-number v-model="bodyLimitMB" :min="0" :max="10" />
                </el-form-item>
                <el-form-item label="严格模式"><el-switch v-model="global.strictMode" /></el-form-item>
                <el-form-item label="Redis"><el-switch v-model="redis.enabled" /></el-form-item>
                <el-form-item label="Redis 地址"><el-input v-model="redis.host" /></el-form-item>
                <el-form-item label="Redis 端口">
                    <el-input-number v-model="redis.port" :min="1" :max="65535" />
                </el-form-item>
                <el-form-item label="Redis DB"><el-input-number v-model="redis.db" :min="0" :max="255" /></el-form-item>
                <el-form-item>
                    <el-button type="primary" @click="saveGlobal(false)">{{ $t('commons.button.save') }}</el-button>
                </el-form-item>
            </el-form>
        </section>
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import {
    applyWafGlobalToSites,
    getWafCustomRules,
    getWafDefaultRules,
    getWafGlobalConfig,
    getWafStatus,
    updateWafCustomRules,
    updateWafDefaultRules,
    updateWafGlobal,
    type WafRule,
    type WafFrequencyLimit,
    type WafGlobalConfig,
    type WafStatus,
} from '@/api/modules/waf';

const section = ref('frequency');
const freq = ref('access');
const loading = ref(false);
const runtime = ref<WafStatus>();
const savedConfig = ref<WafGlobalConfig>();
const bodyLimitMB = ref(1);
const customText = ref('');
const defaultRules = ref<WafRule[]>([]);
const defaultRuleLoading = ref(false);
const defaults: WafFrequencyLimit = { enabled: false, mode: 'uri', period: 10, count: 100, blockTime: 10 };
const freqItems = [
    { key: 'access', label: '访问频率限制' },
    { key: 'attack', label: '攻击频率限制' },
    { key: 'notFound', label: '404 频率限制' },
    { key: 'url', label: 'URL 频率限制' },
];
const global = reactive<WafGlobalConfig>({
    enabled: true,
    standardRules: true,
    mode: 'observe',
    paranoiaLevel: 1,
    inboundThreshold: 5,
    requestBodyLimit: 1048576,
    strictMode: false,
    frequency: Object.fromEntries(freqItems.map((item) => [item.key, { ...defaults }])),
    frequencyLimit: Object.fromEntries(freqItems.map((item) => [item.key, { ...defaults }])),
});
const redis = reactive({ enabled: false, host: '127.0.0.1', port: 6379, db: 0, password: '' });
const currentFreq = computed(() => freqItems.find((item) => item.key === freq.value) || freqItems[0]);
const currentLimit = computed<WafFrequencyLimit>(() => global.frequencyLimit?.[freq.value] || defaults);
const runtimeType = computed(() =>
    runtime.value?.effective ? 'success' : runtime.value?.configured ? 'warning' : 'info',
);
const runtimeTitle = computed(() => {
    if (runtime.value?.effective)
        return `OpenResty 已确认生效${runtime.value.configHash ? `，配置 ${runtime.value.configHash.slice(0, 12)}` : ''}`;
    if (runtime.value?.configured) return runtime.value.error || 'OpenResty 已配置，但当前 WAF 文件尚未确认生效';
    return '当前未启用强制 OpenResty reload，保存只更新 WorkMesh WAF 文件；生产环境需配置 WORKMESH_WAF_RELOAD 或 OpenResty 目标';
});
const normalizeConfig = (data?: WafGlobalConfig) => {
    if (!data) return;
    Object.assign(global, data);
    const source = data.frequencyLimit || data.frequency || {};
    global.frequencyLimit = { ...source };
    global.frequency = global.frequencyLimit;
    for (const item of freqItems) {
        global.frequencyLimit[item.key] ||= { ...defaults };
    }
    bodyLimitMB.value = Math.round(((global.requestBodyLimit || 1048576) / 1024 / 1024) * 10) / 10;
    Object.assign(redis, {
        enabled: false,
        host: '127.0.0.1',
        port: 6379,
        db: 0,
        password: '',
        ...(global.redis || {}),
    });
};
const normalizeDefaultRules = (value: unknown): WafRule[] => {
    if (!Array.isArray(value)) return [];
    return value.map((rule: any, index) => ({
        id: rule.id ? String(rule.id) : undefined,
        websiteID: Number(rule.websiteID) || 0,
        name: String(rule.name || `default-${index + 1}`),
        location: String(rule.location || 'uri'),
        key: rule.key ? String(rule.key) : undefined,
        operator: String(rule.operator || 'contains'),
        value: String(rule.value || ''),
        action: rule.action === 'allow' || rule.action === 'log' ? rule.action : 'block',
        priority: Number(rule.priority) || 100,
        enabled: rule.enabled !== false,
    }));
};
const refreshRuntime = async () => {
    const result: any = await getWafStatus();
    runtime.value = result.data;
    return runtime.value;
};
const confirmEffective = async () => {
    const status = await refreshRuntime();
    if (status?.effective === true) {
        ElMessage.success('已保存并确认 OpenResty 生效');
    } else {
        ElMessage.warning(status?.error || '已保存，但尚未确认 OpenResty 生效');
    }
};
const load = async () => {
    loading.value = true;
    try {
        const [statusResult, configResult, customResult, defaultResult]: any = await Promise.all([
            getWafStatus(),
            getWafGlobalConfig(),
            getWafCustomRules(),
            getWafDefaultRules(),
        ]);
        runtime.value = statusResult.data;
        normalizeConfig(configResult.data);
        savedConfig.value = JSON.parse(JSON.stringify(global)) as WafGlobalConfig;
        customText.value = (customResult.data?.rules || [])
            .map((rule: any) => rule.value || JSON.stringify(rule))
            .join('\n');
        defaultRules.value = normalizeDefaultRules(defaultResult.data);
    } finally {
        loading.value = false;
    }
};
const payload = () => {
    global.requestBodyLimit = Math.round(bodyLimitMB.value * 1024 * 1024);
    global.redis = { ...redis };
    global.frequency = { ...(global.frequencyLimit || {}) };
    global.frequencyLimit = global.frequency;
    return JSON.parse(JSON.stringify(global)) as WafGlobalConfig;
};
const saveGlobal = async (applyAll: boolean) => {
    const before = savedConfig.value
        ? (JSON.parse(JSON.stringify(savedConfig.value)) as WafGlobalConfig)
        : (JSON.parse(JSON.stringify(global)) as WafGlobalConfig);
    try {
        await updateWafGlobal(payload());
        if (applyAll) await applyWafGlobalToSites();
        await confirmEffective();
        await load();
    } catch (error: any) {
        normalizeConfig(before);
        ElMessage.error(error?.message || 'WAF 全局设置保存失败');
    }
};
const applyToSites = async () => {
    await ElMessageBox.confirm(
        '是否将默认规则明细应用到所有网站？全局频率、模式和 Redis 配置不会覆盖已有站点设置。',
        '确认',
    );
    await saveGlobal(true);
};
const saveCustom = async () => {
    try {
        const rules: WafRule[] = customText.value
            .split(/\r?\n/)
            .map((line) => line.trim())
            .filter(Boolean)
            .map((line, index) => ({
                id: String(index + 1),
                websiteID: 0,
                name: `custom-${index + 1}`,
                location: 'uri',
                operator: 'contains',
                value: line,
                action: 'block',
                priority: 100,
                enabled: true,
            }));
        await updateWafCustomRules(rules);
        await confirmEffective();
    } catch (error: any) {
        ElMessage.error(error?.message || '自定义规则保存失败');
    }
};
const appendDefaultRule = () => {
    defaultRules.value.push({
        websiteID: 0,
        name: '',
        location: 'uri',
        operator: 'contains',
        value: '',
        action: 'block',
        priority: 100,
        enabled: true,
    });
};
const saveDefaultRules = async () => {
    if (defaultRules.value.some((rule) => !rule.name.trim() || !rule.value.trim())) {
        ElMessage.warning('请填写默认规则名称和匹配值');
        return;
    }
    try {
        await updateWafDefaultRules(defaultRules.value);
        await confirmEffective();
        await load();
    } catch (error: any) {
        ElMessage.error(error?.message || '默认规则保存失败');
    }
};
onMounted(() => {
    load();
    window.addEventListener('workmesh:waf-refresh', load);
});
onBeforeUnmount(() => window.removeEventListener('workmesh:waf-refresh', load));
</script>

<style scoped>
.global-page {
    display: flex;
    flex-direction: column;
    gap: 10px;
}
.section-tabs,
.main-switch,
.form-panel,
.freq-panel {
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-light);
    border-radius: 4px;
}
.section-tabs {
    padding: 20px 22px 16px;
}
.section-tabs :deep(.el-radio-button__inner) {
    min-width: 100px;
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
    border-color: var(--el-color-primary-light-5);
    box-shadow: none;
}
.section-tabs :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
    color: #fff;
    background: var(--el-color-primary);
    border-color: var(--el-color-primary);
    box-shadow: -1px 0 0 0 var(--el-color-primary);
}
.main-switch {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 18px 22px;
    border-left: 4px solid var(--el-color-primary);
}
.main-switch h3 {
    display: flex;
    align-items: center;
    gap: 10px;
    margin: 0 0 8px;
    font-size: 16px;
}
.main-switch h3 i {
    width: 10px;
    height: 10px;
    background: var(--el-color-primary);
    border-radius: 50%;
}
.main-switch p {
    margin: 0;
    color: var(--el-text-color-secondary);
}
.main-switch :deep(.el-switch) {
    --el-switch-on-color: var(--el-color-primary);
}
.runtime-alert {
    margin: 0;
}
.helper-bar {
    padding: 12px 16px;
    color: var(--el-text-color-secondary);
    background: var(--el-fill-color-lighter);
    border-radius: 4px;
}
.freq-layout {
    display: grid;
    grid-template-columns: 150px 1fr;
    gap: 10px;
}
.freq-menu {
    border: 0;
    border-right: 1px solid var(--el-border-color-light);
    background: var(--el-bg-color);
}
.freq-menu :deep(.el-menu-item.is-active) {
    border-right: 2px solid var(--el-color-primary);
}
.freq-panel,
.form-panel {
    padding: 28px 22px;
}
.freq-heading {
    display: flex;
    align-items: center;
    gap: 16px;
}
.freq-heading h3 {
    margin: 0;
    font-size: 18px;
    font-weight: 500;
}
.freq-panel p {
    color: var(--el-text-color-secondary);
}
.mode-select {
    width: 220px;
}
.unit {
    margin-left: 8px;
    color: var(--el-text-color-secondary);
}
.buttons {
    margin: 18px 0;
}
.desc {
    line-height: 1.8;
}
.warn {
    color: #e6a23c !important;
}
.global-page :deep(.el-empty__image) {
    display: none;
}
.default-rules-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    margin: 24px 0 14px;
    padding-top: 20px;
    border-top: 1px solid var(--el-border-color-light);
}
.default-rules-heading h3 {
    margin: 0 0 6px;
    font-size: 16px;
    font-weight: 500;
}
.default-rules-heading p {
    margin: 0;
    color: var(--el-text-color-secondary);
}
.default-rules-footer {
    display: flex;
    justify-content: flex-end;
    margin-top: 14px;
}
@media (max-width: 900px) {
    .freq-layout {
        grid-template-columns: 1fr;
    }
    .freq-menu {
        border-right: 0;
    }
    .main-switch {
        align-items: flex-start;
        gap: 16px;
    }
    .default-rules-heading {
        flex-direction: column;
    }
}
</style>
