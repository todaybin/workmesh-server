<template>
    <div v-loading="loading">
        <div class="site-form-wrapper">
            <el-form class="site-form moblie-form" ref="siteForm" :model="update" label-width="100px">
                <el-form-item :label="$t('website.siteAlias')">
                    {{ website.alias }}
                </el-form-item>
                <el-form-item :label="$t('website.primaryPath')">
                    <el-space wrap>
                        {{ website.sitePath + '/app' }}
                        <el-button type="primary" link @click="routerToFileWithPath(website.sitePath + '/app')">
                            <el-icon>
                                <FolderOpened />
                            </el-icon>
                        </el-button>
                    </el-space>
                    <span class="input-help" v-if="configDir">
                        {{ $t('php.indexHelper') }}
                    </span>
                </el-form-item>
                <el-form-item v-if="configDir" :label="$t('website.runDir')">
                    <el-space wrap>
                        <el-select v-model="update.siteDir" filterable class="p-w-200">
                            <el-option
                                v-for="(item, index) in dirs"
                                :label="item"
                                :value="item"
                                :key="index"
                            ></el-option>
                        </el-select>
                        <el-button v-permission type="primary" @click="submit(siteForm)">
                            {{ $t('nginx.saveAndReload') }}
                        </el-button>
                    </el-space>
                    <span class="input-help">
                        {{ $t('website.runDirHelper2') }}
                    </span>
                </el-form-item>
                <el-form-item v-if="configDir" :label="$t('website.userGroup')">
                    <el-space wrap>
                        <el-input v-model="updatePermission.user" class="user-num-input">
                            <template #prepend>{{ $t('commons.table.user') }}</template>
                        </el-input>
                        <el-input v-model="updatePermission.group" class="user-num-input">
                            <template #prepend>{{ $t('website.uGroup') }}</template>
                        </el-input>
                        <el-button v-permission type="primary" @click="submitPermission()">
                            {{ $t('commons.button.save') }}
                        </el-button>
                    </el-space>
                </el-form-item>
            </el-form>
            <el-text type="warning" v-if="configDir">{{ $t('website.runUserHelper') }}</el-text>
            <br />
            <el-text type="danger" v-if="dirConfig.msg != ''">{{ dirConfig.msg }}</el-text>
            <br />
            <el-descriptions :title="$t('website.folderTitle')" :column="1" border>
                <el-descriptions-item label="ssl">{{ $t('website.sslFolder') }}</el-descriptions-item>
                <el-descriptions-item label="log">{{ $t('logs.websiteLog') }}</el-descriptions-item>
                <el-descriptions-item label="index">{{ $t('website.indexFolder') }}</el-descriptions-item>
            </el-descriptions>
        </div>
    </div>
</template>
<script lang="ts" setup>
import { Website } from '@/api/interface/website';
import { getDirConfig, getWebsite, updateWebsiteDir, updateWebsiteDirPermission } from '@/api/modules/website';
import i18n from '@/lang';
import { MsgSuccess } from '@/utils/message';
import { routerToFileWithPath } from '@/utils/router';
import { FormInstance } from 'element-plus';
import { computed, onMounted, reactive, ref } from 'vue';

const props = defineProps({
    id: {
        type: Number,
        default: 0,
    },
});
const websiteId = computed(() => {
    return Number(props.id);
});
const website = ref<any>({});
const loading = ref(false);
const configDir = ref(false);
const update = reactive({
    id: 0,
    siteDir: '/',
});
const updatePermission = reactive({
    id: 0,
    user: 'www',
    group: 'www',
});
const siteForm = ref<FormInstance>();
const dirs = ref([]);
const dirConfig = ref<Website.DirConfig>({
    dirs: [''],
    user: '',
    userGroup: '',
    msg: '',
});

const search = () => {
    loading.value = true;
    getWebsite(websiteId.value)
        .then((res) => {
            website.value = res.data;
            update.id = website.value.id;
            update.siteDir = normalizeRunDir(website.value);
            updatePermission.id = website.value.id;
            updatePermission.group = website.value.group === '' ? 'www' : website.value.group;
            updatePermission.user = website.value.user === '' ? 'www' : website.value.user;
            if ((website.value.type === 'static' || !!website.value.runtimeID) && website.value.type != 'subsite') {
                configDir.value = true;
                getConfig();
            }
        })
        .finally(() => {
            loading.value = false;
        });
};

// 后端站点详情同时保留物理 sitePath；运行目录下拉框只展示 app 根下的虚拟相对路径。
const normalizeRunDir = (item: any) => {
    const value = String(item?.siteDir || '').trim();
    const sitePath = String(item?.sitePath || '').replace(/\\+$/, '');
    const appRoot = sitePath ? `${sitePath}/app` : '';
    if (!value || value === '/' || value === sitePath || value === appRoot) return '/';
    if (appRoot && value.startsWith(`${appRoot}/`)) {
        return `/${value.slice(appRoot.length + 1)}`;
    }
    return value.startsWith('/') ? value : `/${value}`;
};

const submit = async (formEl: FormInstance | undefined) => {
    if (!formEl) return;
    await formEl.validate((valid) => {
        if (!valid) {
            return;
        }
        loading.value = true;
        updateWebsiteDir(update)
            .then(() => {
                MsgSuccess(i18n.global.t('commons.msg.updateSuccess'));
                search();
            })
            .finally(() => {
                loading.value = false;
            });
    });
};

const submitPermission = async () => {
    if (updatePermission.user === '' || updatePermission.group === '') {
        return;
    }
    loading.value = true;
    updateWebsiteDirPermission(updatePermission)
        .then(() => {
            MsgSuccess(i18n.global.t('commons.msg.updateSuccess'));
            search();
        })
        .finally(() => {
            loading.value = false;
        });
};

const initData = () => {
    dirs.value = [];
};

const getConfig = async () => {
    try {
        const res = await getDirConfig({ id: props.id });
        dirs.value = res.data.dirs;
        dirConfig.value = res.data;
        // 目录接口返回的是 index 目录的真实 UID/GID，优先用于回填，
        // 避免历史网站元数据中的 www 覆盖实际 1000:1000 属主。
        if (String(res.data.user || '').trim() !== '') {
            updatePermission.user = String(res.data.user);
        }
        if (String(res.data.userGroup || '').trim() !== '') {
            updatePermission.group = String(res.data.userGroup);
        }
    } catch (error) {}
};

onMounted(() => {
    initData();
    search();
});
</script>

<style lang="scss" scoped>
.site-form-wrapper {
    min-width: 600px;
    width: 60%;
    padding: 20px;
}
.site-form {
    :deep(.el-form-item__label) {
        padding-right: 20px !important;
        box-sizing: content-box;
    }
    .user-num-input {
        width: 190px;
    }
}
.warnHelper {
    white-space: pre-line;
    display: block;
}
</style>
