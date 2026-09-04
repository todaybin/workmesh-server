<template>
    <div class="custom-row">
        <el-col :xs="6" :sm="6" :md="3" :lg="3" :xl="3" align="center">
            <el-popover
                :hide-after="20"
                :teleported="false"
                :width="320"
                v-if="chartsOption['load']"
            >
                <el-descriptions :column="1" size="small">
                    <el-descriptions-item :label="$t('home.loadAverage', [1])">
                        {{ formatNumber(currentInfo.load1) }}
                    </el-descriptions-item>
                    <el-descriptions-item :label="$t('home.loadAverage', [5])">
                        {{ formatNumber(currentInfo.load5) }}
                    </el-descriptions-item>
                    <el-descriptions-item :label="$t('home.loadAverage', [15])">
                        {{ formatNumber(currentInfo.load15) }}
                    </el-descriptions-item>
                </el-descriptions>

                <template #reference>
                    <v-charts
                        height="160px"
                        id="load"
                        type="pie"
                        :option="chartsOption['load']"
                        v-if="chartsOption['load']"
                    />
                </template>
            </el-popover>
            <span class="input-help">{{ loadStatus(currentInfo.loadUsagePercent) }}</span>
        </el-col>
        <el-col :xs="6" :sm="6" :md="3" :lg="3" :xl="3">
            <el-popover
                :hide-after="20"
                :teleported="false"
                :width="430"
                v-if="chartsOption['cpu']"
            >
                <el-descriptions :title="baseInfo.cpuModelName" class="ml-1" :column="3" size="small">
                    <el-descriptions-item :label="$t('home.core')">
                        {{ baseInfo.cpuCores }}
                    </el-descriptions-item>
                    <el-descriptions-item :label="$t('home.logicCore')">
                        {{ baseInfo.cpuLogicalCores }}
                    </el-descriptions-item>
                    <el-descriptions-item v-if="baseInfo.cpuMhz > 0" :label="$t('home.cpuFrequency')">
                        {{ formatNumber(baseInfo.cpuMhz) }} MHz
                    </el-descriptions-item>
                </el-descriptions>

                <el-space wrap :size="5" class="ml-1">
                    <template v-for="(item, index) of currentInfo.cpuPercent" :key="index">
                        <div class="cpu-detail" v-if="index < 8">
                            CPU-{{ index }}: {{ formatNumber(item) }}%
                        </div>
                    </template>
                </el-space>
                <template #reference>
                    <v-charts
                        height="160px"
                        id="cpu"
                        type="pie"
                        :option="chartsOption['cpu']"
                        v-if="chartsOption['cpu']"
                    />
                </template>
            </el-popover>
            <div class="text-center">
                <span class="input-help">
                    ( {{ formatNumber(currentInfo.cpuUsed) }} / {{ currentInfo.cpuTotal }} )
                    {{ $t('commons.units.core', currentInfo.cpuTotal) }}
                </span>
            </div>
        </el-col>
        <el-col :xs="6" :sm="6" :md="3" :lg="3" :xl="3" align="center">
            <el-popover
                :hide-after="20"
                :teleported="false"
                :width="480"
                v-if="chartsOption['memory']"
            >
                <el-descriptions direction="vertical" :title="$t('home.mem')" class="ml-1" :column="4" size="small">
                    <el-descriptions-item :label-width="60" :label="$t('home.total')">
                        {{ computeSize(currentInfo.memoryTotal) }}
                    </el-descriptions-item>
                    <el-descriptions-item :label="$t('home.used')">
                        {{ computeSize(currentInfo.memoryUsed) }}
                    </el-descriptions-item>
                    <el-descriptions-item :label="$t('home.free')">
                        {{ computeSize(currentInfo.memoryFree) }}
                    </el-descriptions-item>
                    <el-descriptions-item :label="$t('home.available')">
                        {{ computeSize(currentInfo.memoryAvailable) }}
                    </el-descriptions-item>
                    <el-descriptions-item :label="$t('home.shard')">
                        {{ computeSize(currentInfo.memoryShard) }}
                    </el-descriptions-item>
                    <el-descriptions-item :label="$t('home.cache')">
                        {{ computeSize(currentInfo.memoryCache) }}
                    </el-descriptions-item>
                    <el-descriptions-item :label="$t('home.percent')">
                        {{ formatNumber(currentInfo.memoryUsedPercent) }}%
                    </el-descriptions-item>
                </el-descriptions>

                <el-descriptions
                    v-if="currentInfo.swapMemoryTotal"
                    direction="vertical"
                    :title="$t('home.swapMem')"
                    :column="4"
                    size="small"
                    class="ml-1"
                >
                    <el-descriptions-item :label-width="60" :label="$t('home.total')">
                        {{ computeSize(currentInfo.swapMemoryTotal) }}
                    </el-descriptions-item>
                    <el-descriptions-item :label-width="60" :label="$t('home.used')">
                        {{ computeSize(currentInfo.swapMemoryUsed) }}
                    </el-descriptions-item>
                    <el-descriptions-item :label-width="60" :label="$t('home.free')">
                        {{ computeSize(currentInfo.swapMemoryAvailable) }}
                    </el-descriptions-item>
                    <el-descriptions-item :label-width="60" :label="$t('home.percent')">
                        {{ formatNumber(currentInfo.swapMemoryUsedPercent) }}%
                    </el-descriptions-item>
                </el-descriptions>

                <template #reference>
                    <v-charts
                        height="160px"
                        id="memory"
                        type="pie"
                        :option="chartsOption['memory']"
                        v-if="chartsOption['memory']"
                    />
                </template>
            </el-popover>
            <span class="input-help">
                {{ computeSize(currentInfo.memoryUsed) }} / {{ computeSize(currentInfo.memoryTotal) }}
            </span>
        </el-col>
        <template v-for="(item, index) of currentInfo.diskData" :key="index">
            <el-col :xs="6" :sm="6" :md="3" :lg="3" :xl="3" align="center" v-if="isShow('disk', index)">
                <el-popover :hide-after="20" :teleported="false" :width="450" v-if="chartsOption[`disk${index}`]">
                    <el-descriptions :column="1" size="small">
                        <el-descriptions-item :label="$t('home.mount')">
                            {{ item.path }}
                        </el-descriptions-item>
                        <el-descriptions-item :label="$t('commons.table.type')">
                            {{ item.type }}
                        </el-descriptions-item>
                        <el-descriptions-item :label="$t('home.fileSystem')">
                            {{ item.device }}
                        </el-descriptions-item>
                    </el-descriptions>
                    <el-descriptions title="Inode" direction="vertical" :column="4" size="small">
                        <el-descriptions-item :label="$t('home.total')">{{ item.inodesTotal }}</el-descriptions-item>
                        <el-descriptions-item :label="$t('home.used')">{{ item.inodesUsed }}</el-descriptions-item>
                        <el-descriptions-item :label="$t('home.free')">{{ item.inodesFree }}</el-descriptions-item>
                        <el-descriptions-item :label="$t('home.percent')">
                            {{ formatNumber(item.inodesUsedPercent) }}%
                        </el-descriptions-item>
                    </el-descriptions>

                    <el-descriptions :title="$t('monitor.disk')" direction="vertical" :column="4" size="small">
                        <el-descriptions-item :label="$t('home.total')">
                            {{ computeSize(item.total) }}
                        </el-descriptions-item>
                        <el-descriptions-item :label="$t('home.used')">
                            {{ computeSize(item.used) }}
                        </el-descriptions-item>
                        <el-descriptions-item :label="$t('home.free')">
                            {{ computeSize(item.free) }}
                        </el-descriptions-item>
                        <el-descriptions-item :label="$t('home.percent')">
                            {{ formatNumber(item.usedPercent) }}%
                        </el-descriptions-item>
                    </el-descriptions>
                    <template #reference>
                        <v-charts
                            @click="routerToFileWithPath(item.path)"
                            height="160px"
                            :id="`disk${index}`"
                            type="pie"
                            :option="chartsOption[`disk${index}`]"
                            v-if="chartsOption[`disk${index}`]"
                        />
                    </template>
                </el-popover>
                <span class="input-help">{{ computeSize(item.used) }} / {{ computeSize(item.total) }}</span>
            </el-col>
        </template>
        <el-col :xs="6" :sm="6" :md="3" :lg="3" :xl="3" align="center" v-if="totalCount > 5">
            <el-button v-if="!showMore" link type="primary" @click="changeShowMore(true)" class="buttonClass">
                {{ $t('tabs.more') }}
                <el-icon><Bottom /></el-icon>
            </el-button>
            <el-button v-if="showMore" type="primary" link @click="changeShowMore(false)" class="buttonClass">
                {{ $t('tabs.hide') }}
                <el-icon><Top /></el-icon>
            </el-button>
        </el-col>
    </div>
</template>

<script setup lang="ts">
import { Dashboard } from '@/api/interface/dashboard';
import { computeSize } from '@/utils/size';
import i18n from '@/lang';
import { nextTick, ref } from 'vue';
import { routerToFileWithPath } from '@/utils/router';
const showMore = ref(false);
const totalCount = ref();

const baseInfo = ref<Dashboard.BaseInfo>({
    hostname: '',
    os: '',
    platform: '',
    platformFamily: '',
    platformVersion: '',
    kernelArch: '',
    kernelVersion: '',
    virtualizationSystem: '',
    ipV4Addr: '',
    httpProxy: '',

    cpuCores: 0,
    cpuLogicalCores: 0,
    cpuModelName: '',
    cpuMhz: 0,
    currentInfo: null,
    prettyDistro: '',
    quickJump: [],
});
const currentInfo = ref<Dashboard.CurrentInfo>({
    uptime: 0,
    timeSinceUptime: '',
    runningTime: {
        days: 0,
        hours: 0,
        minutes: 0,
        seconds: 0,
    },
    procs: 0,

    load1: 0,
    load5: 0,
    load15: 0,
    loadUsagePercent: 0,

    cpuPercent: [] as Array<number>,
    cpuUsedPercent: 0,
    cpuUsed: 0,
    cpuTotal: 0,
    cpuDetailedPercent: [] as Array<number>,

    memoryTotal: 0,
    memoryAvailable: 0,
    memoryCache: 0,
    memoryFree: 0,
    memoryShard: 0,
    memoryUsed: 0,
    memoryUsedPercent: 0,
    swapMemoryTotal: 0,
    swapMemoryAvailable: 0,
    swapMemoryUsed: 0,
    swapMemoryUsedPercent: 0,

    ioReadBytes: 0,
    ioWriteBytes: 0,
    ioCount: 0,
    ioReadTime: 0,
    ioWriteTime: 0,

    diskData: [],
    gpuData: [],
    npuData: [],
    xpuData: [],
    netBytesSent: 0,
    netBytesRecv: 0,
    shotTime: new Date(),
});

const chartsOption = ref({
    cpu: { title: 'CPU', data: 0 },
    memory: { title: i18n.global.t('monitor.memory'), data: 0 },
    load: { title: i18n.global.t('home.load'), data: 0 },
});

const acceptParams = (current: Dashboard.CurrentInfo, base: Dashboard.BaseInfo): void => {
    // 节点能力可能按平台缺省返回字段；先归一化，避免模板对 undefined 调用 toFixed/length。
    current = normalizeDashboardInfo(current);
    base = normalizeDashboardBase(base);
    currentInfo.value = current;
    baseInfo.value = base;
    chartsOption.value['cpu'] = {
        title: 'CPU',
        data: formatNumber(currentInfo.value.cpuUsedPercent),
    };
    chartsOption.value['memory'] = {
        title: i18n.global.t('monitor.memory'),
        data: formatNumber(currentInfo.value.memoryUsedPercent),
    };
    chartsOption.value['load'] = {
        title: i18n.global.t('home.load'),
        data: formatNumber(currentInfo.value.loadUsagePercent),
    };
    currentInfo.value.diskData = currentInfo.value.diskData || [];
    nextTick(() => {
        for (let i = 0; i < currentInfo.value.diskData.length; i++) {
            let itemPath = currentInfo.value.diskData[i].path;
            itemPath = itemPath.length > 12 ? itemPath.substring(0, 9) + '...' : itemPath;
            chartsOption.value['disk' + i] = {
                title: itemPath,
                data: formatNumber(currentInfo.value.diskData[i].usedPercent),
            };
        }
        totalCount.value = currentInfo.value.diskData.length;
        showMore.value = localStorage.getItem('dashboard_show') === 'more';
    });
};

const isShow = (val: string, index: number) => {
    let showCount = totalCount.value < 6 ? 5 : 4;
    return val === 'disk' && (showMore.value || index < showCount);
};

const changeShowMore = (show: boolean) => {
    showMore.value = show;
    localStorage.setItem('dashboard_show', show ? 'more' : 'hide');
};

function loadStatus(val: number) {
    if (val < 30) {
        return i18n.global.t('home.runSmoothly');
    }
    if (val < 70) {
        return i18n.global.t('home.runNormal');
    }
    if (val < 80) {
        return i18n.global.t('home.runSlowly');
    }
    return i18n.global.t('home.runJam');
}

function formatNumber(val: number) {
    const number = Number(val);
    return Number.isFinite(number) ? Number(number.toFixed(2)) : 0;
}

const normalizeDashboardBase = (base?: Partial<Dashboard.BaseInfo>): Dashboard.BaseInfo => ({
    ...baseInfo.value,
    ...(base || {}),
    cpuCores: Number(base?.cpuCores) || 0,
    cpuLogicalCores: Number(base?.cpuLogicalCores) || 0,
    cpuMhz: Number(base?.cpuMhz) || 0,
    quickJump: Array.isArray(base?.quickJump) ? base.quickJump : [],
});

const normalizeDashboardInfo = (current?: Partial<Dashboard.CurrentInfo>): Dashboard.CurrentInfo => ({
    ...currentInfo.value,
    ...(current || {}),
    load1: Number(current?.load1) || 0,
    load5: Number(current?.load5) || 0,
    load15: Number(current?.load15) || 0,
    loadUsagePercent: Number(current?.loadUsagePercent) || 0,
    cpuPercent: Array.isArray(current?.cpuPercent) ? current.cpuPercent : [],
    cpuDetailedPercent: Array.isArray(current?.cpuDetailedPercent) ? current.cpuDetailedPercent : [],
    diskData: Array.isArray(current?.diskData) ? current.diskData : [],
    gpuData: [],
    npuData: [],
    xpuData: [],
    cpuUsedPercent: Number(current?.cpuUsedPercent) || 0,
    cpuUsed: Number(current?.cpuUsed) || 0,
    cpuTotal: Number(current?.cpuTotal) || 0,
    memoryTotal: Number(current?.memoryTotal) || 0,
    memoryUsed: Number(current?.memoryUsed) || 0,
    memoryUsedPercent: Number(current?.memoryUsedPercent) || 0,
});


defineExpose({
    acceptParams,
});
</script>

<style scoped lang="scss">
.buttonClass {
    margin-top: 28%;
    font-size: 14px;
}
.cpu-detail {
    font-size: 12px;
    width: 95px;
}
:deep(.el-descriptions__label) {
    width: 80px;
    background-color: transparent !important;
}
.custom-row {
    display: grid;
    grid-template-columns: repeat(12, 1fr);
    gap: 10px;
    width: 100%;
}
.custom-row .el-col {
    width: 100% !important;
    max-width: 100% !important;
    flex: none !important;
    float: none !important;
    display: block !important;
}
.custom-row .el-col.el-col-xs-6 {
    grid-column: span 6;
}
@media (min-width: 768px) {
    .custom-row .el-col.el-col-sm-6 {
        grid-column: span 6;
    }
}
@media (min-width: 992px) {
    .custom-row .el-col.el-col-md-3 {
        grid-column: span 3;
    }
}
@media (min-width: 1200px) {
    .custom-row .el-col.el-col-lg-3 {
        grid-column: span 3;
    }
}
@media (min-width: 1920px) {
    .custom-row .el-col.el-col-xl-3 {
        grid-column: span 3;
    }
}
</style>
