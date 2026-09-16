<template>
    <div class="monitor-dashboard" v-loading="loading">
        <div class="monitor-notice">
            <span>
                {{ monitorEnabled ? $t('xpack.monitor.monitorEnabled') : $t('xpack.monitor.monitorNotEnabled') }}
            </span>
            <el-switch v-model="monitorEnabled" @change="toggleMonitor" />
        </div>

        <section class="website-picker panel">
            <span class="picker-label">{{ $t('menu.website') }}</span>
            <el-select
                v-model="selectedWebsiteID"
                clearable
                filterable
                :disabled="!websites.length"
                :placeholder="$t('serverPages.websiteMonitor.allWebsites')"
                @change="loadDashboard"
            >
                <el-option
                    v-for="website in websites"
                    :key="website.id"
                    :label="website.primaryDomain || website.alias"
                    :value="website.id"
                />
            </el-select>
        </section>

        <section class="panel status-panel">
            <h3>
                <i />
                {{ $t('xpack.waf.todayStatus') }}
            </h3>
            <div class="metric-grid">
                <div v-for="item in statusCards" :key="item.key" class="metric">
                    <span>{{ item.label }}</span>
                    <strong>{{ item.value }}</strong>
                </div>
            </div>
        </section>

        <div class="middle-grid">
            <section class="panel visitor-map-panel">
                <h3>
                    <i />
                    {{ $t('xpack.monitor.uvMap') }}
                </h3>
                <div class="visitor-map-content">
                    <div class="map-stage">
                        <div ref="mapRef" class="map-canvas" />
                        <div class="map-legend">
                            <span>{{ $t('xpack.waf.low') }}</span>
                            <b />
                            <span>{{ $t('xpack.waf.hight') }}</span>
                            <em />
                            {{ $t('xpack.monitor.visitors') }}
                        </div>
                    </div>
                    <div class="source-rail">
                        <el-radio-group v-model="mapMode" size="small">
                            <el-radio-button label="world">{{ $t('xpack.waf.world') }}</el-radio-button>
                            <el-radio-button label="china">{{ $t('xpack.waf.china') }}</el-radio-button>
                        </el-radio-group>
                        <div class="source-table">
                            <div class="source-table-header">
                                <span>{{ $t('xpack.monitor.source') }}</span>
                                <span>{{ $t('xpack.monitor.quantity') }}</span>
                            </div>
                            <el-empty v-if="!locations.length" :description="$t('commons.noneData')" :image-size="42" />
                            <div v-for="item in locations" :key="item.name" class="source-row">
                                <span>{{ item.name }}</span>
                                <strong>{{ item.value }}</strong>
                            </div>
                        </div>
                    </div>
                </div>
            </section>

            <div class="realtime-column">
                <section class="panel realtime-panel">
                    <h3>
                        <i />
                        {{ $t('xpack.monitor.qps') }}
                    </h3>
                    <strong>{{ realtime.qps }}</strong>
                </section>
                <section class="panel realtime-panel">
                    <h3>
                        <i />
                        {{ $t('xpack.monitor.flowSec') }}
                    </h3>
                    <strong>{{ formatBytes(realtime.flow) }}</strong>
                </section>
            </div>
        </div>

        <section class="panel trend-panel">
            <div class="section-heading">
                <h3>
                    <i />
                    {{ $t('xpack.monitor.visitors') }}
                </h3>
                <el-radio-group v-model="trendRange" size="small">
                    <el-radio-button label="today">{{ $t('xpack.monitor.today') }}</el-radio-button>
                    <el-radio-button label="7">{{ $t('xpack.monitor.last7days') }}</el-radio-button>
                    <el-radio-button label="30">{{ $t('xpack.monitor.last30days') }}</el-radio-button>
                </el-radio-group>
            </div>
            <div ref="trendRef" class="trend-canvas" />
        </section>
    </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { ElMessage } from 'element-plus';
import i18n from '@/lang';
import { listWebsites } from '@/api/modules/website';
import {
    getMonitorConfig,
    monitorQPS,
    monitorStat,
    monitorVisitorsLoc,
    updateMonitorConfig,
    type MonitorConfig,
    type MonitorDailyStat,
    type MonitorVisitorLocation,
} from '@/api/modules/website-monitor';
import chinaMap from '@/assets/json/china.json';
import worldMap from '@/assets/json/world.json';
import echarts from '@/utils/echarts';

type WebsiteItem = { id: number; primaryDomain?: string; alias?: string };
type MapMode = 'world' | 'china';

const loading = ref(false);
const websites = ref<WebsiteItem[]>([]);
const selectedWebsiteID = ref<number>();
const todayStats = ref<MonitorDailyStat[]>([]);
const trendStats = ref<MonitorDailyStat[]>([]);
const locations = ref<MonitorVisitorLocation[]>([]);
const trendRange = ref('today');
const mapMode = ref<MapMode>('world');
const monitorEnabled = ref(false);
const mapRef = ref<HTMLElement>();
const trendRef = ref<HTMLElement>();
const realtime = reactive({ qps: 0, flow: 0 });
const config = reactive<MonitorConfig>({
    enabled: false,
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
let mapChart: any;
let trendChart: any;
let loadingRequest = 0;
const t = (key: string) => String(i18n.global.t(key));

const status = computed(() => {
    const summary = todayStats.value.reduce(
        (result, item) => ({
            pv: result.pv + Number(item.pv || 0),
            uv: result.uv + Number(item.uv || 0),
            ip: result.ip + Number(item.ip || 0),
            flow: result.flow + Number(item.flow || 0),
            spider: result.spider + Number(item.spider || 0),
            req: result.req + Number(item.req || 0),
            count4xx: result.count4xx + Number(item.count4xx || 0),
            count5xx: result.count5xx + Number(item.count5xx || 0),
        }),
        { pv: 0, uv: 0, ip: 0, flow: 0, spider: 0, req: 0, count4xx: 0, count5xx: 0 },
    );
    return summary;
});
const statusCards = computed(() => [
    { key: 'pv', label: t('xpack.monitor.pv'), value: status.value.pv },
    { key: 'uv', label: t('xpack.monitor.uv'), value: status.value.uv },
    { key: 'ip', label: t('xpack.monitor.ip'), value: status.value.ip },
    { key: 'flow', label: t('xpack.monitor.flow'), value: formatBytes(status.value.flow) },
    { key: 'spider', label: t('xpack.monitor.spider'), value: status.value.spider },
    { key: 'req', label: t('xpack.monitor.reqCount'), value: status.value.req },
    { key: '4xx', label: t('xpack.waf.count4xx'), value: status.value.count4xx },
    { key: '5xx', label: t('xpack.waf.count5xx'), value: status.value.count5xx },
]);

const dateText = (value: Date) => {
    const year = value.getFullYear();
    const month = String(value.getMonth() + 1).padStart(2, '0');
    const day = String(value.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
};
const range = (days: number) => {
    const end = new Date();
    const start = new Date(end);
    start.setHours(0, 0, 0, 0);
    start.setDate(start.getDate() - days + 1);
    const endExclusive = new Date(end);
    endExclusive.setHours(0, 0, 0, 0);
    endExclusive.setDate(endExclusive.getDate() + 1);
    return { startTime: dateText(start), endTime: dateText(endExclusive) };
};
const minuteRange = () => {
    const end = new Date();
    const start = new Date(end.getTime() - 60 * 1000);
    return { startTime: start.toISOString(), endTime: end.toISOString() };
};
const formatBytes = (value: number) => {
    const bytes = Number(value || 0);
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
    return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`;
};
const query = (days: number) => ({ websiteID: selectedWebsiteID.value, ...range(days) });
const loadConfig = async () => {
    const result: any = await getMonitorConfig(selectedWebsiteID.value || 0);
    Object.assign(config, result.data || {});
    monitorEnabled.value = config.enabled !== false;
};
const loadDashboard = async () => {
    const requestID = ++loadingRequest;
    loading.value = true;
    try {
        await loadConfig();
        const [statResult, locationResult, qpsResult] = await Promise.all([
            monitorStat(query(1)),
            monitorVisitorsLoc(query(30)),
            monitorQPS({ websiteID: selectedWebsiteID.value, ...minuteRange() }),
        ]);
        if (requestID !== loadingRequest) return;
        todayStats.value = statResult.data || [];
        trendStats.value = todayStats.value;
        locations.value = (locationResult.data || []).slice(0, 12);
        realtime.qps = Number(qpsResult.data?.qps || 0);
        realtime.flow = Number(qpsResult.data?.flow || 0);
        await renderCharts();
    } catch (error: any) {
        if (requestID === loadingRequest) {
            todayStats.value = [];
            trendStats.value = [];
            locations.value = [];
            realtime.qps = 0;
            realtime.flow = 0;
            ElMessage.error(error?.message || 'Website monitoring data could not be loaded');
        }
    } finally {
        if (requestID === loadingRequest) loading.value = false;
    }
};
const loadTrend = async () => {
    try {
        const days = trendRange.value === 'today' ? 1 : Number(trendRange.value);
        const result: any = await monitorStat(query(days));
        trendStats.value = trendRange.value === 'today' ? todayStats.value : result.data || [];
        await nextTick();
        renderTrend();
    } catch (error: any) {
        ElMessage.error(error?.message || 'Website visitor trend could not be loaded');
    }
};
const toggleMonitor = async (enabled: boolean | string | number) => {
    const previous = config.enabled;
    config.enabled = Boolean(enabled);
    try {
        await updateMonitorConfig({ ...config, websiteID: selectedWebsiteID.value });
        await loadConfig();
    } catch (error: any) {
        config.enabled = previous;
        monitorEnabled.value = previous;
        ElMessage.error(error?.message || 'Website monitoring settings could not be saved');
    }
};
const mapData = computed(() => {
    const source = mapMode.value === 'world' ? worldMap : chinaMap;
    const names = new Set(
        ((source as any).features || [])
            .map((feature: any) => String(feature.properties?.name || '').trim())
            .filter(Boolean),
    );
    return locations.value
        .filter((item) => names.has(item.name))
        .map((item) => ({ name: item.name, value: item.value }));
});
const renderMap = () => {
    if (!mapRef.value) return;
    const source = mapMode.value === 'world' ? worldMap : chinaMap;
    if (!echarts.getMap(mapMode.value)) echarts.registerMap(mapMode.value, source as any);
    mapChart = mapChart || echarts.init(mapRef.value);
    const max = Math.max(...mapData.value.map((item) => item.value), 1);
    mapChart.setOption(
        {
            animation: false,
            tooltip: { trigger: 'item' },
            visualMap: { show: false, min: 0, max, inRange: { color: ['#f1f3f5', '#1677ff'] } },
            series: [
                {
                    type: 'map',
                    map: mapMode.value,
                    roam: false,
                    selectedMode: false,
                    data: mapData.value,
                    itemStyle: { areaColor: '#f1f3f5', borderColor: '#e2e6eb', borderWidth: 1 },
                    emphasis: { label: { show: false }, itemStyle: { areaColor: '#73a8f8' } },
                },
            ],
        },
        true,
    );
};
const renderTrend = () => {
    if (!trendRef.value) return;
    trendChart = trendChart || echarts.init(trendRef.value);
    trendChart.setOption(
        {
            animation: false,
            grid: { left: 45, right: 18, top: 18, bottom: 28 },
            tooltip: { trigger: 'axis' },
            xAxis: {
                type: 'category',
                boundaryGap: false,
                data: trendStats.value.map((item) => item.day.slice(5)),
                axisLine: { lineStyle: { color: '#dcdfe6' } },
                axisLabel: { color: '#909399' },
            },
            yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: '#ebeef5' } } },
            series: [
                {
                    name: t('xpack.monitor.visitors'),
                    type: 'line',
                    smooth: true,
                    showSymbol: false,
                    data: trendStats.value.map((item) => item.uv),
                    lineStyle: { color: '#1677ff', width: 2 },
                    areaStyle: { color: '#1677ff', opacity: 0.08 },
                },
            ],
        },
        true,
    );
};
const renderCharts = async () => {
    await nextTick();
    renderMap();
    renderTrend();
};
const resizeCharts = () => {
    mapChart?.resize();
    trendChart?.resize();
};
const loadWebsites = async () => {
    const result: any = await listWebsites();
    websites.value = (result.data || []).map((item: any) => ({
        id: Number(item.id),
        primaryDomain: item.primaryDomain,
        alias: item.alias,
    }));
    if (!selectedWebsiteID.value && websites.value.length) selectedWebsiteID.value = websites.value[0].id;
};

watch(mapMode, () => nextTick(renderMap));
watch(trendRange, loadTrend);
watch(selectedWebsiteID, () => loadDashboard());
onMounted(async () => {
    try {
        await loadWebsites();
        await loadDashboard();
    } catch (error: any) {
        ElMessage.error(error?.message || 'Website list could not be loaded');
    }
    window.addEventListener('resize', resizeCharts);
});
onBeforeUnmount(() => {
    window.removeEventListener('resize', resizeCharts);
    mapChart?.dispose();
    trendChart?.dispose();
});
</script>

<style scoped>
.monitor-dashboard {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
}
.panel {
    min-width: 0;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-light);
    border-radius: 4px;
}
.monitor-notice {
    min-height: 42px;
    box-sizing: border-box;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 0 14px;
    color: #e6a23c;
    background: #fdf6ec;
    font-size: 13px;
}
.monitor-notice :deep(.el-switch) {
    --el-switch-on-color: #1677ff;
}
.website-picker {
    min-height: 52px;
    box-sizing: border-box;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 12px;
}
.picker-label {
    color: var(--el-text-color-secondary);
}
.website-picker :deep(.el-select) {
    width: 270px;
}
.status-panel {
    min-height: 146px;
    box-sizing: border-box;
    padding: 20px 22px 14px;
}
h3 {
    margin: 0 0 20px;
    color: var(--el-text-color-primary);
    font-size: 15px;
    font-weight: 500;
}
h3 i {
    display: inline-block;
    width: 4px;
    height: 16px;
    margin-right: 9px;
    vertical-align: -3px;
    background: var(--el-color-primary);
    border-radius: 2px;
}
.metric-grid {
    display: grid;
    grid-template-columns: repeat(8, minmax(0, 1fr));
    gap: 8px;
}
.metric {
    min-width: 0;
    text-align: center;
    color: var(--el-text-color-secondary);
    font-size: 13px;
}
.metric span {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.metric strong {
    display: block;
    margin-top: 10px;
    color: var(--el-color-primary);
    font-size: 24px;
    font-weight: 500;
}
.middle-grid {
    display: grid;
    grid-template-columns: minmax(0, 3fr) minmax(270px, 1fr);
    gap: 8px;
}
.visitor-map-panel {
    min-height: 542px;
    box-sizing: border-box;
    padding: 20px 22px 12px;
}
.visitor-map-content {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 170px;
    gap: 10px;
    min-height: 478px;
}
.map-stage {
    display: flex;
    min-width: 0;
    flex-direction: column;
    justify-content: flex-end;
}
.map-canvas {
    width: 100%;
    min-height: 420px;
}
.map-legend {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    margin-top: 10px;
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.map-legend b {
    display: block;
    width: 126px;
    height: 12px;
    border-radius: 2px;
    background: linear-gradient(90deg, #edf1f5, #1677ff);
}
.map-legend em {
    width: 12px;
    height: 12px;
    margin-left: 28px;
    background: #1677ff;
    border-radius: 2px;
}
.source-rail {
    min-width: 0;
    padding-top: 20px;
}
.source-rail :deep(.el-radio-group) {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 10px;
}
.source-table {
    border: 1px solid var(--el-border-color-lighter);
    min-height: 88px;
}
.source-table-header {
    display: flex;
    justify-content: space-between;
    padding: 8px 10px;
    color: #fff;
    background: var(--el-color-primary);
    font-size: 13px;
    font-weight: 500;
}
.source-table :deep(.el-empty) {
    padding: 14px 0;
}
.source-table :deep(.el-empty__description) {
    margin-top: 0;
}
.source-row {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    padding: 9px 10px;
    color: var(--el-text-color-regular);
    font-size: 12px;
}
.source-row span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.source-row strong {
    color: var(--el-text-color-secondary);
    font-weight: 400;
}
.realtime-column {
    display: grid;
    grid-template-rows: 1fr 1fr;
    gap: 8px;
}
.realtime-panel {
    min-height: 267px;
    box-sizing: border-box;
    padding: 20px 22px;
}
.realtime-panel strong {
    display: block;
    margin-top: 62px;
    color: var(--el-color-primary);
    font-size: 42px;
    font-weight: 400;
    text-align: center;
}
.trend-panel {
    min-height: 454px;
    box-sizing: border-box;
    padding: 20px 22px 12px;
}
.section-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
}
.section-heading h3 {
    margin-bottom: 0;
}
.trend-canvas {
    width: 100%;
    height: 382px;
}
@media (max-width: 900px) {
    .metric-grid {
        grid-template-columns: repeat(4, minmax(0, 1fr));
        row-gap: 18px;
    }
    .middle-grid {
        grid-template-columns: 1fr;
    }
    .visitor-map-panel,
    .realtime-panel {
        min-height: auto;
    }
    .realtime-column {
        grid-template-columns: 1fr 1fr;
        grid-template-rows: auto;
    }
    .realtime-panel strong {
        margin: 28px 0 8px;
    }
}
@media (max-width: 600px) {
    .metric-grid {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .visitor-map-content {
        grid-template-columns: 1fr;
    }
    .map-canvas {
        min-height: 280px;
    }
    .source-rail {
        padding-top: 0;
    }
    .realtime-column {
        grid-template-columns: 1fr;
    }
    .section-heading {
        align-items: stretch;
        flex-direction: column;
    }
    .section-heading :deep(.el-radio-group) {
        align-self: flex-end;
    }
}
</style>
