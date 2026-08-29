<template>
    <template v-for="subItem in menuList" :key="subItem.name">
        <el-sub-menu v-if="subItem?.children?.length > 1" :index="subItem.path" popper-class="sidebar-container-popper">
            <template #title>
                <el-icon v-if="subItem.meta?.icon">
                    <SvgIcon :iconName="subItem.meta?.icon as string" />
                </el-icon>
                <span>{{ getMenuTitle(subItem) }}</span>
            </template>
            <SubItem :menuList="subItem.children" :level="level + 1" />
        </el-sub-menu>

        <el-menu-item v-else-if="subItem?.children?.length === 1" :index="subItem.children[0].path">
            <el-icon v-if="subItem.meta?.icon">
                <SvgIcon :iconName="subItem.meta?.icon as string" />
            </el-icon>
            <template #title>
                <span>{{ getMenuTitle(subItem) }}</span>
            </template>
        </el-menu-item>

        <el-menu-item v-else-if="subItem.path === '/xpack/upage'" :index="''" @click="goUpage">
            <el-icon v-if="subItem.meta?.icon && level === 0">
                <SvgIcon :iconName="subItem.meta?.icon as string" />
            </el-icon>
            <template #title>
                <span v-if="subItem.meta?.icon && level === 0">{{ getMenuTitle(subItem) }}</span>
                <span v-else style="margin-left: 10px">{{ getMenuTitle(subItem) }}</span>
            </template>
        </el-menu-item>

        <el-menu-item v-else :index="subItem.path">
            <el-icon v-if="subItem.meta?.icon && level === 0">
                <SvgIcon :iconName="subItem.meta?.icon as string" />
            </el-icon>
            <template #title>
                <span v-if="subItem.meta?.icon && level === 0">{{ getMenuTitle(subItem) }}</span>
                <span v-else style="margin-left: 10px">{{ getMenuTitle(subItem) }}</span>
            </template>
        </el-menu-item>
    </template>
</template>

<script setup lang="ts">
import { RouteRecordRaw } from 'vue-router';
import SvgIcon from '@/components/svg-icon/svg-icon.vue';
import i18n from '@/lang';

defineProps<{ menuList: RouteRecordRaw[]; level?: number }>();

const getMenuTitle = (item: RouteRecordRaw): string => {
    const title = item.meta?.title;
    if (typeof title === 'string' && title) {
        return i18n.global.t(title, 2);
    }
    const childTitle = item.children?.[0]?.meta?.title;
    if (typeof childTitle === 'string' && childTitle) {
        return i18n.global.t(childTitle, 2);
    }
    return String(item.name || '');
};

const goUpage = () => {
    window.open('https://www.lxware.cn/upage', '_blank', 'noopener,noreferrer');
};
</script>

<style scoped lang="scss">
@use '../index';
</style>
