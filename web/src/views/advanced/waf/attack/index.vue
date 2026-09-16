<template>
    <div class="page">
        <div class="toolbar">
            <el-date-picker
                v-model="range"
                type="daterange"
                value-format="YYYY-MM-DD"
                start-placeholder="开始日期"
                end-placeholder="结束日期"
                @change="load"
            />
        </div>
        <el-row :gutter="8">
            <el-col :xs="24" :lg="8">
                <section class="panel">
                    <h3>
                        <i />
                        IP 报表 (TOP 100)
                    </h3>
                    <el-table v-if="ipReport.length" :data="ipReport" size="small">
                        <el-table-column prop="value" label="攻击 IP" min-width="150" />
                        <el-table-column prop="ipRegion" label="IP 归属地" min-width="110" />
                        <el-table-column prop="count" label="攻击次数" width="90" />
                        <el-table-column prop="ratio" label="占比" width="80" />
                        <el-table-column label="操作" width="70">
                            <template #default="{ row }">
                                <el-button link type="primary" @click="blockIP(row.value)">拉黑</el-button>
                            </template>
                        </el-table-column>
                    </el-table>
                    <el-empty v-else description="暂无数据" :image-size="54" />
                </section>
            </el-col>
            <el-col :xs="24" :lg="8">
                <section class="panel">
                    <h3>
                        <i />
                        URL 报表 (TOP 100)
                    </h3>
                    <el-table v-if="urlReport.length" :data="urlReport" size="small">
                        <el-table-column prop="value" label="URL" min-width="180" show-overflow-tooltip />
                        <el-table-column prop="count" label="攻击次数" width="90" />
                        <el-table-column prop="ratio" label="占比" width="80" />
                        <el-table-column label="操作" width="70">
                            <template #default="{ row }">
                                <el-button link type="primary" @click="allowURL(row.value)">加白</el-button>
                            </template>
                        </el-table-column>
                    </el-table>
                    <el-empty v-else description="暂无数据" :image-size="54" />
                </section>
            </el-col>
            <el-col :xs="24" :lg="8">
                <section class="panel">
                    <h3>
                        <i />
                        网站报表 (TOP 100)
                    </h3>
                    <el-table v-if="siteReport.length" :data="siteReport" size="small">
                        <el-table-column prop="value" label="网站" min-width="170" show-overflow-tooltip />
                        <el-table-column prop="count" label="攻击次数" width="90" />
                        <el-table-column prop="ratio" label="占比" width="80" />
                    </el-table>
                    <el-empty v-else description="暂无数据" :image-size="54" />
                </section>
            </el-col>
        </el-row>
    </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import { ElMessage } from 'element-plus';
import {
    getWafAccessLists,
    getWafAudit,
    getWafStatus,
    updateWafAccessLists,
    type WafAccessLists,
} from '@/api/modules/waf';

type ReportRow = { value: string; count: number; ratio: string; ipRegion?: string };
const today = new Date().toISOString().slice(0, 10);
const range = ref<string[]>([today, today]);
const rows = ref<Array<Record<string, any>>>([]);
const ipReport = ref<ReportRow[]>([]);
const urlReport = ref<ReportRow[]>([]);
const siteReport = ref<ReportRow[]>([]);
const valueOf = (row: Record<string, any>, ...keys: string[]) => {
    for (const key of keys)
        if (row[key] !== undefined && row[key] !== null && String(row[key]).trim()) return String(row[key]);
    return '-';
};
const aggregate = (key: string, includeRegion = false): ReportRow[] => {
    const counts = new Map<string, { count: number; ipRegion: string }>();
    rows.value.forEach((row) => {
        const value = valueOf(row, key, ...(key === 'client_ip' ? ['clientIP', 'ip'] : []));
        if (value === '-') return;
        const current = counts.get(value) || { count: 0, ipRegion: '-' };
        current.count += 1;
        if (includeRegion && current.ipRegion === '-') {
            current.ipRegion = valueOf(row, 'ipRegion', 'ip_region', 'region', 'country');
        }
        counts.set(value, current);
    });
    const total = rows.value.length || 1;
    return [...counts.entries()]
        .sort((a, b) => b[1].count - a[1].count)
        .slice(0, 100)
        .map(([value, item]) => ({
            value,
            count: item.count,
            ratio: `${((item.count / total) * 100).toFixed(1)}%`,
            ...(includeRegion ? { ipRegion: item.ipRegion } : {}),
        }));
};
const load = async () => {
    try {
        const params = {
            page: 1,
            pageSize: 500,
            startTime: range.value[0],
            endTime: range.value[1],
        };
        const result = await getWafAudit('attack', params);
        const items = [...(result.data?.items || [])];
        const total = Number(result.data?.total || items.length);
        const pageCount = Math.ceil(total / params.pageSize);
        if (pageCount > 1) {
            const pages = await Promise.all(
                Array.from({ length: pageCount - 1 }, (_, index) =>
                    getWafAudit('attack', { ...params, page: index + 2 }),
                ),
            );
            for (const page of pages) items.push(...(page.data?.items || []));
        }
        rows.value = items;
        ipReport.value = aggregate('client_ip', true);
        urlReport.value = aggregate('uri');
        siteReport.value = aggregate('host');
    } catch (error: any) {
        rows.value = [];
        ipReport.value = [];
        urlReport.value = [];
        siteReport.value = [];
        ElMessage.error(error?.message || '攻击报表加载失败');
    }
};
const updateList = async (mutate: (lists: WafAccessLists) => void) => {
    const result: any = await getWafAccessLists();
    const lists: WafAccessLists = result.data || { whitelist: [], blacklist: [] };
    mutate(lists);
    await updateWafAccessLists(lists);
    const status: any = await getWafStatus();
    if (status.data?.effective === true) ElMessage.success('已保存并确认 OpenResty 生效');
    else ElMessage.warning(status.data?.error || '已保存，但尚未确认 OpenResty 生效');
};
const blockIP = async (ip: string) => {
    try {
        await updateList((lists) => {
            lists.blacklist = [...new Set([...(lists.blacklist || []), ip])];
        });
    } catch (error: any) {
        ElMessage.error(error?.message || '拉黑 IP 失败');
    }
};
const allowURL = async (url: string) => {
    try {
        await updateList((lists) => {
            lists.urlWhitelist = [...new Set([...(lists.urlWhitelist || []), url])];
        });
    } catch (error: any) {
        ElMessage.error(error?.message || '加白 URL 失败');
    }
};
onMounted(() => {
    load();
    window.addEventListener('workmesh:waf-refresh', load);
});
onBeforeUnmount(() => window.removeEventListener('workmesh:waf-refresh', load));
</script>

<style scoped>
.page {
    display: flex;
    flex-direction: column;
    gap: 10px;
}
.toolbar {
    display: flex;
    min-height: 48px;
    align-items: center;
    padding: 8px 12px;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-light);
}
.toolbar :deep(.el-date-editor) {
    width: 450px;
}
.panel {
    min-height: 445px;
    padding: 18px 12px 12px;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-light);
}
h3 {
    margin: 0 0 18px;
    font-size: 16px;
    font-weight: 500;
}
h3 i {
    display: inline-block;
    width: 4px;
    height: 18px;
    margin-right: 10px;
    vertical-align: -3px;
    background: var(--el-color-primary);
    border-radius: 2px;
}
.page :deep(.el-empty__image) {
    display: none;
}
</style>
