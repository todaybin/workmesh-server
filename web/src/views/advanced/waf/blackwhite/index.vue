<template>
    <div class="page">
        <div class="kind-tabs">
            <el-radio-group v-model="kind">
                <el-radio-button v-for="item in kinds" :key="item.key" :label="item.key">
                    {{ item.label }}
                </el-radio-button>
            </el-radio-group>
        </div>
        <section class="list-panel">
            <div class="list-layout">
                <div v-if="kind !== 'ipGroups'" class="side-menu">
                    <button
                        v-for="item in [
                            { key: 'blacklist', label: '黑名单' },
                            { key: 'whitelist', label: '白名单' },
                        ]"
                        :key="item.key"
                        type="button"
                        :class="{ active: listType === item.key }"
                        @click="listType = item.key"
                    >
                        {{ item.label }}
                    </button>
                </div>
                <div class="content">
                    <div class="list-toolbar">
                        <el-button plain @click="appendLine">创建</el-button>
                        <el-switch v-model="enabled" active-text="启用" @change="save" />
                        <span class="helper">{{ helper }}</span>
                    </div>
                    <el-table v-if="kind === 'ipGroups'" :data="groups" border>
                        <el-table-column prop="name" label="组名" width="220">
                            <template #default="{ row }"><el-input v-model="row.name" /></template>
                        </el-table-column>
                        <el-table-column label="IP / CIDR">
                            <template #default="{ row }">
                                <el-input
                                    v-model="row.entriesText"
                                    type="textarea"
                                    :rows="4"
                                    resize="vertical"
                                    placeholder="每行一个 IP 或 CIDR"
                                />
                            </template>
                        </el-table-column>
                        <el-table-column label="状态" width="100">
                            <template #default="{ row }"><el-switch v-model="row.enabled" /></template>
                        </el-table-column>
                        <el-table-column label="操作" width="90">
                            <template #default="{ $index }">
                                <el-button link type="danger" @click="groups.splice($index, 1)">删除</el-button>
                            </template>
                        </el-table-column>
                    </el-table>
                    <el-table v-else :data="visibleRows" border>
                        <el-table-column label="类型" width="130">
                            <template #default>{{ kindLabel }}</template>
                        </el-table-column>
                        <el-table-column label="规则" min-width="300">
                            <template #default="{ row }">
                                <el-input v-model="row.value" :placeholder="placeholder" />
                            </template>
                        </el-table-column>
                        <el-table-column label="状态" width="100">
                            <template #default="{ row }">
                                <el-switch v-model="row.enabled" />
                            </template>
                        </el-table-column>
                        <el-table-column label="备注" min-width="200">
                            <template #default="{ row }">
                                <el-input v-model="row.remark" placeholder="备注" />
                            </template>
                        </el-table-column>
                        <el-table-column label="操作" width="90">
                            <template #default="{ row }">
                                <el-button link type="danger" @click="removeRow(row)">删除</el-button>
                            </template>
                        </el-table-column>
                    </el-table>
                    <el-pagination
                        v-if="kind !== 'ipGroups'"
                        v-model:current-page="page"
                        v-model:page-size="pageSize"
                        class="pagination"
                        :page-sizes="[20, 50, 100]"
                        :total="rows.length"
                        layout="total, sizes, prev, pager, next, jumper"
                    />
                    <div class="footer">
                        <el-button type="primary" @click="save">保存</el-button>
                        <span>保存后会执行 OpenResty 配置检查并 reload，失败时不会提交。</span>
                    </div>
                </div>
            </div>
        </section>
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { ElMessage } from 'element-plus';
import { getWafAccessLists, getWafStatus, updateWafAccessLists, type WafAccessLists } from '@/api/modules/waf';
const kinds = [
    { key: 'ip', label: 'IP' },
    { key: 'url', label: 'URL' },
    { key: 'ua', label: 'User-Agent' },
    { key: 'ipGroups', label: 'IP 组' },
];
const kind = ref('ip');
const listType = ref('blacklist');
const enabled = ref(true);
const data = ref<WafAccessLists>({ whitelist: [], blacklist: [], enabled: {} } as WafAccessLists);
const rows = ref<Array<{ value: string; enabled: boolean; remark: string }>>([]);
const groups = ref<Array<{ name: string; entriesText: string; entries: string[]; enabled?: boolean }>>([]);
const page = ref(1);
const pageSize = ref(20);
const listKey = computed(() => {
    if (kind.value === 'url') return listType.value === 'blacklist' ? 'urlBlacklist' : 'urlWhitelist';
    if (kind.value === 'ua') return listType.value === 'blacklist' ? 'uaBlacklist' : 'uaWhitelist';
    return listType.value;
});
const enabledKey = computed(() => (kind.value === 'ipGroups' ? 'ipGroups' : listKey.value));
const helper = computed(() =>
    kind.value === 'ip'
        ? '黑名单中的 IP 无法访问网站'
        : kind.value === 'url'
          ? '匹配 URL 的请求将被拦截或放行'
          : kind.value === 'ua'
            ? '匹配 User-Agent 的请求将被拦截或放行'
            : 'IP 组可用于自定义规则匹配',
);
const kindLabel = computed(() => (kind.value === 'ip' ? 'IP' : kind.value === 'url' ? 'URL' : 'User-Agent'));
const visibleRows = computed(() => rows.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value));
const placeholder = computed(() =>
    kind.value === 'ip'
        ? '每行一个 IP 或 CIDR'
        : kind.value === 'url'
          ? '每行一个 URL 或路径'
          : '每行一个 User-Agent 关键字',
);
const listMetaKey = (value: string) => `${listKey.value}:${value}`;
const normalizeEntries = (value: unknown): string[] => {
    if (Array.isArray(value)) {
        return value
            .flatMap((item) => String(item ?? '').split(/\r?\n|,|;/))
            .map((item) => item.trim())
            .filter(Boolean);
    }
    return String(value ?? '')
        .split(/\r?\n|,|;/)
        .map((item) => item.trim())
        .filter(Boolean);
};
const load = async () => {
    const result: any = await getWafAccessLists();
    data.value = result.data || data.value;
    enabled.value = data.value.enabled?.[enabledKey.value] !== false;
    if (kind.value === 'ipGroups') {
        groups.value = (data.value.ipGroups || []).map((group) => ({
            ...group,
            entries: normalizeEntries(group.entries),
            entriesText: normalizeEntries(group.entries).join('\n'),
        }));
    } else {
        const values = (data.value as any)[listKey.value];
        const meta = data.value.listMeta || {};
        rows.value = (Array.isArray(values) ? values : []).map((value: string) => {
            const item = meta[listMetaKey(value)];
            return { value, enabled: item?.enabled !== false, remark: item?.remark || '' };
        });
        page.value = 1;
    }
};
const appendLine = () => {
    if (kind.value === 'ipGroups') groups.value.push({ name: '', entries: [], entriesText: '', enabled: true });
    else {
        rows.value.push({ value: '', enabled: true, remark: '' });
        page.value = Math.ceil(rows.value.length / pageSize.value);
    }
};
const removeRow = (row: { value: string; enabled: boolean; remark: string }) => {
    const index = rows.value.indexOf(row);
    if (index >= 0) rows.value.splice(index, 1);
    if (page.value > Math.max(1, Math.ceil(rows.value.length / pageSize.value))) {
        page.value = Math.max(1, Math.ceil(rows.value.length / pageSize.value));
    }
};
const save = async () => {
    const previous = JSON.parse(JSON.stringify(data.value)) as WafAccessLists;
    const next: WafAccessLists = {
        ...data.value,
        enabled: { ...(data.value.enabled || {}), [enabledKey.value]: enabled.value },
    };
    if (kind.value === 'ipGroups') {
        next.ipGroups = groups.value.map((group) => ({
            name: group.name.trim(),
            entries: normalizeEntries(group.entriesText),
            enabled: group.enabled,
        }));
    } else {
        const normalized = rows.value.map((row) => ({ ...row, value: row.value.trim() })).filter((row) => row.value);
        (next as any)[listKey.value] = normalized.map((row) => row.value);
        const listMeta = { ...(next.listMeta || {}) };
        const prefix = `${listKey.value}:`;
        for (const key of Object.keys(listMeta)) {
            if (key.startsWith(prefix)) delete listMeta[key];
        }
        for (const row of normalized) {
            listMeta[`${prefix}${row.value}`] = { enabled: row.enabled, remark: row.remark.trim() };
        }
        next.listMeta = listMeta;
    }
    try {
        const result: any = await updateWafAccessLists(next);
        data.value = result.data || next;
        const status: any = await getWafStatus();
        if (status.data?.effective === true) {
            ElMessage.success('已保存并确认 OpenResty 生效');
        } else {
            ElMessage.warning(status.data?.error || '已保存，但尚未确认 OpenResty 生效');
        }
    } catch (error: any) {
        data.value = previous;
        await load();
        ElMessage.error(error?.message || '黑白名单保存失败');
    }
};
watch([kind, listType], load);
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
.kind-tabs,
.list-panel {
    background: var(--el-bg-color);
    border: 1px solid var(--el-border-color-light);
}
.kind-tabs {
    padding: 20px 24px 10px;
}
.kind-tabs :deep(.el-radio-button__inner) {
    min-width: 74px;
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
    border-color: var(--el-color-primary-light-5);
    box-shadow: none;
}
.kind-tabs :deep(.el-radio-button:first-child .el-radio-button__inner) {
    border-left: 1px solid var(--el-color-primary-light-5);
}
.kind-tabs :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
    color: #fff;
    background: var(--el-color-primary);
    border-color: var(--el-color-primary);
    box-shadow: -1px 0 0 0 var(--el-color-primary);
}
.list-panel {
    padding: 12px 14px;
}
.list-layout {
    display: flex;
    min-height: 280px;
}
.side-menu {
    width: 100px;
    flex: 0 0 100px;
    border-right: 1px solid var(--el-border-color-light);
}
.side-menu button {
    display: block;
    width: 100%;
    min-height: 44px;
    padding: 0 12px;
    color: var(--el-text-color-primary);
    background: transparent;
    border: 0;
    border-right: 2px solid transparent;
    text-align: left;
    cursor: pointer;
}
.side-menu button.active {
    color: var(--el-color-primary);
    border-right-color: var(--el-color-primary);
    font-weight: 500;
}
.content {
    flex: 1;
    padding-left: 14px;
}
.list-toolbar {
    display: flex;
    align-items: center;
    gap: 14px;
    margin-bottom: 14px;
}
.helper {
    color: var(--el-text-color-secondary);
}
.footer {
    display: flex;
    align-items: center;
    gap: 14px;
    margin-top: 14px;
    color: var(--el-text-color-secondary);
    font-size: 13px;
}
.page :deep(.el-empty__image) {
    display: none;
}
</style>
