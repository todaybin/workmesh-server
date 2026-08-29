<template>
    <div
        class="sidebar-container"
        element-loading-text="Loading..."
        :element-loading-spinner="loadingSvg"
        element-loading-svg-view-box="-10, -10, 50, 50"
        element-loading-background="rgba(122, 122, 122, 0.01)"
    >
        <Logo :isCollapse="isCollapse" />
        <el-scrollbar class="sidebar-scrollbar">
            <el-menu
                :default-active="activeMenu"
                :router="true"
                :collapse="isCollapse"
                :collapse-transition="false"
                :unique-opened="!menuAccordion"
                @select="handleMenuClick"
                class="custom-menu"
            >
                <SubItem :menuList="routerMenus" :level="0" />
            </el-menu>
        </el-scrollbar>
        <Collapse :version="version" @open-task="openTask" @refresh="search" />
    </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { RouteRecordRaw, useRoute } from 'vue-router';
import { loadingSvg } from '@/utils/svg';
import Logo from './components/Logo.vue';
import Collapse from './components/Collapse.vue';
import SubItem from './components/SubItem.vue';
import { menuList } from '@/routers/router';
import { MenuStore } from '@/store';
import { getSettingBaseInfo } from '@/api/modules/setting';
import { hasPermissionMetaAccess, hasRouteRoleAccess } from '@/utils/rbac';
import { useGlobalStore } from '@/composables/useGlobalStore';

const route = useRoute();
const menuStore = MenuStore();
const { currentNode, isAdmin, isEE, isIntl, menuAccordion, permissions } = useGlobalStore();
const version = ref();

const activeMenu = computed(() => {
    const { meta, path } = route;
    return typeof meta.activeMenu === 'string' ? meta.activeMenu : path;
});
const isCollapse = computed((): boolean => menuStore.isCollapse);

let routerMenus = computed((): RouteRecordRaw[] => {
    return menuStore.menuList.filter((route) => route.meta && !route.meta.hideInSidebar) as RouteRecordRaw[];
});

const screenWidth = ref(0);
const listeningWindow = () => {
    window.onresize = () => {
        return (() => {
            screenWidth.value = document.body.clientWidth;
            if (!isCollapse.value && screenWidth.value < 1200) menuStore.setCollapse();
            if (isCollapse.value && screenWidth.value > 1200) menuStore.setCollapse();
        })();
    };
};
listeningWindow();
const emit = defineEmits(['menuClick', 'openTask']);
const handleMenuClick = (path) => {
    emit('menuClick', path);
};

function getCheckedLabels(menu: any, showSet: Set<string>) {
    for (const item of menu) {
        if (item.isShow) {
            showSet.add(item.label);
        }
        if (item.children) {
            getCheckedLabels(item.children, showSet);
        }
    }
}

function getAlwaysVisibleNames(menu: RouteRecordRaw[], showSet: Set<string>) {
    for (const item of menu) {
        if (item.meta?.alwaysVisible && item.name) {
            showSet.add(item.name as string);
        }
        if (Array.isArray(item.children)) {
            getAlwaysVisibleNames(item.children, showSet);
        }
    }
}

const openTask = () => {
    emit('openTask');
};

const search = async () => {
    let settingInfo: { systemVersion: string; hideMenu?: string; menuAccordion?: string } | null = null;
    try {
        const res = await getSettingBaseInfo();
        settingInfo = res.data;
        version.value = res.data.systemVersion;
        menuAccordion.value = res.data.menuAccordion === 'Enable';
    } catch (error) {
        version.value = '';
    }

    if (!settingInfo?.hideMenu) {
        setDefaultMenuList();
        return;
    }

    try {
        const rstMenuList = buildMenuListFromSettings(settingInfo.hideMenu);
        if (!isSameMenuList(menuStore.menuList as RouteRecordRaw[], rstMenuList)) {
            menuStore.setMenuList(rstMenuList);
        }
    } catch (error) {
        setDefaultMenuList();
    }
};

function isSameMenuList(source: RouteRecordRaw[], target: RouteRecordRaw[]) {
    return JSON.stringify(source) === JSON.stringify(target);
}

function setDefaultMenuList() {
    const rstMenuList = buildAuthVisibleMenuList(menuList);
    if (!isSameMenuList(menuStore.menuList as RouteRecordRaw[], rstMenuList)) {
        menuStore.setMenuList(rstMenuList);
    }
}

function allowMenuItem(item: RouteRecordRaw) {
    if (!hasRouteRoleAccess(item.meta)) {
        return false;
    }
    return hasPermissionMetaAccess(item.meta?.permission as string | string[] | undefined);
}

function buildMenuListFromSettings(hideMenuValue?: string) {
    const hideMenu = JSON.parse(hideMenuValue || '[]');
    const showSet = new Set<string>();
    getCheckedLabels(hideMenu, showSet);
    // 新增的基础菜单及其子菜单不应被旧版本保存的 hideMenu 配置吞掉。
    getAlwaysVisibleNames(menuList, showSet);
    const rstMenuList: RouteRecordRaw[] = [];
    const resMenuList = adjustAndCleanMenu(hideMenu, menuList);
    for (const item of menuList) {
        if (item.meta?.alwaysVisible) {
            const existingIndex = resMenuList.findIndex((menu) => menu.name === item.name);
            if (existingIndex < 0) {
                resMenuList.push(item);
            } else {
                // 用当前路由定义恢复完整子树，避免旧 hideMenu 只保存了父级而吞掉新入口。
                resMenuList[existingIndex] = item;
            }
        }
    }
    for (const menu of resMenuList) {
        const menuItem = buildVisibleMenu(menu, showSet);
        if (menuItem) {
            rstMenuList.push(menuItem);
        }
    }
    return rstMenuList;
}

function buildAuthVisibleMenuList(source: RouteRecordRaw[]) {
    return source
        .map((item) => {
            if (!allowMenuItem(item)) {
                return null;
            }
            const menuItem = JSON.parse(JSON.stringify(item));
            const children = Array.isArray(menuItem.children) ? menuItem.children : [];
            if (children.length === 0) {
                return menuItem;
            }
            menuItem.children = buildAuthVisibleMenuList(children).filter(Boolean);
            if (menuItem.children.length === 0) {
                return null;
            }
            if (menuItem.children.length === 1) {
                const onlyChild = menuItem.children[0];
                if (onlyChild.meta?.icon) {
                    menuItem.meta.icon = onlyChild.meta.icon;
                }
                if (onlyChild.meta?.title) {
                    menuItem.meta.title = onlyChild.meta.title;
                }
            }
            if (menuItem.name === 'Xpack-Menu') {
                menuItem.meta.hideInSidebar = false;
            }
            return menuItem;
        })
        .filter(Boolean) as RouteRecordRaw[];
}

function buildVisibleMenu(menu: RouteRecordRaw, showSet: Set<string>): RouteRecordRaw | null {
    const menuItem = JSON.parse(JSON.stringify(menu));
    if (!menuItem?.name || !showSet.has(menuItem.name as string)) {
        return null;
    }
    if (!allowMenuItem(menuItem)) {
        return null;
    }

    const children = Array.isArray(menuItem.children) ? menuItem.children : [];
    if (children.length === 0) {
        return menuItem;
    }

    const visibleChildren = children
        .map((item) => {
            if (item.name === 'Upage' && (isIntl.value || (isEE.value && !isAdmin.value))) {
                return null;
            }
            if (item.name === 'XApp' && isIntl.value) {
                return null;
            }
            return buildVisibleMenu(item, showSet);
        })
        .filter(Boolean) as RouteRecordRaw[];

    menuItem.children = visibleChildren;
    if (menuItem.children.length === 0) {
        return null;
    }

    if (menuItem.children.length === 1) {
        const onlyChild = menuItem.children[0];
        if (onlyChild.meta?.icon) {
            menuItem.meta.icon = onlyChild.meta.icon;
        }
        if (onlyChild.meta?.title) {
            menuItem.meta.title = onlyChild.meta.title;
        }
    }
    if (menuItem.name === 'Xpack-Menu') {
        menuItem.meta.hideInSidebar = false;
    }
    return menuItem;
}

function adjustAndCleanMenu(menuItem, list) {
    const menuList = JSON.parse(JSON.stringify(list));
    const itemMap = new Map();
    for (const parent of menuList) {
        itemMap.set(parent.name, parent);
        if (Array.isArray(parent.children)) {
            for (const child of parent.children) {
                itemMap.set(child.name, child);
            }
        }
    }

    function buildTree(refList) {
        const result = [];

        for (const ref of refList) {
            const refName = ref.label;
            const matched = itemMap.get(refName);

            if (!matched) continue;

            if (Array.isArray(ref.children) && ref.children.length > 0) {
                matched.children = buildTree(ref.children);
            } else {
                delete matched.children;
            }

            result.push(matched);
        }

        return result;
    }

    const newMenu = buildTree(menuItem);
    for (const menu of newMenu) {
        if (menu.children?.length === 1) {
            const onlyChild = menu.children[0];
            if (onlyChild.meta?.icon) {
                menu.meta.icon = onlyChild.meta.icon;
            }
            if (onlyChild.meta?.title) {
                menu.meta.title = onlyChild.meta.title;
            }
        }
    }

    return newMenu;
}

onMounted(() => {
    if (!menuStore.menuList || menuStore.menuList.length === 0) {
        menuStore.setMenuList(buildAuthVisibleMenuList(menuList));
    }
    search();
});

watch(
    () => [currentNode.value, isAdmin.value, permissions.value.join('|')],
    () => {
        search();
    },
);
</script>

<style lang="scss" scoped>
@use 'index';

.background {
    z-index: 20;
}

.custom-menu :deep(.el-menu-item) {
    white-space: normal !important;
    word-break: break-word;
    overflow-wrap: break-word;
    line-height: normal;
}

// 子菜单标题占满菜单项，箭头相对标题右边界定位，一级菜单不会被内容挤出。
.custom-menu :deep(.el-sub-menu__title) {
    position: relative !important;
    width: 100% !important;
    max-width: 100% !important;
    box-sizing: border-box !important;
    padding-right: 28px !important;
    overflow: hidden;
}

.custom-menu :deep(.el-sub-menu__title > span) {
    display: block;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.custom-menu :deep(.el-sub-menu__title > .el-sub-menu__icon-arrow) {
    position: absolute !important;
    top: 50% !important;
    right: 4px !important;
    inset-inline-end: 4px !important;
    left: auto !important;
    width: 16px !important;
    margin: -6px 0 0 !important;
    transform: translateY(0) !important;
}

.custom-menu :deep(.el-sub-menu.is-opened > .el-sub-menu__title > .el-sub-menu__icon-arrow) {
    transform: rotate(180deg) !important;
}

.sidebar-container {
    position: relative;
    display: flex;
    flex-direction: column;
    height: 100%;
    box-sizing: border-box;
    background: #fff;
    border-right: 1px solid #e5e7eb;

    .el-scrollbar {
        flex: 1;
        background-color: #fff;

        :deep(.el-scrollbar__wrap),
        :deep(.el-scrollbar__view) {
            background-color: #fff;
        }

        .el-menu {
            overflow: auto;
            overflow-x: hidden;
            border-right: none;
        }
    }
}

.ico {
    height: 20px !important;
}
</style>
