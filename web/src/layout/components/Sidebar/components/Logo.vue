<template>
    <div class="logo" style="cursor: pointer" @click="goHome">
        <template v-if="isCollapse">
            <img
                v-if="themeConfig.logo && !logoLoadFailed"
                :src="`/api/v2/images/logo?t=${Date.now()}`"
                style="cursor: pointer"
                alt="logo"
                @error="logoLoadFailed = true"
            />
            <span class="workmesh-logo-collapsed" aria-label="WorkMesh">
                <img class="workmesh-logo workmesh-logo--collapsed" :src="workmeshLogo" alt="WorkMesh" />
            </span>
        </template>
        <template v-else>
            <img
                v-if="themeConfig.logoWithText && !logoWithTextLoadFailed"
                :src="`/api/v2/images/logoWithText?t=${Date.now()}`"
                style="cursor: pointer"
                alt="logo"
                @error="logoWithTextLoadFailed = true"
            />
            <img class="workmesh-logo" :src="workmeshLogo" alt="WorkMesh" />
        </template>
    </div>
</template>

<script setup lang="ts">
import { useGlobalStore } from '@/composables/useGlobalStore';
import workmeshLogo from '@/assets/images/workmesh-logo.svg?url&no-inline';
import { ref } from 'vue';
import { routerToNameWithQuery } from '@/utils/router';

defineProps<{ isCollapse: boolean }>();

const logoLoadFailed = ref(false);
const logoWithTextLoadFailed = ref(false);
const { themeConfig } = useGlobalStore();

const goHome = () => {
    routerToNameWithQuery('home', { t: Date.now() });
};
</script>

<style scoped lang="scss">
.logo {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 49px;
    box-sizing: border-box;
    background-color: #fff;
    z-index: 1;
    img {
        object-fit: contain;
        width: 95%;
        height: 45px;
    }

    .workmesh-logo {
        width: 120px;
        height: 30px;
    }

    .workmesh-logo-collapsed {
        display: block;
        width: 30px;
        height: 30px;
        overflow: hidden;
    }

    .workmesh-logo--collapsed {
        max-width: none;
        object-fit: contain;
        object-position: left center;
    }
}
</style>
