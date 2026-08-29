<template>
    <header class="desktop-header">
        <div class="header-leading">
            <el-tooltip
                :content="menuStore.isCollapse ? $t('commons.button.expand') : $t('commons.button.collapse')"
                placement="bottom"
            >
                <el-button
                    class="collapse-button"
                    text
                    circle
                    :icon="menuStore.isCollapse ? 'ArrowRight' : 'ArrowLeft'"
                    :aria-label="menuStore.isCollapse ? $t('commons.button.expand') : $t('commons.button.collapse')"
                    @click="toggleCollapse"
                />
            </el-tooltip>
            <el-breadcrumb v-if="breadcrumbItems.length" separator="/" class="breadcrumb">
                <el-breadcrumb-item v-for="item in breadcrumbItems" :key="item.key">
                    {{ item.title }}
                </el-breadcrumb-item>
            </el-breadcrumb>
        </div>
    </header>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useRoute } from 'vue-router';
import { MenuStore } from '@/store';
import i18n from '@/lang';

const route = useRoute();
const menuStore = MenuStore();

/** 将路由标题转换为当前语言文本，未注册的标题直接使用原文。 */
const translateTitle = (title: unknown) => {
    if (typeof title !== 'string' || !title) {
        return '';
    }
    return i18n.global.te(title) ? i18n.global.t(title) : title;
};

const breadcrumbItems = computed(() =>
    route.matched
        .filter((record) => record.meta?.title && !record.meta?.hideInBreadcrumb)
        .map((record) => ({
            key: record.path || String(record.name || record.meta.title),
            title: translateTitle(record.meta.title),
        }))
        .filter((item) => item.title),
);

/** 切换侧栏展开状态，保持内容区宽度同步调整。 */
const toggleCollapse = () => {
    menuStore.setCollapse();
};
</script>

<style scoped lang="scss">
.desktop-header {
    display: flex;
    align-items: center;
    flex-shrink: 0;
    height: 49px;
    min-height: 49px;
    padding: 0 20px 0 12px;
    background-color: #ffffff;
    border-bottom: none;
    box-sizing: border-box;
}

.header-leading {
    display: flex;
    align-items: center;
    min-width: 0;
    height: 100%;
}

.collapse-button {
    flex-shrink: 0;
    width: 32px;
    height: 32px;
    margin-right: 8px;
    color: var(--el-text-color-regular);
}

.breadcrumb {
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
}

.breadcrumb :deep(.el-breadcrumb__item) {
    max-width: 240px;
    overflow: hidden;
    text-overflow: ellipsis;
}
</style>
