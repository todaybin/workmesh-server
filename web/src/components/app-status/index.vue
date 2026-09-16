<template>
    <div class="app-status" v-if="data.isExist">
        <el-card>
            <div class="flex w-full flex-col gap-4 md:flex-row">
                <div class="flex flex-wrap gap-4 ml-3">
                    <el-tag effect="dark" type="success">{{ data.app }}</el-tag>
                    <Status class="mt-0.5" :key="refresh" :status="data.status"></Status>
                    <el-tag>{{ $t('app.version') }}: {{ data.version }}</el-tag>
                </div>

                <div class="mt-0.5">
                    <el-button
                        type="primary"
                        v-permission="'app_manage'"
                        v-if="!isRunning"
                        link
                        @click="onOperate('start')"
                        :disabled="isInstalling"
                    >
                        {{ $t('commons.operate.start') }}
                    </el-button>
                    <el-button
                        type="primary"
                        v-permission="'app_manage'"
                        v-if="isRunning"
                        link
                        @click="onOperate('stop')"
                    >
                        {{ $t('commons.operate.stop') }}
                    </el-button>
                    <el-divider direction="vertical" />
                    <el-button
                        type="primary"
                        v-permission="'app_manage'"
                        link
                        :disabled="isInstalling"
                        @click="onOperate('restart')"
                    >
                        {{ $t('commons.operate.restart') }}
                    </el-button>
                    <el-divider v-if="!hideSetting" direction="vertical" />
                    <el-button
                        type="primary"
                        link
                        v-permission:view="'website_manage'"
                        v-if="isOpenResty"
                        @click="onOperate('reload')"
                        :disabled="!isRunning"
                    >
                        {{ $t('commons.operate.reload') }}
                    </el-button>
                    <el-divider v-if="isOpenResty" direction="vertical" />
                    <el-button
                        v-if="!hideSetting"
                        type="primary"
                        @click="setting"
                        link
                        :disabled="isInstalling"
                    >
                        {{ $t('commons.button.set') }}
                    </el-button>
                </div>
                <div class="ml-5" v-if="isOpenResty && (httpPort != 80 || httpsPort != 443)">
                    <el-tooltip
                        effect="dark"
                        :content="$t('website.openrestyHelper', [httpPort, httpsPort])"
                        placement="top-start"
                    >
                        <el-alert
                            :title="$t('app.checkTitle')"
                            :closable="false"
                            center
                            type="warning"
                            show-icon
                            class="h-6 check-title"
                        />
                    </el-tooltip>
                </div>
            </div>
        </el-card>
    </div>
</template>
<script lang="ts" setup>
import { checkAppInstalled, installedOp } from '@/api/modules/app';
import { operateNginx } from '@/api/modules/nginx';
import { computed, onMounted, reactive, ref } from 'vue';
import Status from '@/components/status/index.vue';
import { ElMessageBox } from 'element-plus';
import i18n from '@/lang';
import { MsgError, MsgSuccess } from '@/utils/message';
import { AppInstallId, getAppInstallId, isAppActive, isAppPresent, resolveAppOperationTarget } from '@/utils/app-install';

const props = defineProps({
    appKey: {
        type: String,
        default: 'openresty',
    },
    appName: {
        type: String,
        default: '',
    },
    hideSetting: {
        type: Boolean,
        default: false,
    },
});

let key = ref('');
let name = ref('');

let data = ref({
    app: '',
    version: '',
    status: '',
    isActive: false,
    lastBackupAt: '',
    appInstallId: undefined as AppInstallId | undefined,
    isExist: false,
    containerName: '',
});
let operateReq = reactive({
    installId: undefined as AppInstallId | undefined,
    operate: '',
});
let refresh = ref(1);
const httpPort = ref(0);
const httpsPort = ref(0);
const isOpenResty = computed(() => {
    return ['openresty', 'nginx'].includes(String(key.value || data.value.app).toLowerCase());
});
const normalizedStatus = computed(() => {
    const status = String(data.value.status || '').trim().toLowerCase();
    if (status) return status;
    return isAppActive(data.value.isActive) ? 'running' : '';
});
const isRunning = computed(() => normalizedStatus.value === 'running');
const isInstalling = computed(() => normalizedStatus.value === 'installing');

const em = defineEmits([
    'setting',
    'isExist',
    'before',
    'after',
    'update:loading',
    'update:maskShow',
    'update:appInstallID',
]);
const setting = () => {
    em('setting', false);
};

const onCheck = async (key: any, name: any) => {
    await checkAppInstalled(key, name)
        .then((res) => {
            const responseData = res.data as any;
            const installId = getAppInstallId(
                responseData.appInstallId,
                responseData.appInstallID,
                responseData.installId,
                responseData.id,
            );
            const isExist = isAppPresent(responseData, installId);
            data.value = { ...responseData, isExist, appInstallId: installId };
            em('isExist', data.value);
            em('update:maskShow', !isRunning.value);
            operateReq.installId = installId;
            em('update:appInstallID', installId);
            httpPort.value = Number(responseData.httpPort) || 0;
            httpsPort.value = Number(responseData.httpsPort) || 0;
            refresh.value++;
        })
        .catch(() => {
            data.value = {
                ...data.value,
                isExist: false,
                appInstallId: undefined,
                status: '',
                containerName: '',
            };
            operateReq.installId = undefined;
            httpPort.value = 0;
            httpsPort.value = 0;
            em('isExist', false);
            refresh.value++;
        });
};

const onOperate = async (operation: string) => {
    const target = resolveAppOperationTarget(key.value, data.value);
    if (!target) {
        MsgError(i18n.global.t('app.installIdInvalid'));
        return;
    }
    ElMessageBox.confirm(
        i18n.global.t('app.operatorHelper', [i18n.global.t('commons.operate.' + operation)]),
        i18n.global.t('commons.operate.' + operation),
        {
            confirmButtonText: i18n.global.t('commons.button.confirm'),
            cancelButtonText: i18n.global.t('commons.button.cancel'),
            type: 'info',
        },
    ).then(() => {
        em('update:maskShow', true);
        em('update:loading', true);
        em('before');
        (target.kind === 'openresty'
            ? operateNginx({ operate: operation })
            : installedOp({ installId: target.installId, operate: operation }))
            .then(() => {
                em('update:loading', false);
                MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
                onCheck(key.value, name.value);
                em('after');
            })
            .catch(() => {
                em('update:loading', false);
            });
    });
};

onMounted(() => {
    key.value = props.appKey;
    name.value = props.appName;
    onCheck(key.value, name.value);
});

defineExpose({
    onCheck,
    getHttpPort: () => httpPort.value,
    getHttpsPort: () => httpsPort.value,
});
</script>
<style scoped lang="scss">
.check-title {
    color: var(--el-color-warning);
    border: 1px solid var(--el-color-warning);
    background-color: transparent;
    padding: 8px 8px;
    width: 70px;
}
</style>
