<template>
    <div class="overview-page" v-loading="loading">
        <div class="overview-layout">
            <div class="overview-left">
                <section class="panel status-panel">
                    <h3>
                        <i />
                        {{ $t('xpack.waf.todayStatus') }}
                    </h3>
                    <el-row :gutter="8">
                        <el-col v-for="item in statusCards" :key="item.key" :xs="12" :sm="6" class="metric">
                            <span>{{ item.label }}</span>
                            <strong>{{ item.value }}</strong>
                        </el-col>
                    </el-row>
                </section>
                <section class="panel source-panel">
                    <h3>
                        <i />
                        拦截地图（30日）
                    </h3>
                    <div class="source-content">
                        <div class="source-visual">
                            <div ref="sourceMapRef" class="source-map" />
                            <div class="source-legend">
                                <span>低</span>
                                <b />
                                <span>高</span>
                                <em />
                                拦截数
                            </div>
                        </div>
                        <div class="source-rail">
                            <el-radio-group v-model="sourceMode" size="small" class="source-switch">
                                <el-radio-button label="world">世界</el-radio-button>
                                <el-radio-button label="china">中国</el-radio-button>
                            </el-radio-group>
                            <div class="source-table">
                                <div class="source-table-header">
                                    <span>来源</span>
                                    <span>数量</span>
                                </div>
                                <el-empty v-if="!overview.sources.length" description="暂无数据" :image-size="52" />
                                <div v-for="item in overview.sources" :key="item.source" class="source-row">
                                    <span>{{ item.source }}</span>
                                    <strong>{{ item.count }}</strong>
                                </div>
                            </div>
                        </div>
                    </div>
                </section>
            </div>
            <div class="overview-right">
                <section class="panel chart-panel">
                    <h3>
                        <i />
                        {{ $t('xpack.waf.requestTrends') }}
                    </h3>
                    <div ref="requestChartRef" class="trend-chart" />
                </section>
                <section class="panel chart-panel">
                    <h3>
                        <i />
                        {{ $t('xpack.waf.interceptTrends') }}
                    </h3>
                    <div ref="interceptChartRef" class="trend-chart" />
                </section>
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { getWafOverview, type WafOverview } from '@/api/modules/waf';
import chinaMap from '@/assets/json/china.json';
import worldMap from '@/assets/json/world.json';
import echarts from '@/utils/echarts';

const loading = ref(false);
const sourceMode = ref<'world' | 'china'>('china');
const sourceMapRef = ref<HTMLElement>();
const requestChartRef = ref<HTMLElement>();
const interceptChartRef = ref<HTMLElement>();
let sourceChart: any;
let requestChart: any;
let interceptChart: any;
const overview = reactive<WafOverview>({
    today: { requests: 0, intercepts: 0, count4xx: 0, count5xx: 0 },
    requestTrend: [],
    interceptTrend: [],
    sources: [],
});
const statusCards = computed(() => [
    { key: 'requests', label: '请求', value: overview.today.requests },
    { key: 'intercepts', label: '拦截', value: overview.today.intercepts },
    { key: '4xx', label: '4xx 数量', value: overview.today.count4xx },
    { key: '5xx', label: '5xx 数量', value: overview.today.count5xx },
]);
type MapFeatureCollection = {
    features?: Array<{ properties?: { name?: string } }>;
};

const mapSources: Record<'world' | 'china', MapFeatureCollection> = {
    world: worldMap as MapFeatureCollection,
    china: chinaMap as MapFeatureCollection,
};
const chinaSourceAliases: Record<string, string> = {
    北京: 'Beijing',
    北京市: 'Beijing',
    天津: 'Tianjin',
    天津市: 'Tianjin',
    河北: 'Hebei',
    河北省: 'Hebei',
    山西: 'Shanxi',
    山西省: 'Shanxi',
    内蒙古: 'Inner Mongolia',
    内蒙古自治区: 'Inner Mongolia',
    辽宁: 'Liaoning',
    辽宁省: 'Liaoning',
    吉林: 'Jilin',
    吉林省: 'Jilin',
    黑龙江: 'Heilongjiang',
    黑龙江省: 'Heilongjiang',
    上海: 'Shanghai',
    上海市: 'Shanghai',
    江苏: 'Jiangsu',
    江苏省: 'Jiangsu',
    浙江: 'Zhejiang',
    浙江省: 'Zhejiang',
    安徽: 'Anhui',
    安徽省: 'Anhui',
    福建: 'Fujian',
    福建省: 'Fujian',
    江西: 'Jiangxi',
    江西省: 'Jiangxi',
    山东: 'Shandong',
    山东省: 'Shandong',
    河南: 'Henan',
    河南省: 'Henan',
    湖北: 'Hubei',
    湖北省: 'Hubei',
    湖南: 'Hunan',
    湖南省: 'Hunan',
    广东: 'Guangdong',
    广东省: 'Guangdong',
    广西: 'Guangxi',
    广西壮族自治区: 'Guangxi',
    海南: 'Hainan',
    海南省: 'Hainan',
    重庆: 'Chongqing',
    重庆市: 'Chongqing',
    四川: 'Sichuan',
    四川省: 'Sichuan',
    贵州: 'Guizhou',
    贵州省: 'Guizhou',
    云南: 'Yunnan',
    云南省: 'Yunnan',
    西藏: 'Tibet',
    西藏自治区: 'Tibet',
    陕西: 'Shaanxi',
    陕西省: 'Shaanxi',
    甘肃: 'Gansu',
    甘肃省: 'Gansu',
    青海: 'Qinghai',
    青海省: 'Qinghai',
    宁夏: 'Ningxia',
    宁夏回族自治区: 'Ningxia',
    新疆: 'Xinjiang',
    新疆维吾尔自治区: 'Xinjiang',
    台湾: 'Taiwan',
    台湾省: 'Taiwan',
    香港: 'HongKong',
    香港特别行政区: 'HongKong',
    'Hong Kong': 'HongKong',
    澳门: 'Macao',
    澳门特别行政区: 'Macao',
    Macau: 'Macao',
};
const normalizeMapName = (name: string) =>
    name
        .trim()
        .toLocaleLowerCase()
        .replace(/[\s._-]/g, '');
const mapFeatureNames = computed(() => {
    const names = new Map<string, string>();
    for (const feature of mapSources[sourceMode.value].features || []) {
        const name = feature.properties?.name?.trim();
        if (name) {
            names.set(normalizeMapName(name), name);
        }
    }
    return names;
});
const resolveMapName = (source: string) => {
    const trimmed = source.trim();
    const alias = chinaSourceAliases[trimmed] || trimmed;
    return mapFeatureNames.value.get(normalizeMapName(alias));
};
const sourceMapData = computed(() => {
    const counts = new Map<string, number>();
    for (const item of overview.sources) {
        const name = resolveMapName(item.source);
        if (name) {
            counts.set(name, (counts.get(name) || 0) + item.count);
        }
    }
    return Array.from(counts, ([name, value]) => ({ name, value }));
});
const fallbackTrend = () => {
    const today = new Date();
    return Array.from({ length: 7 }, (_, index) => {
        const date = new Date(today);
        date.setDate(today.getDate() - (6 - index));
        return { day: date.toISOString().slice(0, 10), count: 0 };
    });
};
const sourceMapOption = () => {
    const max = Math.max(...sourceMapData.value.map((item) => item.value), 1);
    return {
        animation: false,
        tooltip: {
            trigger: 'item',
            formatter: (params: { name?: string; value?: number }) =>
                `${params.name || ''}<br/>拦截数：${params.value || 0}`,
        },
        visualMap: {
            show: false,
            min: 0,
            max,
            inRange: { color: ['#f4f6f8', '#1677ff'] },
        },
        series: [
            {
                type: 'map',
                map: sourceMode.value,
                name: '拦截数',
                roam: false,
                selectedMode: false,
                data: sourceMapData.value,
                itemStyle: {
                    areaColor: '#f4f6f8',
                    borderColor: '#e4e7ed',
                    borderWidth: 1,
                },
                emphasis: {
                    itemStyle: { areaColor: '#79adff' },
                    label: { show: false },
                },
            },
        ],
    };
};
const chartOption = (items: Array<{ day: string; count: number }>, color: string) => ({
    animation: false,
    grid: { left: 42, right: 16, top: 18, bottom: 28 },
    tooltip: { trigger: 'axis' },
    xAxis: {
        type: 'category',
        boundaryGap: false,
        data: items.map((item) => item.day.slice(5)),
        axisLabel: { color: '#606266' },
        axisLine: { lineStyle: { color: '#dcdfe6' } },
    },
    yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: '#ebeef5' } } },
    series: [
        {
            type: 'line',
            smooth: true,
            showSymbol: false,
            data: items.map((item) => item.count),
            lineStyle: { color, width: 2 },
            areaStyle: { color, opacity: 0.08 },
        },
    ],
});
const renderCharts = () => {
    if (requestChartRef.value) {
        requestChart = requestChart || echarts.init(requestChartRef.value);
        requestChart.setOption(chartOption(overview.requestTrend.length ? overview.requestTrend : fallbackTrend(), '#1677ff'));
    }
    if (interceptChartRef.value) {
        interceptChart = interceptChart || echarts.init(interceptChartRef.value);
        interceptChart.setOption(
            chartOption(overview.interceptTrend.length ? overview.interceptTrend : fallbackTrend(), '#1677ff'),
        );
    }
};
const renderSourceMap = () => {
    if (!sourceMapRef.value) {
        return;
    }
    if (!echarts.getMap(sourceMode.value)) {
        echarts.registerMap(sourceMode.value, mapSources[sourceMode.value] as any);
    }
    sourceChart = sourceChart || echarts.init(sourceMapRef.value);
    sourceChart.setOption(sourceMapOption(), true);
};
const renderAllCharts = () => {
    renderSourceMap();
    renderCharts();
};
const load = async () => {
    loading.value = true;
    try {
        const result: any = await getWafOverview();
        const data = result.data as WafOverview;
        Object.assign(overview, {
            today: data?.today || overview.today,
            requestTrend: data?.requestTrend || [],
            interceptTrend: data?.interceptTrend || [],
            sources: data?.sources || [],
        });
        await nextTick();
        renderAllCharts();
    } finally {
        loading.value = false;
    }
};
const resize = () => {
    sourceChart?.resize();
    requestChart?.resize();
    interceptChart?.resize();
};
watch(sourceMode, async () => {
    await nextTick();
    renderSourceMap();
});
onMounted(() => {
    load();
    window.addEventListener('resize', resize);
    window.addEventListener('workmesh:waf-refresh', load);
});
onBeforeUnmount(() => {
    window.removeEventListener('resize', resize);
    window.removeEventListener('workmesh:waf-refresh', load);
    sourceChart?.dispose();
    requestChart?.dispose();
    interceptChart?.dispose();
});
</script>

<style scoped>
.overview-page {
    min-width: 0;
}
.overview-layout {
    display: grid;
    grid-template-columns: minmax(0, 2fr) minmax(350px, 1fr);
    gap: 8px;
}
.overview-left,
.overview-right {
    min-width: 0;
}
.panel {
    min-width: 0;
    padding: 18px 14px 12px;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-light);
    border-radius: 4px;
}
.status-panel {
    height: 178px;
    margin-bottom: 8px;
}
.panel h3 {
    margin: 0 0 22px;
    font-size: 16px;
    font-weight: 500;
    color: var(--el-text-color-primary);
}
.panel h3 i {
    display: inline-block;
    width: 4px;
    height: 18px;
    margin-right: 10px;
    vertical-align: -3px;
    background: var(--el-color-primary);
    border-radius: 2px;
}
.overview-page :deep(.el-empty__image) {
    display: none;
}
.metric {
    text-align: center;
    color: var(--el-text-color-secondary);
}
.metric strong {
    display: block;
    margin-top: 12px;
    color: var(--el-color-primary);
    font-size: 28px;
    font-weight: 500;
}
.source-panel {
    min-height: 690px;
}
.source-content {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 214px;
    gap: 14px;
    min-height: 600px;
}
.source-visual {
    display: flex;
    min-width: 0;
    flex-direction: column;
    justify-content: flex-end;
}
.source-map {
    width: 100%;
    min-height: 490px;
}
.source-rail {
    min-width: 0;
    padding-top: 16px;
}
.source-switch {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 22px;
}
.source-legend {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    margin-top: 20px;
    color: var(--el-text-color-secondary);
    font-size: 13px;
}
.source-legend b {
    width: 154px;
    height: 16px;
    background: linear-gradient(90deg, #eef3f9, #1677ff);
    border-radius: 3px;
}
.source-legend em {
    width: 28px;
    height: 14px;
    margin-left: 40px;
    background: #1677ff;
    border-radius: 3px;
}
.source-table {
    align-self: start;
    border: 1px solid var(--el-border-color-lighter);
}
.source-table-header {
    display: grid;
    grid-template-columns: 1fr 70px;
    padding: 13px 12px;
    color: #fff;
    background: var(--el-color-primary);
    font-weight: 600;
}
.source-row {
    display: grid;
    grid-template-columns: 1fr 70px;
    padding: 11px 12px;
    border-top: 1px solid var(--el-border-color-lighter);
    color: var(--el-text-color-regular);
}
.source-row strong {
    text-align: right;
    color: var(--el-text-color-primary);
}
.chart-panel {
    height: 440px;
    margin-bottom: 8px;
}
.chart-panel:last-child {
    margin-bottom: 0;
}
.trend-chart {
    width: 100%;
    height: 350px;
}
@media (max-width: 900px) {
    .overview-layout {
        grid-template-columns: 1fr;
    }
    .source-panel {
        min-height: 0;
    }
    .source-content {
        grid-template-columns: 1fr;
        min-height: 0;
    }
    .source-map {
        min-height: 280px;
    }
    .source-rail {
        padding-top: 0;
    }
    .source-table {
        width: 100%;
    }
    .chart-panel {
        height: 380px;
    }
    .trend-chart {
        height: 300px;
    }
}
</style>
