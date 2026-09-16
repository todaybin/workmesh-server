<template>
    <div class="page">
        <section class="table-panel">
            <div class="actions">
                <el-button plain @click="clearAll">清空所有日志</el-button>
                <el-button plain :disabled="!selected.length" @click="persistSelected">批量拉黑 IP</el-button>
                <el-button plain :disabled="!selected.length" @click="persistSelectedCIDR">批量拉黑 IP 段</el-button>
                <el-button plain @click="exportLogs">导出日志</el-button>
                <span class="spacer" />
                <el-input v-model="ip" clearable placeholder="请输入 IP" class="ip-search" @keyup.enter="load" />
                <el-button :icon="Refresh" @click="load">刷新</el-button>
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
            <el-alert
                title="封锁 IP 临时存储在 OpenResty 中，重启 OpenResty 会解封；可以通过拉黑功能永久拉黑。"
                type="info"
                :closable="false"
                class="notice"
            />
            <el-table
                v-if="rows.length || loading"
                :data="rows"
                stripe
                @selection-change="selected = $event"
                v-loading="loading"
            >
                <el-table-column type="selection" width="48" />
                <el-table-column label="IP" min-width="180">
                    <template #default="{ row }">{{ String(row.client_ip || row.clientIP || row.ip || '-') }}</template>
                </el-table-column>
                <el-table-column v-if="isColumnVisible('time')" prop="time" label="时间" width="190" />
                <el-table-column v-if="isColumnVisible('status')" prop="status" label="状态" width="90" />
                <el-table-column v-if="isColumnVisible('blockTime')" prop="blockTime" label="封禁时间" width="110" />
                <el-table-column v-if="isColumnVisible('region')" prop="ipRegion" label="IP 归属地" min-width="150" />
                <el-table-column v-if="isColumnVisible('rule')" prop="rule" label="命中规则" min-width="170" />
                <el-table-column label="操作" width="90">
                    <template #default="{ row }">
                        <el-button
                            link
                            type="primary"
                            @click="persistIP(String(row.client_ip || row.clientIP || row.ip || ''))"
                        >
                            永久拉黑
                        </el-button>
                    </template>
                </el-table-column>
            </el-table>
            <el-empty v-else description="暂无数据" :image-size="54" />
            <div class="pager">
                <el-pagination
                    v-model:current-page="page"
                    v-model:page-size="pageSize"
                    :total="total"
                    layout="total, sizes, prev, pager, next, jumper"
                    @current-change="load"
                    @size-change="load"
                />
            </div>
        </section>
    </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Refresh, Setting } from '@element-plus/icons-vue';
import { clearWafAudit, getWafAccessLists, getWafAudit, getWafStatus, updateWafAccessLists } from '@/api/modules/waf';
const rows = ref<Array<Record<string, any>>>([]);
const selected = ref<Array<Record<string, any>>>([]);
const loading = ref(false);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const ip = ref('');
const autoRefresh = ref(false);
let timer: ReturnType<typeof setInterval> | undefined;
const columnOptions = [
    { key: 'time', label: '时间' },
    { key: 'status', label: '状态' },
    { key: 'blockTime', label: '封禁时间' },
    { key: 'region', label: 'IP 归属地' },
    { key: 'rule', label: '命中规则' },
];
const visibleColumns = ref(columnOptions.map((column) => column.key));
const isColumnVisible = (key: string) => visibleColumns.value.includes(key);
const rowIP = (row: Record<string, any>) => String(row.client_ip || row.clientIP || row.ip || '').trim();
const load = async () => {
    loading.value = true;
    try {
        const result: any = await getWafAudit('block', { page: page.value, pageSize: pageSize.value, ip: ip.value });
        rows.value = result.data?.items || [];
        total.value = result.data?.total || 0;
    } catch (error: any) {
        rows.value = [];
        total.value = 0;
        ElMessage.error(error?.message || '封锁记录加载失败');
    } finally {
        loading.value = false;
    }
};
const persistIPs = async (values: string[], message: string) => {
    const ips = [...new Set(values.map((value) => value.trim()).filter(Boolean))];
    if (!ips.length) return;
    try {
        const result: any = await getWafAccessLists();
        const lists = result.data || { whitelist: [], blacklist: [] };
        lists.blacklist = [...new Set([...(lists.blacklist || []), ...ips])];
        await updateWafAccessLists(lists);
        const status: any = await getWafStatus();
        if (status.data?.effective === true) ElMessage.success(`${message}并确认 OpenResty 生效`);
        else ElMessage.warning(status.data?.error || `${message}，但尚未确认 OpenResty 生效`);
    } catch (error: any) {
        ElMessage.error(error?.message || `${message}失败`);
    }
};
const persistIP = async (value: string) => persistIPs([value], '已保存');
const persistSelected = async () => {
    await persistIPs(selected.value.map(rowIP), 'IP 已保存');
    await load();
};
const toCIDR = (value: string) => {
    if (value.includes(':')) return `${value.split(':').slice(0, 4).join(':')}::/64`;
    const parts = value.split('.');
    return parts.length === 4 ? `${parts.slice(0, 3).join('.')}.0/24` : value;
};
const persistSelectedCIDR = async () => {
    await persistIPs(
        selected.value.map((row) => toCIDR(rowIP(row))),
        'IP 段已保存',
    );
    await load();
};
const clearAll = async () => {
    try {
        await ElMessageBox.confirm('确认清空封锁记录？', '清空日志');
        await clearWafAudit('block');
        ElMessage.success('日志已清空');
        await load();
    } catch (error: any) {
        if (error !== 'cancel' && error !== 'close') ElMessage.error(error?.message || '清空日志失败');
    }
};
const exportLogs = () => {
    const link = document.createElement('a');
    link.href = URL.createObjectURL(new Blob([JSON.stringify(rows.value, null, 2)], { type: 'application/json' }));
    link.download = 'waf-block-logs.json';
    link.click();
    URL.revokeObjectURL(link.href);
};
watch(autoRefresh, (enabled) => {
    if (timer) clearInterval(timer);
    timer = enabled ? setInterval(load, 10000) : undefined;
});
onMounted(() => {
    load();
    window.addEventListener('workmesh:waf-refresh', load);
});
onBeforeUnmount(() => {
    if (timer) clearInterval(timer);
    window.removeEventListener('workmesh:waf-refresh', load);
});
</script>

<style scoped>
.page {
    display: flex;
    flex-direction: column;
    gap: 10px;
}
.table-panel {
    padding: 18px 12px 12px;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-light);
}
.actions {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    margin-bottom: 12px;
}
.spacer {
    flex: 1;
}
.ip-search {
    width: 220px;
}
.notice {
    margin-bottom: 14px;
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
