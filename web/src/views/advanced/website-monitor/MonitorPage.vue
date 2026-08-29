<template>
    <div v-loading="loading">
        <el-card v-if="type === 'dashboard'" shadow="never">
            <template #header>网站监控概况</template>
            <el-row :gutter="12">
                <el-col v-for="item in cards" :key="item.key" :span="4">
                    <el-statistic :title="item.label" :value="item.value" />
                </el-col>
            </el-row>
            <el-divider />
            <el-row :gutter="16">
                <el-col :span="16">
                    <el-table :data="stats" stripe>
                        <el-table-column prop="day" label="日期" />
                        <el-table-column prop="pv" label="PV" />
                        <el-table-column prop="uv" label="UV" />
                        <el-table-column prop="flow" label="流量" />
                        <el-table-column prop="req" label="请求数" />
                    </el-table>
                </el-col>
                <el-col :span="8">
                    <el-table :data="locations" stripe size="small">
                        <el-table-column prop="name" label="访客地域" />
                        <el-table-column prop="value" label="访问量" width="90" />
                    </el-table>
                </el-col>
            </el-row>
        </el-card>
        <el-card v-else-if="type === 'rank'" shadow="never">
            <template #header>访问统计</template>
            <el-radio-group v-model="rankType" @change="load">
                <el-radio-button v-for="item in rankTypes" :key="item.value" :label="item.value">
                    {{ item.label }}
                </el-radio-button>
            </el-radio-group>
            <el-table :data="rank" class="mt-3">
                <el-table-column type="index" width="60" />
                <el-table-column prop="name" label="名称" />
                <el-table-column prop="value" label="次数" />
            </el-table>
        </el-card>
        <el-card v-else-if="type === 'log'" shadow="never">
            <template #header>请求日志</template>
            <el-form :inline="true">
                <el-form-item label="IP"><el-input v-model="query.ip" clearable /></el-form-item>
                <el-form-item label="URI"><el-input v-model="query.uri" clearable /></el-form-item>
                <el-form-item><el-button type="primary" @click="load">查询</el-button></el-form-item>
            </el-form>
            <el-table :data="logs">
                <el-table-column prop="occurredAt" label="时间" />
                <el-table-column prop="ip" label="IP" />
                <el-table-column prop="method" label="方法" />
                <el-table-column prop="uri" label="URI" />
                <el-table-column prop="status" label="状态码" />
                <el-table-column prop="durationMs" label="耗时(ms)" />
            </el-table>
        </el-card>
        <el-card v-else-if="type === 'websites'" shadow="never">
            <template #header>网站列表</template>
            <el-table :data="websites">
                <el-table-column prop="alias" label="网站" />
                <el-table-column prop="primaryDomain" label="域名" />
                <el-table-column prop="pv" label="PV" />
                <el-table-column prop="uv" label="UV" />
                <el-table-column prop="flow" label="流量" />
                <el-table-column prop="req" label="请求数" />
            </el-table>
        </el-card>
        <el-card v-else-if="type === 'setting'" shadow="never">
            <template #header>监控设置</template>
            <el-form :model="config" label-width="120px" style="max-width: 680px">
                <el-form-item label="监控开关"><el-switch v-model="config.enabled" /></el-form-item>
                <el-form-item label="保存天数">
                    <el-input-number v-model="config.storeDays" :min="1" :max="3650" />
                </el-form-item>
                <el-form-item label="保存大小(MB)"><el-input-number v-model="sizeMB" :min="64" /></el-form-item>
                <el-form-item label="CDN 类型">
                    <el-select v-model="config.cdnType" clearable>
                        <el-option label="无 CDN" value="" />
                        <el-option label="Cloudflare" value="cloudflare" />
                        <el-option label="阿里云 CDN" value="aliyun" />
                        <el-option label="腾讯云 CDN" value="tencent" />
                        <el-option label="其他 CDN" value="other" />
                    </el-select>
                </el-form-item>
                <el-form-item label="真实 IP 请求头">
                    <el-input v-model="config.realIPHeader" placeholder="CF-Connecting-IP 或 X-Forwarded-For" />
                </el-form-item>
                <el-form-item label="排除状态码">
                    <el-input v-model="config.excludeStatus" placeholder="404,499" />
                </el-form-item>
                <el-form-item label="排除扩展名">
                    <el-input v-model="config.excludeExt" placeholder=".css,.js,.png" />
                </el-form-item>
                <el-form-item label="排除 URI"><el-input v-model="config.excludeURI" /></el-form-item>
                <el-form-item label="排除 IP"><el-input v-model="config.excludeIP" /></el-form-item>
                <el-form-item label="排除 UA"><el-input v-model="config.excludeUA" /></el-form-item>
                <el-form-item><el-button type="primary" @click="save">保存</el-button></el-form-item>
            </el-form>
        </el-card>
        <el-card v-else shadow="never">
            <template #header>趋势统计</template>
            <el-table :data="stats">
                <el-table-column prop="day" label="日期" />
                <el-table-column prop="pv" label="PV" />
                <el-table-column prop="uv" label="UV" />
                <el-table-column prop="flow" label="流量" />
                <el-table-column prop="req" label="请求数" />
            </el-table>
        </el-card>
    </div>
</template>
<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { ElMessage } from 'element-plus';
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
const rankTypes = [
    { value: 'uri', label: 'URI' },
    { value: 'referer', label: 'Referer' },
    { value: 'ip', label: 'IP' },
    { value: 'browser', label: '浏览器' },
    { value: 'os', label: '操作系统' },
    { value: 'device', label: '设备' },
    { value: 'status_code', label: '状态码' },
];
const cards = computed(() => {
    const item = stats.value[0] || {};
    return [
        { key: 'pv', label: 'PV', value: item.pv || 0 },
        { key: 'uv', label: 'UV', value: item.uv || 0 },
        { key: 'ip', label: 'IP', value: item.ip || 0 },
        { key: 'flow', label: '流量', value: item.flow || 0 },
        { key: 'spider', label: '蜘蛛', value: item.spider || 0 },
        { key: 'req', label: '请求数', value: item.req || 0 },
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
    ElMessage.success('监控设置已保存');
}
onMounted(() => {
    sizeMB.value = Math.round(config.storeSize / 1024 / 1024);
    load();
});
</script>
