<template>
    <div>
        <RouterButton :buttons="buttons" />
        <LayoutContent><router-view /></LayoutContent>
    </div>
</template>
<script setup lang="ts">
import i18n from '@/lang';
const buttons = [
    { label: i18n.global.t('xpack.monitor.overview'), path: '/advanced/website-monitor/dashboard' },
    { label: i18n.global.t('xpack.monitor.visitStats'), path: '/advanced/website-monitor/rank' },
    { label: i18n.global.t('xpack.monitor.trendStats'), path: '/advanced/website-monitor/trend' },
    { label: i18n.global.t('xpack.monitor.requestLogs'), path: '/advanced/website-monitor/log' },
    { label: i18n.global.t('xpack.monitor.websiteList'), path: '/advanced/website-monitor/websites' },
    { label: i18n.global.t('xpack.monitor.settings'), path: '/advanced/website-monitor/setting' },
];
</script>
