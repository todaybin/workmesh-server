<template>
    <div v-loading="loading">
        <el-card v-if="type === 'dashboard'" shadow="never">
            <template #header>{{ $t('serverPages.websiteMonitor.dashboard') }}</template>
            <el-row :gutter="12">
                <el-col v-for="item in cards" :key="item.key" :span="4">
                    <el-statistic :title="item.label" :value="item.value" />
                </el-col>
            </el-row>
            <el-divider />
            <el-row :gutter="16">
                <el-col :span="16">
                    <el-table :data="stats" stripe>
                        <el-table-column prop="day" :label="$t('serverPages.websiteMonitor.date')" />
                        <el-table-column prop="pv" :label="$t('serverPages.websiteMonitor.pv')" />
                        <el-table-column prop="uv" :label="$t('serverPages.websiteMonitor.uv')" />
                        <el-table-column prop="flow" :label="$t('serverPages.websiteMonitor.flow')" />
                        <el-table-column prop="req" :label="$t('serverPages.websiteMonitor.requests')" />
                    </el-table>
                </el-col>
                <el-col :span="8">
                    <el-table :data="locations" stripe size="small">
                        <el-table-column prop="name" :label="$t('serverPages.websiteMonitor.visitorRegion')" />
                        <el-table-column prop="value" :label="$t('serverPages.websiteMonitor.visits')" width="90" />
                    </el-table>
                </el-col>
            </el-row>
        </el-card>
        <el-card v-else-if="type === 'rank'" shadow="never">
            <template #header>{{ $t('serverPages.websiteMonitor.rank') }}</template>
            <el-radio-group v-model="rankType" @change="load">
                <el-radio-button v-for="item in rankTypes" :key="item.value" :label="item.value">
                    {{ item.label }}
                </el-radio-button>
            </el-radio-group>
            <el-table :data="rank" class="mt-3">
                <el-table-column type="index" width="60" />
                <el-table-column prop="name" :label="$t('serverPages.websiteMonitor.name')" />
                <el-table-column prop="value" :label="$t('serverPages.websiteMonitor.count')" />
            </el-table>
        </el-card>
        <el-card v-else-if="type === 'log'" shadow="never">
            <template #header>{{ $t('serverPages.websiteMonitor.log') }}</template>
            <el-form :inline="true">
                <el-form-item :label="$t('serverPages.websiteMonitor.ip')"><el-input v-model="query.ip" clearable /></el-form-item>
                <el-form-item :label="$t('serverPages.websiteMonitor.uri')"><el-input v-model="query.uri" clearable /></el-form-item>
                <el-form-item><el-button type="primary" @click="load">{{ $t('serverPages.websiteMonitor.query') }}</el-button></el-form-item>
            </el-form>
            <el-table :data="logs">
                <el-table-column prop="occurredAt" :label="$t('serverPages.websiteMonitor.time')" />
                <el-table-column prop="ip" :label="$t('serverPages.websiteMonitor.ip')" />
                <el-table-column prop="method" :label="$t('serverPages.websiteMonitor.method')" />
                <el-table-column prop="uri" :label="$t('serverPages.websiteMonitor.uri')" />
                <el-table-column prop="status" :label="$t('serverPages.websiteMonitor.statusCode')" />
                <el-table-column prop="durationMs" :label="$t('serverPages.websiteMonitor.duration')" />
            </el-table>
        </el-card>
        <el-card v-else-if="type === 'websites'" shadow="never">
            <template #header>{{ $t('serverPages.websiteMonitor.websites') }}</template>
            <el-table :data="websites">
                <el-table-column prop="alias" :label="$t('menu.website')" />
                <el-table-column prop="primaryDomain" :label="$t('serverPages.websiteMonitor.domain')" />
                <el-table-column prop="pv" :label="$t('serverPages.websiteMonitor.pv')" />
                <el-table-column prop="uv" :label="$t('serverPages.websiteMonitor.uv')" />
                <el-table-column prop="flow" :label="$t('serverPages.websiteMonitor.flow')" />
                <el-table-column prop="req" :label="$t('serverPages.websiteMonitor.requests')" />
            </el-table>
        </el-card>
        <el-card v-else-if="type === 'setting'" shadow="never">
            <template #header>{{ $t('serverPages.websiteMonitor.setting') }}</template>
            <el-form :model="config" label-width="120px" style="max-width: 680px">
                <el-form-item :label="$t('serverPages.websiteMonitor.monitorSwitch')"><el-switch v-model="config.enabled" /></el-form-item>
                <el-form-item :label="$t('serverPages.websiteMonitor.retentionDays')">
                    <el-input-number v-model="config.storeDays" :min="1" :max="3650" />
                </el-form-item>
                <el-form-item :label="$t('serverPages.websiteMonitor.retentionSize')"><el-input-number v-model="sizeMB" :min="64" /></el-form-item>
                <el-form-item :label="$t('serverPages.websiteMonitor.cdnType')">
                    <el-select v-model="config.cdnType" clearable>
                        <el-option :label="$t('serverPages.websiteMonitor.noCdn')" value="" />
                        <el-option :label="$t('serverPages.websiteMonitor.cloudflare')" value="cloudflare" />
                        <el-option :label="$t('serverPages.websiteMonitor.aliyunCdn')" value="aliyun" />
                        <el-option :label="$t('serverPages.websiteMonitor.tencentCdn')" value="tencent" />
                        <el-option :label="$t('serverPages.websiteMonitor.otherCdn')" value="other" />
                    </el-select>
                </el-form-item>
                <el-form-item :label="$t('serverPages.websiteMonitor.realIPHeader')">
                    <el-input v-model="config.realIPHeader" :placeholder="$t('serverPages.websiteMonitor.cdnHeaderPlaceholder')" />
                </el-form-item>
                <el-form-item :label="$t('serverPages.websiteMonitor.excludeStatus')">
                    <el-input v-model="config.excludeStatus" placeholder="404,499" />
                </el-form-item>
                <el-form-item :label="$t('serverPages.websiteMonitor.excludeExtension')">
                    <el-input v-model="config.excludeExt" placeholder=".css,.js,.png" />
                </el-form-item>
                <el-form-item :label="$t('serverPages.websiteMonitor.excludeURI')"><el-input v-model="config.excludeURI" /></el-form-item>
                <el-form-item :label="$t('serverPages.websiteMonitor.excludeIP')"><el-input v-model="config.excludeIP" /></el-form-item>
                <el-form-item :label="$t('serverPages.websiteMonitor.excludeUA')"><el-input v-model="config.excludeUA" /></el-form-item>
                <el-form-item><el-button type="primary" @click="save">{{ $t('serverPages.websiteMonitor.save') }}</el-button></el-form-item>
            </el-form>
        </el-card>
        <el-card v-else shadow="never">
            <template #header>{{ $t('serverPages.websiteMonitor.trend') }}</template>
            <el-table :data="stats">
                <el-table-column prop="day" :label="$t('serverPages.websiteMonitor.date')" />
                <el-table-column prop="pv" :label="$t('serverPages.websiteMonitor.pv')" />
                <el-table-column prop="uv" :label="$t('serverPages.websiteMonitor.uv')" />
                <el-table-column prop="flow" :label="$t('serverPages.websiteMonitor.flow')" />
                <el-table-column prop="req" :label="$t('serverPages.websiteMonitor.requests')" />
            </el-table>
        </el-card>
    </div>
</template>
<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { ElMessage } from 'element-plus';
import i18n from '@/lang';
import {
    getMonitorConfig,
    monitorLogs,
    monitorRank,
    monitorStat,
    monitorVisitorsLoc,
    monitorWebsites,
    updateMonitorConfig,
} from '@/api/modules/website-monitor';
const props = defineProps<{ type: string }>();
const loading = ref(false);
const stats = ref<any[]>([]);
const rank = ref<any[]>([]);
const logs = ref<any[]>([]);
const websites = ref<any[]>([]);
const locations = ref<any[]>([]);
const rankType = ref('uri');
const sizeMB = ref(1024);
const query = reactive<any>({ page: 1, pageSize: 50 });
const config = reactive<any>({
    enabled: true,
    storeDays: 30,
    storeSize: 1073741824,
    excludeStatus: '',
    excludeExt: '',
    excludeURI: '',
    excludeIP: '',
    excludeUA: '',
    cdnType: '',
    realIPHeader: '',
});
const rankTypes = computed(() => [
                { value: 'uri', label: i18n.global.t('serverPages.websiteMonitor.uri') },
    { value: 'referer', label: i18n.global.t('serverPages.websiteMonitor.referer') },
    { value: 'ip', label: i18n.global.t('serverPages.websiteMonitor.ip') },
    { value: 'browser', label: i18n.global.t('serverPages.websiteMonitor.browser') },
    { value: 'os', label: i18n.global.t('serverPages.websiteMonitor.operatingSystem') },
    { value: 'device', label: i18n.global.t('serverPages.websiteMonitor.device') },
    { value: 'status_code', label: i18n.global.t('serverPages.websiteMonitor.statusCode') },
]);
const cards = computed(() => {
    const item = stats.value[0] || {};
    return [
        { key: 'pv', label: i18n.global.t('serverPages.websiteMonitor.pv'), value: item.pv || 0 },
        { key: 'uv', label: i18n.global.t('serverPages.websiteMonitor.uv'), value: item.uv || 0 },
        { key: 'ip', label: i18n.global.t('serverPages.websiteMonitor.ip'), value: item.ip || 0 },
        { key: 'flow', label: i18n.global.t('serverPages.websiteMonitor.flow'), value: item.flow || 0 },
        { key: 'spider', label: i18n.global.t('serverPages.websiteMonitor.spider'), value: item.spider || 0 },
        { key: 'req', label: i18n.global.t('serverPages.websiteMonitor.requests'), value: item.req || 0 },
    ];
});
async function load() {
    loading.value = true;
    try {
        if (props.type === 'rank') {
            const res: any = await monitorRank({ ...query, type: rankType.value });
            rank.value = res.data || [];
        } else if (props.type === 'log') {
            const res: any = await monitorLogs(query);
            logs.value = res.data?.items || [];
        } else if (props.type === 'websites') {
            const res: any = await monitorWebsites(query);
            websites.value = res.data || [];
        } else if (props.type === 'setting') {
            const res: any = await getMonitorConfig();
            Object.assign(config, res.data || {});
            sizeMB.value = Math.round((config.storeSize || 1073741824) / 1024 / 1024);
        } else {
            const res: any = await monitorStat(query);
            stats.value = res.data || [];
            if (props.type === 'dashboard') {
                const locationRes: any = await monitorVisitorsLoc(query);
                locations.value = locationRes.data || [];
            }
        }
    } finally {
        loading.value = false;
    }
}
async function save() {
    config.storeSize = sizeMB.value * 1024 * 1024;
    await updateMonitorConfig(config);
    ElMessage.success(i18n.global.t('serverPages.websiteMonitor.saved'));
}
onMounted(() => {
    sizeMB.value = Math.round(config.storeSize / 1024 / 1024);
    load();
});
</script>
