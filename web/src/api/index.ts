import axios, { AxiosInstance, AxiosError, AxiosRequestConfig, AxiosResponse, InternalAxiosRequestConfig } from 'axios';
import { ResultData } from '@/api/interface';
import { ResultEnum } from '@/enums/http-enum';
import { checkStatus } from './helper/check-status';
import router from '@/routers';
import { MsgError } from '@/utils/message';
import { encodeBase64 } from '@/utils/base64';
import i18n from '@/lang';
import { changeToLocal } from '@/utils/node';
import { getCookie } from '@/utils/auth';
import { handleAuthResponseCode } from '@/utils/auth-response';
import { GlobalStore } from '@/store';
import { getOperateNodeOverride } from '@/utils/operate-node';
import { configuredApiPath } from '@/api/transport';

const config = {
    // 即使构建变量被配置为完整 URL，也只允许浏览器请求当前服务入口。
    baseURL: configuredApiPath(),
    timeout: ResultEnum.TIMEOUT as number,
    withCredentials: true,
};

const isCsrfForbidden = (response?: AxiosResponse<any>) => {
    const message = response?.data?.message;
    return typeof message === 'string' && message.toLowerCase().includes('csrf token invalid');
};

type RequestConfig = AxiosRequestConfig & {
    skipErrorMessage?: boolean;
};

class RequestHttp {
    service: AxiosInstance;
    public constructor(config: AxiosRequestConfig) {
        this.service = axios.create(config);
        this.service.interceptors.request.use(
            (config: AxiosRequestConfig) => {
                const globalStore = GlobalStore();
                config.headers = {
                    'Accept-Language': globalStore.language,
                    ...config.headers,
                };
                if (config.headers.CurrentNode == undefined) {
                    config.headers.CurrentNode = encodeURIComponent(
                        getOperateNodeOverride() || globalStore.currentNode,
                    );
                } else {
                    config.headers.CurrentNode = encodeURIComponent(String(config.headers.CurrentNode));
                }
                if (
                    config.url === '/core/auth/login' ||
                    config.url === '/core/auth/mfalogin' ||
                    config.url === '/core/auth/passkey/begin' ||
                    config.url === '/core/auth/passkey/finish' ||
                    config.url === '/core/auth/oidc/begin' ||
                    config.url === '/core/auth/oidc/finish' ||
                    config.url === '/core/auth/saml2/begin' ||
                    config.url === '/core/auth/saml2/finish'
                ) {
                    config.headers.EntranceCode = encodeBase64(globalStore.entrance);
                }
                const method = (config.method || 'get').toUpperCase();
                const requiresToken = !['GET', 'HEAD', 'OPTIONS', 'TRACE'].includes(method);
                if (requiresToken) {
                    const csrfToken = getCookie('pcsrftoken');
                    if (csrfToken) {
                        config.headers['X-CSRF-Token'] = csrfToken;
                        globalStore.csrfToken = csrfToken;
                    }
                }
                return {
                    ...config,
                } as InternalAxiosRequestConfig<any>;
            },
            (error: AxiosError) => {
                return Promise.reject(error);
            },
        );

        this.service.interceptors.response.use(
            (response: AxiosResponse) => {
                const globalStore = GlobalStore();
                const { data } = response;
                const authResult = handleAuthResponseCode(data, { showRBACMessage: true });
                if (authResult.handled) {
                    if (authResult.action === 'return') {
                        return;
                    }
                    return Promise.reject(data);
                }
                if (data.code == ResultEnum.ERR_XPACK) {
                    // 商业能力采用登录授权，接口能力不足时只返回原始错误，不清除当前登录授权状态。
                    return Promise.reject(data);
                }
                if (data.code == ResultEnum.ERR_ENTERPRISE) {
                    // 不再跳转许可证页面；企业接口仍由后端返回能力错误，避免旧许可证流程干扰登录态。
                    return Promise.reject(data);
                }
                if (data.code == ResultEnum.NODE_UNBIND) {
                    changeToLocal();
                    window.location.reload();
                    return;
                }
                if (data.code == ResultEnum.ERR_GLOBAL_LOADING) {
                    globalStore.isLoading = true;
                    globalStore.loadingText = data.message;
                    return;
                } else {
                    if (globalStore.isLoading) {
                        globalStore.isLoading = false;
                    }
                }
                if (data.code == ResultEnum.ERR_AUTH) {
                    return data;
                }
                if (data.code && data.code !== ResultEnum.SUCCESS) {
                    if (data.message.toLowerCase().indexOf('operation not permitted') !== -1) {
                        MsgError(i18n.global.t('license.tamperHelper'));
                        return Promise.reject(data);
                    }
                    if (!(response.config as RequestConfig).skipErrorMessage) {
                        MsgError(data.message);
                    }
                    return Promise.reject(data);
                }
                return data;
            },
            async (error: AxiosError) => {
                const { response } = error;

                // 服务重启或会话过期后，任意接口都可能返回本地鉴权错误。
                // 在全局拦截器清理持久化状态，避免页面继续以失效会话请求绑定接口。
                const localAuthRequired =
                    response?.status === 401 ||
                    (response?.data as any)?.details?.errCode === 'LOCAL_AUTH_REQUIRED';
                if (localAuthRequired && !String(response?.config?.url || '').includes('/core/auth/login')) {
                    const globalStore = GlobalStore();
                    globalStore.setLogStatus(false);
                    globalStore.clearAuthInfo();
                    if (router.currentRoute.value.name !== 'login' && router.currentRoute.value.name !== 'entrance') {
                        await router.replace({ name: 'login' });
                    }
                }

                if (error.message.indexOf('timeout') !== -1) MsgError(i18n.global.t('commons.msg.requestTimeout'));
                if (response) {
                    switch (response.status) {
                        case 313:
                            router.push({ name: 'Expired' });
                            return;
                        case 403:
                            if (isCsrfForbidden(response)) {
                                return Promise.reject(error);
                            }
                            if (response.data && response.data['message']) {
                                MsgError(response.data['message']);
                            } else {
                                MsgError(i18n.global.t('commons.res.forbidden'));
                            }
                            return Promise.reject(error);
                        case 500:
                        case 502:
                        case 524:
                        case 407:
                            checkStatus(
                                response.status,
                                response.data && response.data['message'] ? response.data['message'] : '',
                            );
                            return Promise.reject(error);
                        default:
                            return Promise.reject(error);
                    }
                }
                if (!window.navigator.onLine) router.replace({ path: '/500' });
                return Promise.reject(error);
            },
        );
    }

    get<T>(url: string, params?: object, _object = {}): Promise<ResultData<T>> {
        return this.service.get(url, { params, ..._object });
    }
    post<T>(url: string, params?: object, timeout?: number, headers?: object): Promise<ResultData<T>> {
        let config = {
            baseURL: configuredApiPath(),
            timeout: timeout ? timeout : (ResultEnum.TIMEOUT as number),
            withCredentials: true,
            headers: headers,
        };
        if (headers) {
            config.headers = headers;
        }
        return this.service.post(url, params, config);
    }
    postWithConfig<T>(url: string, params?: object, config?: RequestConfig): Promise<ResultData<T>> {
        return this.service.post(url, params, {
            baseURL: configuredApiPath(),
            timeout: ResultEnum.TIMEOUT as number,
            withCredentials: true,
            ...config,
        });
    }
    postLocalNode<T>(url: string, params?: object, timeout?: number): Promise<ResultData<T>> {
        return this.service.post(url, params, {
            baseURL: configuredApiPath(),
            timeout: timeout ? timeout : (ResultEnum.TIMEOUT as number),
            withCredentials: true,
            headers: {
                CurrentNode: 'local',
            },
        });
    }
    put<T>(url: string, params?: object, _object = {}): Promise<ResultData<T>> {
        return this.service.put(url, params, _object);
    }
    delete<T>(url: string, params?: any, _object = {}): Promise<ResultData<T>> {
        return this.service.delete(url, { params, ..._object });
    }
    download<BlobPart>(url: string, params?: object, _object = {}): Promise<BlobPart> {
        return this.service.post(url, params, _object) as unknown as Promise<BlobPart>;
    }
    upload<T>(url: string, params: object = {}, config?: RequestConfig): Promise<ResultData<T>> {
        return this.service.post(url, params, config);
    }
}

export default new RequestHttp(config);
