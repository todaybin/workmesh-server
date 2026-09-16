<template>
    <div class="page">
        <section class="table-panel">
            <div class="toolbar">
                <el-input
                    v-model="search"
                    clearable
                    :placeholder="$t('xpack.waf.websiteSearchHelper')"
                    class="search"
                />
                <el-button :icon="Refresh" @click="load" />
            </div>
            <el-table v-if="pagedRows.length || loading" :data="pagedRows" stripe v-loading="loading">
                <el-table-column :label="$t('menu.website')" min-width="240">
                    <template #default="{ row }">{{ row.primaryDomain || row.alias || row.websiteID }}</template>
                </el-table-column>
                <el-table-column :label="$t('xpack.waf.name')" min-width="160">
                    <template #default="{ row }">{{ row.remark || '-' }}</template>
                </el-table-column>
                <el-table-column :label="$t('xpack.waf.websiteSwitch')" width="110">
                    <template #default="{ row }"><el-switch v-model="row.enabled" @change="save(row)" /></template>
                </el-table-column>
                <el-table-column label="执行策略" width="190">
                    <template #default="{ row }">
                        <el-radio-group v-model="row.mode" size="small" @change="save(row)">
                            <el-radio-button label="block">防护模式</el-radio-button>
                            <el-radio-button label="observe">观察模式</el-radio-button>
                        </el-radio-group>
                    </template>
                </el-table-column>
                <el-table-column label="检测强度" width="180">
                    <template #default="{ row }">
                        <el-radio-group v-model="row.detectionLevel" size="small" @change="save(row)">
                            <el-radio-button :label="1">标准模式</el-radio-button>
                            <el-radio-button :label="2">严格模式</el-radio-button>
                        </el-radio-group>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('xpack.waf.frequencyLimit')" width="110">
                    <template #default="{ row }">
                        <el-switch v-model="row.frequencyEnabled" @change="save(row)" />
                    </template>
                </el-table-column>
                <el-table-column label="操作" width="110">
                    <template #default="{ row }">
                        <el-button link type="primary" @click="detail(row)">详细设置</el-button>
                    </template>
                </el-table-column>
            </el-table>
            <el-empty v-else :description="$t('commons.noData')" />
            <div class="pager">
                <span>共 {{ filteredRows.length }} 条</span>
                <el-pagination
                    v-model:current-page="page"
                    v-model:page-size="pageSize"
                    :total="filteredRows.length"
                    layout="total, sizes, prev, pager, next, jumper"
                />
            </div>
        </section>
        <el-dialog
            v-model="detailVisible"
            :title="`${detailWebsite?.primaryDomain || detailWebsite?.alias || ''} WAF 详细设置`"
            width="min(1000px, calc(100vw - 32px))"
            class="detail-dialog"
        >
            <div class="detail-toolbar">
                <span>站点规则会写入该网站的 WAF 配置，并在保存后执行 OpenResty 校验和 reload。</span>
                <el-button type="primary" @click="addRule">新增规则</el-button>
            </div>
            <el-table :data="detailRules" border class="detail-table" v-loading="detailLoading">
                <el-table-column label="规则名称" min-width="140">
                    <template #default="{ row }"><el-input v-model="row.name" /></template>
                </el-table-column>
                <el-table-column label="位置" width="110">
                    <template #default="{ row }">
                        <el-select v-model="row.location">
                            <el-option label="URL" value="uri" />
                            <el-option label="IP 组" value="ip-group" />
                            <el-option label="参数" value="args" />
                            <el-option label="请求头" value="header" />
                            <el-option label="请求体" value="body" />
                        </el-select>
                    </template>
                </el-table-column>
                <el-table-column label="匹配值" min-width="220">
                    <template #default="{ row }"><el-input v-model="row.value" /></template>
                </el-table-column>
                <el-table-column label="动作" width="110">
                    <template #default="{ row }">
                        <el-select v-model="row.action">
                            <el-option label="放行" value="allow" />
                            <el-option label="记录" value="log" />
                            <el-option label="拦截" value="block" />
                        </el-select>
                    </template>
                </el-table-column>
                <el-table-column label="优先级" width="100">
                    <template #default="{ row }">
                        <el-input-number v-model="row.priority" :min="1" :max="10000" />
                    </template>
                </el-table-column>
                <el-table-column label="启用" width="80">
                    <template #default="{ row }"><el-switch v-model="row.enabled" /></template>
                </el-table-column>
                <el-table-column label="操作" width="130">
                    <template #default="{ row, $index }">
                        <el-button link type="primary" @click="saveRule(row)">保存</el-button>
                        <el-button link type="danger" @click="removeRule(row, $index)">删除</el-button>
                    </template>
                </el-table-column>
            </el-table>
            <template #footer>
                <el-button @click="detailVisible = false">关闭</el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Refresh } from '@element-plus/icons-vue';
import {
    deleteWafRule,
    getWafStatus,
    listWafRules,
    listWafSites,
    upsertWafRule,
    updateWafSite,
    type WafRule,
    type WafSiteConfig,
} from '@/api/modules/waf';
import { listWebsites } from '@/api/modules/website';

type SiteRow = WafSiteConfig & {
    primaryDomain: string;
    remark: string;
    detectionLevel: number;
    frequencyEnabled: boolean;
};
const loading = ref(false);
const search = ref('');
const page = ref(1);
const pageSize = ref(20);
const rows = ref<SiteRow[]>([]);
const savedRows = new Map<number, SiteRow>();
const detailVisible = ref(false);
const detailLoading = ref(false);
const detailWebsite = ref<SiteRow>();
const detailRules = ref<WafRule[]>([]);
const filteredRows = computed(() => {
    const keyword = search.value.trim().toLowerCase();
    if (!keyword) return rows.value;
    return rows.value.filter((row) =>
        [row.primaryDomain, row.alias, row.remark].some((value) =>
            String(value || '')
                .toLowerCase()
                .includes(keyword),
        ),
    );
});
const pagedRows = computed(() =>
    filteredRows.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value),
);
const load = async () => {
    loading.value = true;
    page.value = 1;
    try {
        const [wafResult, websiteResult]: any = await Promise.all([listWafSites(), listWebsites()]);
        const websites = websiteResult.data || [];
        rows.value = (wafResult.data || []).map((item: WafSiteConfig) => {
            const website = websites.find((candidate: any) => Number(candidate.id) === item.websiteID);
            return {
                ...item,
                primaryDomain: item.primaryDomain || website?.primaryDomain || '',
                remark: item.remark || website?.remark || '',
                detectionLevel: item.detectionLevel || 1,
                frequencyEnabled: item.frequencyEnabled !== false,
            };
        });
        savedRows.clear();
        rows.value.forEach((row) => savedRows.set(row.websiteID, { ...row }));
        if (page.value > 1 && (page.value - 1) * pageSize.value >= filteredRows.value.length) page.value = 1;
    } catch (error: any) {
        rows.value = [];
        ElMessage.error(error?.message || '网站 WAF 设置加载失败');
    } finally {
        loading.value = false;
    }
};
const confirmEffective = async () => {
    const result: any = await getWafStatus();
    if (result.data?.effective === true) {
        ElMessage.success('已保存并确认 OpenResty 生效');
    } else {
        ElMessage.warning(result.data?.error || '已保存，但尚未确认 OpenResty 生效');
    }
};
const save = async (row: SiteRow) => {
    const previous = savedRows.get(row.websiteID);
    try {
        await updateWafSite({
            websiteID: row.websiteID,
            enabled: row.enabled,
            mode: row.mode,
            frequencyEnabled: row.frequencyEnabled,
            detectionLevel: row.detectionLevel,
        });
        savedRows.set(row.websiteID, { ...row });
        await confirmEffective();
    } catch (error: any) {
        if (previous) Object.assign(row, previous);
        ElMessage.error(error?.message || '网站 WAF 设置保存失败');
    }
};
const detail = async (row: SiteRow) => {
    detailWebsite.value = row;
    detailVisible.value = true;
    detailLoading.value = true;
    try {
        await reloadDetailRules(row.websiteID);
    } finally {
        detailLoading.value = false;
    }
};
const reloadDetailRules = async (websiteID: number) => {
    const result: any = await listWafRules(websiteID);
    detailRules.value = (result.data || []).map((rule: WafRule) => ({ ...rule }));
};
const addRule = () => {
    if (!detailWebsite.value) return;
    detailRules.value.push({
        websiteID: detailWebsite.value.websiteID,
        name: '',
        location: 'uri',
        operator: 'contains',
        value: '',
        action: 'block',
        priority: 100,
        enabled: true,
    });
};
const saveRule = async (rule: WafRule) => {
    if (!detailWebsite.value || !rule.name.trim() || !rule.value.trim()) {
        ElMessage.warning('规则名称和匹配值不能为空');
        return;
    }
    const websiteID = detailWebsite.value.websiteID;
    try {
        await upsertWafRule({ ...rule, websiteID });
        await confirmEffective();
        await reloadDetailRules(websiteID);
    } catch (error: any) {
        await reloadDetailRules(websiteID);
        ElMessage.error(error?.message || '站点规则保存失败');
    }
};
const removeRule = async (rule: WafRule, index: number) => {
    if (!rule.id) {
        detailRules.value.splice(index, 1);
        return;
    }
    await ElMessageBox.confirm('确认删除这条站点 WAF 规则？', '删除规则');
    const websiteID = detailWebsite.value?.websiteID || rule.websiteID;
    try {
        await deleteWafRule({ websiteID, id: rule.id });
        await confirmEffective();
        await reloadDetailRules(websiteID);
    } catch (error: any) {
        await reloadDetailRules(websiteID);
        ElMessage.error(error?.message || '站点规则删除失败');
    }
};
watch(search, () => {
    page.value = 1;
});
onMounted(() => {
    load();
    window.addEventListener('workmesh:waf-refresh', load);
});
onBeforeUnmount(() => window.removeEventListener('workmesh:waf-refresh', load));
</script>

<style scoped>
.page {
    display: flex;
    flex-direction: column;
    gap: 10px;
}
.table-panel {
    padding: 18px 12px 12px;
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-light);
    border-radius: 4px;
}
.toolbar {
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    margin-bottom: 18px;
}
.search {
    width: 396px;
}
.pager {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 16px;
    margin-top: 16px;
    color: var(--el-text-color-secondary);
}
.detail-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 12px;
    color: var(--el-text-color-secondary);
}
.detail-dialog :deep(.el-dialog__body) {
    overflow-x: auto;
}
.detail-table {
    min-width: 820px;
}
.page :deep(.el-empty__image) {
    display: none;
}
@media (max-width: 900px) {
    .search {
        width: min(100%, 396px);
    }
    .toolbar {
        justify-content: stretch;
    }
    .search {
        flex: 1;
    }
    .detail-toolbar {
        align-items: flex-start;
        flex-direction: column;
    }
}
</style>
