<template>
    <div>
        <div class="flex w-full flex-col gap-2 md:flex-row items-center">
            <div class="flex flex-wrap gap-y-2 items-center">
                <div class="flex flex-wrap items-center">
                    <el-link underline="never" class="version" type="primary" @click="toZoomtk">
                        {{ version }}
                    </el-link>
                    <el-badge
                        is-dot
                        v-if="isAdmin && !isOffline && !isEE"
                        class="-mt-0.5"
                        :hidden="version === 'Waiting' || !hasNewVersion"
                    >
                        <el-link class="ml-2" underline="never" type="primary" @click="onLoadUpgradeInfo">
                            {{ $t('commons.button.update') }}
                        </el-link>
                    </el-badge>
                    <el-tag v-if="version === 'Waiting'" round class="ml-2.5">{{ $t('setting.upgrading') }}</el-tag>
                </div>
            </div>
        </div>

        <Upgrade ref="upgradeRef" @search="search" />
        <Releases ref="releasesRef" />
    </div>
</template>

<script setup lang="ts">
import { getSettingBaseInfo, loadUpgradeInfo } from '@/api/modules/setting';
import Upgrade from '@/components/system-upgrade/upgrade/index.vue';
import Releases from '@/components/system-upgrade/releases/index.vue';
import i18n from '@/lang';
import { MsgSuccess } from '@/utils/message';
import { onMounted, ref } from 'vue';
import { useGlobalStore } from '@/composables/useGlobalStore';

const { isOffline, isEE, isAdmin, hasNewVersion } = useGlobalStore();
const upgradeRef = ref();
const releasesRef = ref();

const version = ref<string>('');
const loading = ref(false);
const upgradeInfo = ref();
const upgradeVersion = ref();

const search = async () => {
    const res = await getSettingBaseInfo();
    version.value = res.data.systemVersion;
};

const toZoomtk = () => {
    window.open('https://www.zoomtk.com/workmesh', '_blank', 'noopener,noreferrer');
};

const onLoadUpgradeInfo = async () => {
    loading.value = true;
    await loadUpgradeInfo()
        .then((res) => {
            loading.value = false;
            if (res.data.testVersion || res.data.newVersion || res.data.latestVersion) {
                upgradeInfo.value = res.data;
                if (upgradeInfo.value.latestVersion) {
                    upgradeVersion.value = upgradeInfo.value.latestVersion;
                } else if (upgradeInfo.value.testVersion) {
                    upgradeVersion.value = upgradeInfo.value.testVersion;
                } else if (upgradeInfo.value.newVersion) {
                    upgradeVersion.value = upgradeInfo.value.newVersion;
                }
                upgradeRef.value.acceptParams({ upgradeInfo: upgradeInfo.value, upgradeVersion: upgradeVersion.value });
            } else {
                MsgSuccess(i18n.global.t('setting.noUpgrade'));
                return;
            }
        })
        .catch(() => {
            loading.value = false;
        });
};

onMounted(() => {
    search();
});
</script>

<style lang="scss" scoped>
.line-height {
    line-height: 25px;
}
:deep(.el-link__inner) {
    font-weight: 400;
}
.version {
    margin-left: 8px;
    font-size: 14px;
    color: var(--panel-color-primary-light-4);
    text-decoration: none;
    letter-spacing: 0.5px;
    cursor: pointer;
    font-family: auto;
}
</style>
