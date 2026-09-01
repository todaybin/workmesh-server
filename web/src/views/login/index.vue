<template>
    <div class="login-background" :style="backgroundStyle">
        <div
            v-if="externalLoginPending"
            v-loading="true"
            class="absolute inset-0 z-20 login-loading-mask"
            aria-busy="true"
        ></div>
        <div v-show="!externalLoginPending" class="login-wrapper">
            <div :class="isDesktopLayout ? 'left inline-block' : ''">
                <div class="login-title">
                    <span>{{ themeConfig.title || $t('setting.description') }}</span>
                </div>
                <img
                    v-if="isDesktopLayout"
                    v-show="imgLoaded"
                    :src="loadImage('loginImage')"
                    class="login-visual"
                    alt="WorkMesh"
                    @load="onImgLoad"
                    @error="onImgError"
                />
            </div>
            <div :class="isDesktopLayout ? 'right inline-block' : ''">
                <div id="login-container" class="login-container">
                    <LoginForm ref="loginRef" @external-login-ready="externalLoginPending = false"></LoginForm>
                </div>
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import LoginForm from './components/login-form.vue';
import { ref, onMounted } from 'vue';
import { useGlobalStore } from '@/composables/useGlobalStore';
import { preloadImage } from '@/utils/browser';
import { hasExternalLoginTicket } from '@/utils/external-login';
defineOptions({ name: 'Login' });
const { entrance, themeConfig } = useGlobalStore();
const defaultLoginImage = new URL('@/assets/images/workmesh-login-reference.png', import.meta.url).href;
const defaultLoginBgImage = new URL('@/assets/images/workmesh-login-bg-reference.png', import.meta.url).href;
const loadedLoginImage = ref<string | null>(null);
const loadedBackgroundImage = ref<string | null>(null);
const backgroundStyle = ref<Record<string, string>>({
    '--login-background-image': `url(${defaultLoginBgImage})`,
});
const imgLoaded = ref(false);
const currentDefaultLoginImage = computed(() => defaultLoginImage);

const externalLoginPending = ref(hasExternalLoginTicket());

function onImgLoad() {
    imgLoaded.value = true;
}
const mySafetyCode = defineProps({
    code: {
        type: String,
        default: '',
    },
});

const getStatus = async () => {
    let code = mySafetyCode.code;
    if (code != '') {
        entrance.value = code;
    }
};

const loadImage = (name: string) => {
    const { loginImage, loginBackground, loginBgType } = themeConfig.value;
    if (name === 'loginImage') {
        if (loginImage === 'loginImage') {
            return loadedLoginImage.value || currentDefaultLoginImage.value;
        }
        if (loginImage) {
            return loginImage;
        }
        return currentDefaultLoginImage.value;
    }
    if (name === 'loginBackground') {
        if (loginBgType === 'image') {
            if (loginBackground === 'loginBackground') {
                return loadedBackgroundImage.value || defaultLoginBgImage;
            }
            if (loginBackground) {
                return loginBackground;
            }
            return defaultLoginBgImage;
        }
        if (loginBgType === 'color') {
            return loginBackground;
        }
        return defaultLoginBgImage;
    }
    return '';
};

const onImgError = (event: any) => {
    event.target.src = currentDefaultLoginImage.value;
    imgLoaded.value = true;
};

onMounted(async () => {
    await getStatus();
    const loginImageUrl = `/api/v2/images/loginImage?t=${Date.now()}`;
    const backgroundImageUrl = `/api/v2/images/loginBackground?t=${Date.now()}`;
    if (themeConfig.value.loginImage === 'loginImage') {
        loadedLoginImage.value = await preloadImage(loginImageUrl);
    }
    if (themeConfig.value.loginBgType === 'image' && themeConfig.value.loginBackground === 'loginBackground') {
        loadedBackgroundImage.value = await preloadImage(backgroundImageUrl);
    }
    if (themeConfig.value.loginBgType === 'color') {
        backgroundStyle.value = {
            '--login-background-image': 'none',
            backgroundColor: themeConfig.value.loginBackground,
        };
    } else {
        const img = new Image();
        const url = loadImage('loginBackground');
        img.onload = () => {
            backgroundStyle.value = {
                '--login-background-image': `url(${url})`,
            };
        };
        img.onerror = () => {
            backgroundStyle.value = {
                '--login-background-image': `url(${defaultLoginBgImage})`,
            };
        };
        img.src = url;
    }
});

const useWindowSize = () => {
    const width = ref(window.innerWidth);
    const height = ref(window.innerHeight);

    const updateSize = () => {
        width.value = window.innerWidth;
        height.value = window.innerHeight;
    };

    onMounted(() => window.addEventListener('resize', updateSize));
    onUnmounted(() => window.removeEventListener('resize', updateSize));

    return { width, height };
};
const { width } = useWindowSize();
const isDesktopLayout = computed(() => width.value > 1110);
</script>

<style scoped lang="scss">
.login-background {
    position: relative;
    min-height: 100vh;
    width: 100%;
    overflow: auto;
    background:
        var(--login-background-image, none) no-repeat,
        radial-gradient(153.25% 257.2% at 118.99% 181.67%, rgba(50, 132, 255, 0.2), rgba(82, 120, 255, 0)),
        radial-gradient(123.54% 204.83% at 25.87% 195.17%, rgba(111, 76, 253, 0.15), rgba(122, 76, 253, 0) 78.85%),
        linear-gradient(0deg, rgba(0, 94, 235, 0.03), rgba(0, 94, 235, 0.03)),
        radial-gradient(109.58% 109.58% at 31.53% -36.58%, rgba(0, 94, 235, 0.3), rgba(0, 94, 235, 0)),
        rgba(0, 57, 142, 0.05);
}

.login-loading-mask {
    background: rgba(238, 245, 255, 0.92);
}

.login-wrapper {
    width: 80%;
    margin: 0 auto;
    padding-top: 8%;
    box-sizing: border-box;
}

.login-wrapper .left {
    vertical-align: middle;
    text-align: right;
    width: 60%;
}

.login-wrapper .left img {
    object-fit: contain;
    width: 100%;
}

.login-wrapper .right {
    vertical-align: middle;
    width: 40%;
}

.login-title {
    margin-right: 10%;
    text-align: right;
}

.login-title span {
    color: var(--el-color-primary);
    font-family: 'PingFang SC', 'Microsoft YaHei', sans-serif;
    font-size: 40px;
    font-weight: 600;
}

.login-container {
    box-sizing: border-box;
    width: 390px;
    margin-top: 40px;
    padding: 40px 0;
    border-radius: 4px;
    background: rgba(255, 255, 255, 0.55);
    box-shadow: 2px 4px 22px rgba(0, 94, 235, 0.2);
}

@media only screen and (min-width: 1440px) {
    .login-wrapper .left img {
        width: 85%;
    }
}

@media only screen and (max-width: 1440px) {
    .login-container {
        margin-top: 60px;
    }
}

@media only screen and (max-width: 1110px) {
    .login-title {
        margin-right: 0;
        margin-bottom: 20px;
        text-align: center;
    }

    .login-container {
        margin: 60px auto 0;
    }
}

@media only screen and (max-width: 768px) {
    .login-wrapper {
        width: calc(100% - 32px);
        padding-top: 32px;
        padding-bottom: 32px;
    }

    .login-title span {
        font-size: 35px;
    }

    .login-container {
        width: 100%;
    }
}
</style>
