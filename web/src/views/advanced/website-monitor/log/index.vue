<template>
    <LayoutContent :title="$t('serverPages.websiteMonitor.log')" v-loading="loading">
        <template #rightToolBar><TableRefresh @search="load" /></template>
        <template #main>
            <el-card shadow="never">
                <el-form :inline="true">
                    <el-form-item :label="$t('serverPages.websiteMonitor.time')">
                        <el-date-picker v-model="range" type="daterange" value-format="YYYY-MM-DD" />
                    </el-form-item>
                    <el-form-item label="IP"><el-input v-model="query.ip" clearable /></el-form-item>
                    <el-form-item label="URI"><el-input v-model="query.uri" clearable /></el-form-item>
                    <el-form-item :label="$t('serverPages.websiteMonitor.method')">
                        <el-select v-model="query.method" clearable>
                            <el-option v-for="method in methods" :key="method" :label="method" :value="method" />
                        </el-select>
                    </el-form-item>
                    <el-form-item :label="$t('serverPages.websiteMonitor.statusCode')">
                        <el-input-number v-model="query.status" :min="0" :max="599" controls-position="right" />
                    </el-form-item>
                    <el-form-item>
                        <el-button type="primary" @click="load">{{ $t('serverPages.websiteMonitor.query') }}</el-button>
                        <el-button @click="exportLogs">{{ $t('serverPages.websiteMonitor.export') }}</el-button>
                        <el-button type="danger" plain @click="clearLogs">{{ $t('serverPages.websiteMonitor.clean') }}</el-button>
                    </el-form-item>
                </el-form>
            </el-card>
            <el-card shadow="never" class="mt-4">
                <el-table :data="logs" stripe>
                    <el-table-column prop="occurredAt" :label="$t('serverPages.websiteMonitor.time')" width="190" />
                    <el-table-column prop="ip" label="IP" width="150" />
                    <el-table-column prop="method" :label="$t('serverPages.websiteMonitor.method')" width="90" />
                    <el-table-column prop="uri" label="URI" min-width="260" />
                    <el-table-column prop="status" :label="$t('serverPages.websiteMonitor.statusCode')" width="90" />
                    <el-table-column prop="bytes" :label="$t('serverPages.websiteMonitor.flow')" width="100" />
                    <el-table-column prop="durationMs" :label="$t('serverPages.websiteMonitor.duration')" width="110" />
                    <el-table-column :label="$t('serverPages.multiNode.operation')" width="90">
                        <template #default="{ row }">
                            <el-button link type="primary" @click="detail(row)">{{ $t('serverPages.websiteMonitor.detail') }}</el-button>
                        </template>
                    </el-table-column>
                </el-table>
                <div class="pager">
                    <el-pagination
                        v-model:current-page="query.page"
                        v-model:page-size="query.pageSize"
                        :total="total"
                        layout="total, prev, pager, next, sizes"
                        @current-change="load"
                        @size-change="load"
                    />
                </div>
            </el-card>
            <el-dialog v-model="detailOpen" :title="$t('serverPages.websiteMonitor.detail')" width="720px">
                <pre class="detail">{{ JSON.stringify(selected, null, 2) }}</pre>
            </el-dialog>
        </template>
    </LayoutContent>
</template>
<script setup lang="ts">
import { reactive, ref } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import i18n from '@/lang';
import { clearMonitorLogs, monitorLogDetail, monitorLogs } from '@/api/modules/website-monitor';
const loading = ref(false);
const logs = ref<any[]>([]);
const total = ref(0);
const range = ref<string[]>([]);
const detailOpen = ref(false);
const selected = ref<any>();
const methods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'];
const query = reactive<any>({ page: 1, pageSize: 20, ip: '', uri: '', method: '', status: 0 });
const load = async () => {
    loading.value = true;
    try {
        const res: any = await monitorLogs({ ...query, startTime: range.value[0], endTime: range.value[1] });
        logs.value = res.data?.items || [];
        total.value = res.data?.total || 0;
    } finally {
        loading.value = false;
    }
};
const detail = async (row: any) => {
    const res: any = await monitorLogDetail({ ...query, page: 1, pageSize: 1, ip: row.ip, uri: row.uri });
    selected.value = res.data || row;
    detailOpen.value = true;
};
const clearLogs = async () => {
    await ElMessageBox.confirm(
        i18n.global.t('serverPages.websiteMonitor.clearConfirm'),
        i18n.global.t('serverPages.websiteMonitor.clean'),
    );
    await clearMonitorLogs({ ...query, startTime: range.value[0], endTime: range.value[1] });
    ElMessage.success(i18n.global.t('serverPages.websiteMonitor.clearSuccess'));
    await load();
};
const exportLogs = () => {
    const blob = new Blob([JSON.stringify(logs.value, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'website-monitor-logs.json';
    link.click();
    URL.revokeObjectURL(url);
};
load();
</script>
<style scoped>
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
}
.detail {
    white-space: pre-wrap;
    max-height: 520px;
    overflow: auto;
}
</style>
