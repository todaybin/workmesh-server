<template>
    <LayoutContent :title="$t('serverPages.websiteMonitor.trend')" v-loading="loading">
        <template #rightToolBar><TableRefresh @search="load" /></template>
        <template #main>
            <el-card shadow="never">
                <el-form :inline="true">
                    <el-form-item :label="$t('menu.website')">
                        <el-select v-model="websiteID" clearable :placeholder="$t('serverPages.websiteMonitor.allWebsites')" @change="load">
                            <el-option
                                v-for="site in websites"
                                :key="site.websiteID"
                                :label="site.alias"
                                :value="site.websiteID"
                            />
                        </el-select>
                    </el-form-item>
                    <el-form-item :label="$t('serverPages.websiteMonitor.time')">
                        <el-date-picker v-model="range" type="daterange" value-format="YYYY-MM-DD" @change="load" />
                    </el-form-item>
                </el-form>
                <div ref="chartRef" class="trend-chart" />
            </el-card>
            <el-card shadow="never" class="mt-4">
                <el-table :data="stats" stripe>
                    <el-table-column prop="day" :label="$t('serverPages.websiteMonitor.date')" />
                    <el-table-column prop="pv" label="PV" />
                    <el-table-column prop="uv" label="UV" />
                    <el-table-column prop="flow" :label="$t('serverPages.websiteMonitor.flow')" />
                    <el-table-column prop="req" :label="$t('serverPages.websiteMonitor.requests')" />
                    <el-table-column prop="count4xx" label="4xx" />
                    <el-table-column prop="count5xx" label="5xx" />
                </el-table>
            </el-card>
        </template>
    </LayoutContent>
</template>
<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import { monitorStat, monitorWebsites } from '@/api/modules/website-monitor';
import echarts from '@/utils/echarts';
import i18n from '@/lang';
const loading = ref(false);
const chartRef = ref<HTMLElement>();
const stats = ref<any[]>([]);
const websites = ref<any[]>([]);
const websiteID = ref<number>();
const range = ref<string[]>([]);
let chart: any;
const load = async () => {
    loading.value = true;
    try {
        const res: any = await monitorStat({
            websiteID: websiteID.value,
            startTime: range.value[0],
            endTime: range.value[1],
        });
        stats.value = res.data || [];
        await nextTick();
        renderChart();
    } finally {
        loading.value = false;
    }
};
const renderChart = () => {
    if (!chartRef.value) return;
    chart = chart || echarts.init(chartRef.value);
    chart.setOption({
        tooltip: { trigger: 'axis' },
        legend: { data: ['PV', 'UV', i18n.global.t('serverPages.websiteMonitor.requests')] },
        grid: { left: 40, right: 20, bottom: 30, top: 35 },
        xAxis: { type: 'category', data: stats.value.map((item) => item.day) },
        yAxis: { type: 'value' },
        series: [
            { name: 'PV', type: 'line', smooth: true, data: stats.value.map((item) => item.pv) },
            { name: 'UV', type: 'line', smooth: true, data: stats.value.map((item) => item.uv) },
            { name: i18n.global.t('serverPages.websiteMonitor.requests'), type: 'line', smooth: true, data: stats.value.map((item) => item.req) },
        ],
    });
};
const resize = () => chart?.resize();
onMounted(async () => {
    const res: any = await monitorWebsites();
    websites.value = res.data || [];
    await load();
    window.addEventListener('resize', resize);
});
onBeforeUnmount(() => {
    window.removeEventListener('resize', resize);
    chart?.dispose();
    chart = undefined;
});
</script>
<style scoped>
.trend-chart {
    width: 100%;
    height: 340px;
}
</style>
