<template>
    <div class="page">
        <section class="filter-panel">
            <el-select v-model="query.host" clearable placeholder="网站 请选择" class="field">
                <el-option v-for="site in sites" :key="site.alias" :label="site.alias" :value="site.alias" />
            </el-select>
            <el-select
                v-model="query.rule"
                clearable
                filterable
                allow-create
                placeholder="命中规则 请选择"
                class="field wide"
            >
                <el-option v-for="rule in ruleOptions" :key="rule" :label="rule" :value="rule" />
            </el-select>
            <el-select v-model="query.action" clearable placeholder="动作 所有" class="field">
                <el-option label="所有" value="" />
                <el-option label="拦截" value="block" />
                <el-option label="记录" value="log" />
                <el-option label="放行" value="allow" />
            </el-select>
            <el-input v-model="query.hostKeyword" clearable placeholder="请输入 Host，支持模糊" class="field" />
            <el-input v-model="query.uri" clearable placeholder="请输入 URL，支持模糊" class="field" />
            <el-input v-model="query.ip" clearable placeholder="请输入 IP" class="field" />
            <el-input v-model="query.ipRegion" clearable placeholder="请输入 IP 归属地" class="field" />
        </section>
        <section class="table-panel">
            <div class="actions">
                <el-button plain @click="clearAll">清空所有日志</el-button>
                <el-button plain :disabled="!selected.length" @click="batchBlock">批量拉黑 IP</el-button>
                <el-button plain :disabled="!selected.length" @click="batchBlockCIDR">批量拉黑 IP 段</el-button>
                <el-button plain :disabled="!selected.length" @click="batchAllowURL">批量加白 URL</el-button>
                <el-button plain @click="exportLogs">导出日志</el-button>
                <span class="spacer" />
                <el-button :icon="Refresh" @click="search">刷新</el-button>
                <el-button @click="autoRefresh = !autoRefresh">{{ autoRefresh ? '停止刷新' : '不刷新' }}</el-button>
                <el-popover placement="bottom-end" :width="190" trigger="click">
                    <template #reference>
                        <el-tooltip content="列设置" placement="top">
                            <el-button :icon="Setting" circle />
                        </el-tooltip>
                    </template>
                    <el-checkbox-group v-model="visibleColumns">
                        <el-checkbox v-for="column in columnOptions" :key="column.key" :label="column.key">
                            {{ column.label }}
                        </el-checkbox>
                    </el-checkbox-group>
                </el-popover>
            </div>
            <el-table v-if="rows.length || loading" :data="rows" stripe @selection-change="selected = $event" v-loading="loading">
                <el-table-column type="selection" width="48" />
                <el-table-column label="IP" width="150">
                    <template #default="{ row }">{{ rowIP(row) || '-' }}</template>
                </el-table-column>
                <el-table-column
                    v-if="isColumnVisible('host')"
                    prop="host"
                    label="域名"
                    min-width="170"
                    show-overflow-tooltip
                />
                <el-table-column
                    v-if="isColumnVisible('uri')"
                    prop="uri"
                    label="URL"
                    min-width="240"
                    show-overflow-tooltip
                />
                <el-table-column v-if="isColumnVisible('region')" prop="ipRegion" label="IP 归属地" min-width="120" />
                <el-table-column v-if="isColumnVisible('action')" prop="action" label="动作" width="90" />
                <el-table-column
                    v-if="isColumnVisible('rule')"
                    prop="rule"
                    label="命中规则"
                    width="150"
                    show-overflow-tooltip
                />
                <el-table-column v-if="isColumnVisible('category')" prop="category" label="类型" width="110" />
                <el-table-column v-if="isColumnVisible('time')" prop="time" label="时间" width="180" />
                <el-table-column label="操作" width="90">
                    <template #default="{ row }">
                        <el-button link type="primary" @click="blockSingle(row)">拉黑</el-button>
                    </template>
                </el-table-column>
            </el-table>
            <el-empty v-else description="暂无数据" :image-size="54" />
            <div class="pager">
                <el-pagination
                    v-model:current-page="query.page"
                    v-model:page-size="query.pageSize"
                    :total="total"
                    layout="total, sizes, prev, pager, next, jumper"
                    @current-change="search"
                    @size-change="search"
                />
            </div>
        </section>
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Refresh, Setting } from '@element-plus/icons-vue';
import {
    getWafAccessLists,
    getWafAudit,
    getWafStatus,
    listWafSites,
    updateWafAccessLists,
    clearWafAudit,
    listWafStandardRules,
    type WafAccessLists,
} from '@/api/modules/waf';
const loading = ref(false);
const rows = ref<Array<Record<string, any>>>([]);
const selected = ref<Array<Record<string, any>>>([]);
const total = ref(0);
const sites = ref<any[]>([]);
const ruleOptions = ref<string[]>([]);
const autoRefresh = ref(false);
let timer: ReturnType<typeof setInterval> | undefined;
const columnOptions = [
    { key: 'host', label: '域名' },
    { key: 'uri', label: 'URL' },
    { key: 'region', label: 'IP 归属地' },
    { key: 'action', label: '动作' },
    { key: 'rule', label: '命中规则' },
    { key: 'category', label: '类型' },
    { key: 'time', label: '时间' },
];
const visibleColumns = ref(columnOptions.map((column) => column.key));
const query = reactive<any>({
    page: 1,
    pageSize: 20,
    host: '',
    hostKeyword: '',
    rule: '',
    action: '',
    uri: '',
    ip: '',
    ipRegion: '',
});
const normalized = computed(() => ({ ...query, host: query.host || query.hostKeyword }));
const rowIP = (row: Record<string, any>) => String(row.client_ip || row.clientIP || row.ip || '').trim();
const toCIDR = (ip: string) => {
    if (ip.includes(':')) return `${ip.split(':').slice(0, 4).join(':')}::/64`;
    const parts = ip.split('.');
    return parts.length === 4 ? `${parts.slice(0, 3).join('.')}.0/24` : ip;
};
const search = async () => {
    loading.value = true;
    try {
        const result = await getWafAudit('intercept', normalized.value);
        rows.value = result.data?.items || [];
        total.value = result.data?.total || 0;
    } catch (error: any) {
        rows.value = [];
        total.value = 0;
        ElMessage.error(error?.message || '拦截记录加载失败');
    } finally {
        loading.value = false;
    }
};
const lists = async (mutate: (value: WafAccessLists) => void) => {
    const result: any = await getWafAccessLists();
    const value: WafAccessLists = result.data || { whitelist: [], blacklist: [] };
    mutate(value);
    await updateWafAccessLists(value);
    return getWafStatus();
};
const isColumnVisible = (key: string) => visibleColumns.value.includes(key);
const persistIPs = async (values: string[], message: string) => {
    const ips = [...new Set(values.map((value) => value.trim()).filter(Boolean))];
    if (!ips.length) return;
    try {
        const status: any = await lists((value) => {
            value.blacklist = [...new Set([...(value.blacklist || []), ...ips])];
        });
        status.data?.effective === true
            ? ElMessage.success(`${message}并确认 OpenResty 生效`)
            : ElMessage.warning(status.data?.error || `${message}，但尚未确认 OpenResty 生效`);
    } catch (error: any) {
        ElMessage.error(error?.message || `${message}失败`);
    }
};
const batchBlock = async () => {
    await persistIPs(selected.value.map(rowIP), 'IP 已保存');
};
const batchBlockCIDR = async () => {
    await persistIPs(
        selected.value.map((row) => toCIDR(rowIP(row))),
        'IP 段已保存',
    );
};
const blockSingle = async (row: Record<string, any>) => {
    await persistIPs([rowIP(row)], 'IP 已保存');
};
const batchAllowURL = async () => {
    const urls = [...new Set(selected.value.map((row) => String(row.uri || '').trim()).filter(Boolean))];
    if (!urls.length) return;
    try {
        const status: any = await lists((value) => {
            value.urlWhitelist = [...new Set([...(value.urlWhitelist || []), ...urls])];
        });
        status.data?.effective === true
            ? ElMessage.success('URL 已保存并确认 OpenResty 生效')
            : ElMessage.warning(status.data?.error || 'URL 已保存，但尚未确认 OpenResty 生效');
    } catch (error: any) {
        ElMessage.error(error?.message || 'URL 加白失败');
    }
};
const clearAll = async () => {
    try {
        await ElMessageBox.confirm('确认清空当前 WAF 日志？', '清空日志');
        await clearWafAudit('intercept');
        ElMessage.success('日志已清空');
        await search();
    } catch (error: any) {
        if (error !== 'cancel' && error !== 'close') ElMessage.error(error?.message || '清空日志失败');
    }
};
const exportLogs = () => {
    const link = document.createElement('a');
    link.href = URL.createObjectURL(new Blob([JSON.stringify(rows.value, null, 2)], { type: 'application/json' }));
    link.download = 'waf-intercept-logs.json';
    link.click();
    URL.revokeObjectURL(link.href);
};
watch(autoRefresh, (enabled) => {
    if (timer) clearInterval(timer);
    timer = enabled ? setInterval(search, 10000) : undefined;
});
onMounted(async () => {
    try {
        const [siteResult, standardResult] = await Promise.all([listWafSites(), listWafStandardRules()]);
        sites.value = siteResult.data || [];
        ruleOptions.value = (standardResult.data || []).map((rule) => rule.id || rule.category).filter(Boolean);
        await search();
        window.addEventListener('workmesh:waf-refresh', search);
    } catch (error: any) {
        ElMessage.error(error?.message || '拦截记录初始化失败');
    }
});
onBeforeUnmount(() => {
    if (timer) clearInterval(timer);
    window.removeEventListener('workmesh:waf-refresh', search);
});
</script>

<style scoped>
.page {
    display: flex;
    flex-direction: column;
    gap: 10px;
}
.filter-panel,
.table-panel {
    padding: 10px 12px;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-light);
}
.filter-panel {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
}
.field {
    width: 220px;
}
.wide {
    width: 440px;
}
.table-panel {
    padding-top: 18px;
}
.actions {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 16px;
    flex-wrap: wrap;
}
.spacer {
    flex: 1;
}
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
}
.page :deep(.el-empty__image) {
    display: none;
}
</style>
