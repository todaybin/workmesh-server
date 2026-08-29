<template>
    <div class="footer" :class="{ 'footer--mobile': isMobile }">
        <div class="flex w-full flex-col gap-4 md:justify-between md:flex-row">
            <div class="flex flex-wrap gap-4">
                <a href="https://www.zoomtk.com/workmesh" target="_blank">@{{ year }}-2050</a>
            </div>
            <div class="footer-actions">
                <FooterNavigation />
                <SystemUpgrade />
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import SystemUpgrade from '@/components/system-upgrade/index.vue';
import FooterNavigation from '@/components/footer-navigation/index.vue';
import { useGlobalStore } from '@/composables/useGlobalStore';

const { isMobile } = useGlobalStore();

const year = new Date().getFullYear();
</script>

<style scoped lang="scss">
.footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-height: 48px;
    height: auto;
    background: var(--panel-footer-bg);
    border-top: 1px solid var(--panel-footer-border);
    box-sizing: border-box;
    padding: 10px 20px;
    a {
        font-size: 12px;
        color: #858585;
        text-decoration: none;
        letter-spacing: 0.5px;
    }
    span {
        font-size: 12px;
        color: #858585;
        text-decoration: none;
        letter-spacing: 0.5px;
    }
}

.footer--mobile {
    min-height: 108px;
}

.footer-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    row-gap: 8px;
}

@media (max-width: 767px) {
    .footer-actions {
        column-gap: 8px;
        justify-content: center;
    }
}
</style>
