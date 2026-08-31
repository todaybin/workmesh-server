import router from '@/routers/router';
import NProgress from '@/config/nprogress';
import { useGlobalStore } from '@/composables/useGlobalStore';
import { AxiosCanceler } from '@/api/helper/axios-cancel';
import { hasRouteAccess } from '@/utils/rbac';
import i18n from '@/lang';
import { MsgError } from '@/utils/message';
import { getWorkMeshGatewayStatus } from '@/api/modules/workmesh';

const axiosCanceler = new AxiosCanceler();

let isRedirecting = false;
const noLoginWhiteList = ['entrance', 'login', 'file-share', '404', 'Expired'];

const clearLoginStatus = () => {
    const { globalStore } = useGlobalStore();
    globalStore.setLogStatus(false);
    globalStore.clearAuthInfo();
};

// 服务重启后本地会话可能失效；识别统一错误码，避免把用户误导到 Gateway 绑定页。
const isLocalAuthRequired = (error: any) =>
    error?.response?.status === 401 || error?.response?.data?.details?.errCode === 'LOCAL_AUTH_REQUIRED';

router.beforeEach(async (to, from) => {
    const { entrance, isLogin } = useGlobalStore();
    NProgress.start();
    axiosCanceler.removeAllPending();

    if (!isLogin.value) {
        clearLoginStatus();
    }
    if (!isLogin.value && !noLoginWhiteList.includes(String(to.name))) {
        NProgress.done();
        return entrance.value
            ? {
                  name: 'entrance',
                  params: { code: entrance.value },
              }
            : {
                  name: 'login',
              };
    }
    let gatewayConfigured = true;
    let bindingRequired = true;
    if (isLogin.value && !['entrance', 'file-share', '404', 'Expired'].includes(String(to.name))) {
        try {
            const gatewayStatus = await getWorkMeshGatewayStatus();
            bindingRequired = gatewayStatus.data.bindingRequired !== false;
            gatewayConfigured = !bindingRequired || !!gatewayStatus.data.configured;
        } catch (error) {
            if (isLocalAuthRequired(error)) {
                clearLoginStatus();
                NProgress.done();
                return { name: 'login' };
            }
            gatewayConfigured = false;
        }
    }
    if (to.name === 'login' && !isLogin.value && entrance.value) {
        NProgress.done();
        return {
            name: 'entrance',
            params: { code: entrance.value },
        };
    }
    if (to.name === 'login' && isLogin.value) {
        NProgress.done();
        return { name: gatewayConfigured ? 'home' : 'GatewayBind' };
    }
    if (isLogin.value && !gatewayConfigured && String(to.name) !== 'GatewayBind') {
        NProgress.done();
        return { name: 'GatewayBind' };
    }
    if (isLogin.value && gatewayConfigured && String(to.name) === 'GatewayBind') {
        NProgress.done();
        return { name: 'home' };
    }
    if (to.name === 'entrance' && isLogin.value) {
        if (to.params.code === entrance.value) {
            NProgress.done();
            return {
                name: 'home',
            };
        }
        NProgress.done();
        return { name: '404' };
    }

    const originalPath = String(to.path);
    const compatPath = originalPath.startsWith('/xpack/monitor')
        ? originalPath.replace(/^\/xpack\/monitor/, '/advanced/website-monitor')
        : originalPath.startsWith('/xpack/waf')
          ? '/advanced/waf'
          : originalPath.startsWith('/xpack/node')
            ? '/advanced/multi-node'
            : originalPath;
    if (compatPath !== to.path) {
        return { path: compatPath, query: to.query, hash: to.hash };
    }
    if (to.path === '/apps/all' && to.query.install != undefined) {
        return true;
    }
    if (to.name === 'Expired') {
        return true;
    }
    const activeMenuKey = 'cachedRoute' + (to.meta.activeMenu || '');
    if (to.query.uncached != undefined) {
        const query = { ...to.query };
        delete query.uncached;
        localStorage.removeItem(activeMenuKey);
        return { path: to.path, query };
    }

    const cachedRoute = localStorage.getItem(activeMenuKey);
    if (
        to.meta.activeMenu &&
        to.meta.activeMenu != from.meta.activeMenu &&
        cachedRoute &&
        cachedRoute !== to.path &&
        !isRedirecting
    ) {
        const cachedRouteInfo = router.resolve(cachedRoute);
        if (cachedRouteInfo.matched.length > 0 && hasRouteAccess(cachedRouteInfo)) {
            isRedirecting = true;
            NProgress.done();
            return cachedRoute;
        }
        localStorage.removeItem(activeMenuKey);
    }

    if (!hasRouteAccess(to)) {
        MsgError(i18n.global.t('commons.res.forbidden'));
        NProgress.done();
        return false;
    }
    return true;
});

router.afterEach((to) => {
    if (to.meta.activeMenu && !to.meta.ignoreTab && !isRedirecting) {
        let notMathParam = true;
        if (to.matched.some((record) => record.path.includes(':'))) {
            notMathParam = false;
        }
        if (notMathParam) {
            if (to.meta.activeMenu === '/cronjobs' && to.path === '/cronjobs/cronjob/operate') {
                localStorage.setItem('cachedRoute' + to.meta.activeMenu, '/cronjobs/cronjob');
            } else if (to.meta.activeMenu === '/containers' && to.path === '/containers/container/operate') {
                localStorage.setItem('cachedRoute' + to.meta.activeMenu, '/containers/container');
            } else if (to.meta.activeMenu === '/toolbox' && to.path === '/toolbox/clam/setting') {
                localStorage.setItem('cachedRoute' + to.meta.activeMenu, '/toolbox/clam');
            } else {
                localStorage.setItem('cachedRoute' + to.meta.activeMenu, to.path);
            }
        }
    }

    isRedirecting = false;
    NProgress.done();
});

export default router;
