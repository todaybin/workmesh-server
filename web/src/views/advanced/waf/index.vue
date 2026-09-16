<template>
    <div>
        <RouterButton :buttons="buttons" />
        <LayoutContent>
            <router-view />
        </LayoutContent>
    </div>
</template>

<script setup lang="ts">
import i18n from '@/lang';
const buttons = [
    { label: i18n.global.t('xpack.waf.overview'), path: '/advanced/waf/overview' },
    { label: i18n.global.t('xpack.waf.stat'), path: '/advanced/waf/attack' },
    { label: i18n.global.t('xpack.waf.intercept'), path: '/advanced/waf/intercept' },
    { label: i18n.global.t('xpack.waf.blockRecords'), path: '/advanced/waf/block' },
    { label: i18n.global.t('xpack.waf.blackWhite'), path: '/advanced/waf/blackwhite' },
    { label: i18n.global.t('xpack.waf.websiteSetting'), path: '/advanced/waf/websites' },
    { label: i18n.global.t('xpack.waf.globalSetting'), path: '/advanced/waf/global' },
];
</script>
