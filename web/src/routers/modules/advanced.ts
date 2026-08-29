import { Layout } from '@/routers/constant';

// 基础版高级能力统一入口；页面继续复用已有模块，权限仍由原页面控制。
const advancedRouter = {
    // 计划任务排序为 9，高级功能紧随其后。
    sort: 9.1,
    path: '/advanced',
    name: 'Advanced-Menu',
    component: Layout,
    redirect: '/advanced/website-monitor/dashboard',
    meta: {
        icon: 'p-toolbox',
        title: 'menu.advanced',
        alwaysVisible: true,
    },
    children: [
        {
            path: '/advanced/website-monitor',
            name: 'AdvancedWebsiteMonitor',
            component: () => import('@/views/advanced/website-monitor/index.vue'),
            meta: {
                icon: 'p-system-monitor-menu',
                title: 'menu.websiteMonitor',
                alwaysVisible: true,
            },
            children: [
                {
                    path: 'dashboard',
                    name: 'AdvancedWebsiteMonitorDashboard',
                    component: () => import('@/views/advanced/website-monitor/dashboard/index.vue'),
                    meta: { activeMenu: '/advanced/website-monitor' },
                },
                {
                    path: 'rank',
                    name: 'AdvancedWebsiteMonitorRank',
                    component: () => import('@/views/advanced/website-monitor/rank/index.vue'),
                    meta: { activeMenu: '/advanced/website-monitor' },
                },
                {
                    path: 'trend',
                    name: 'AdvancedWebsiteMonitorTrend',
                    component: () => import('@/views/advanced/website-monitor/trend/index.vue'),
                    meta: { activeMenu: '/advanced/website-monitor' },
                },
                {
                    path: 'log',
                    name: 'AdvancedWebsiteMonitorLog',
                    component: () => import('@/views/advanced/website-monitor/log/index.vue'),
                    meta: { activeMenu: '/advanced/website-monitor' },
                },
                {
                    path: 'websites',
                    name: 'AdvancedWebsiteMonitorWebsites',
                    component: () => import('@/views/advanced/website-monitor/websites/index.vue'),
                    meta: { activeMenu: '/advanced/website-monitor' },
                },
                {
                    path: 'setting',
                    name: 'AdvancedWebsiteMonitorSetting',
                    component: () => import('@/views/advanced/website-monitor/setting/index.vue'),
                    meta: { activeMenu: '/advanced/website-monitor' },
                },
            ],
        },
        {
            path: '/advanced/waf',
            name: 'AdvancedWaf',
            component: () => import('@/views/advanced/waf/index.vue'),
            meta: {
                icon: 'p-firewalld-menu',
                title: 'menu.waf',
                alwaysVisible: true,
            },
        },
        {
            path: '/advanced/multi-node',
            name: 'AdvancedMultiNode',
            component: () => import('@/views/advanced/multi-node/index.vue'),
            meta: {
                icon: 'p-host',
                title: 'menu.multiNode',
                alwaysVisible: true,
            },
        },
    ],
};

export default advancedRouter;
