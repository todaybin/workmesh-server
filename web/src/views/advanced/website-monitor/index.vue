<template>
    <div>
        <RouterButton :buttons="buttons" />
        <LayoutContent><router-view /></LayoutContent>
    </div>
</template>
<script setup lang="ts">
import i18n from '@/lang';
const buttons = [
    { label: i18n.global.t('menu.home'), path: '/advanced/website-monitor/dashboard' },
    { label: i18n.global.t('xpack.monitor.reqRank'), path: '/advanced/website-monitor/rank' },
    { label: i18n.global.t('xpack.monitor.trend'), path: '/advanced/website-monitor/trend' },
    { label: i18n.global.t('xpack.monitor.log'), path: '/advanced/website-monitor/log' },
    { label: i18n.global.t('xpack.monitor.websites'), path: '/advanced/website-monitor/websites' },
    { label: i18n.global.t('commons.button.set'), path: '/advanced/website-monitor/setting' },
];
</script>
