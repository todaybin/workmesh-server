<template>
    <LayoutContent :title="$t('menu.waf')" v-loading="loading">
        <template #rightToolBar><TableRefresh @search="load" /></template>
        <template #main>
            <el-tabs v-model="activeTab" class="waf-tabs">
                <el-tab-pane label="概况" name="dashboard">
                    <el-row :gutter="12" class="mb-4">
                        <el-col v-for="card in auditCards" :key="card.label" :span="6">
                            <el-card shadow="never"><el-statistic :title="card.label" :value="card.value" /></el-card>
                        </el-col>
                    </el-row>
                    <el-card shadow="never">
                        <template #header>最近攻击</template>
                        <el-table :data="auditRecords" stripe>
                            <el-table-column prop="time" label="时间" width="180" />
                            <el-table-column prop="client_ip" label="来源 IP" width="150" />
                            <el-table-column prop="host" label="网站" width="180" />
                            <el-table-column prop="uri" label="请求" min-width="240" />
                            <el-table-column prop="rule" label="规则" width="150" />
                            <el-table-column prop="action" label="动作" width="90" />
                        </el-table>
                    </el-card>
                </el-tab-pane>
                <el-tab-pane label="攻击报表" name="attack">
                    <el-card shadow="never">
                        <el-table :data="auditRecords" stripe>
                            <el-table-column prop="time" label="时间" width="180" />
                            <el-table-column prop="client_ip" label="来源 IP" width="150" />
                            <el-table-column prop="host" label="网站" width="180" />
                            <el-table-column prop="rule" label="规则" width="150" />
                            <el-table-column prop="message" label="详情" min-width="260" />
                        </el-table>
                    </el-card>
                </el-tab-pane>
                <el-tab-pane label="拦截记录" name="logs">
                    <el-card shadow="never">
                        <el-table :data="blockedRecords" stripe>
                            <el-table-column prop="time" label="时间" width="180" />
                            <el-table-column prop="client_ip" label="来源 IP" width="150" />
                            <el-table-column prop="uri" label="URI" min-width="260" />
                            <el-table-column prop="status" label="状态" width="90" />
                            <el-table-column prop="rule" label="规则" width="150" />
                        </el-table>
                    </el-card>
                </el-tab-pane>
                <el-tab-pane label="封锁记录" name="blocks">
                    <el-card shadow="never">
                        <el-table :data="blockedRecords" stripe>
                            <el-table-column prop="time" label="时间" width="180" />
                            <el-table-column prop="client_ip" label="来源 IP" width="160" />
                            <el-table-column prop="host" label="网站" width="180" />
                            <el-table-column prop="reason" label="封锁原因" min-width="240" />
                            <el-table-column label="状态" width="100">
                                <template #default>已封锁</template>
                            </el-table-column>
                        </el-table>
                    </el-card>
                </el-tab-pane>
                <el-tab-pane label="黑白名单" name="lists">
                    <el-card shadow="never">
                        <el-alert
                            type="info"
                            :closable="false"
                            title="每行一个 IP 或 CIDR 网段，白名单优先于拦截规则。"
                            class="mb-4"
                        />
                        <el-row :gutter="24">
                            <el-col :span="12">
                                <el-form-item label="白名单">
                                    <el-input
                                        v-model="listForm.whitelistText"
                                        type="textarea"
                                        :rows="12"
                                        placeholder="192.168.1.0/24"
                                    />
                                </el-form-item>
                            </el-col>
                            <el-col :span="12">
                                <el-form-item label="黑名单">
                                    <el-input
                                        v-model="listForm.blacklistText"
                                        type="textarea"
                                        :rows="12"
                                        placeholder="203.0.113.10"
                                    />
                                </el-form-item>
                            </el-col>
                        </el-row>
                        <el-button type="primary" @click="saveLists">保存黑白名单</el-button>
                    </el-card>
                </el-tab-pane>
                <el-tab-pane label="网站设置" name="sites" />
                <el-tab-pane label="全局设置" name="global" />
            </el-tabs>
            <el-alert
                v-if="status && !status.available"
                type="error"
                :closable="false"
                title="WAF 运行时不可用，请先安装带 ModSecurity 和 CRS 的 OpenResty 镜像。"
            />
            <el-card v-else-if="activeTab === 'global'" class="mb-4" shadow="never">
                <template #header><span>全局策略</span></template>
                <el-form :inline="true" :model="global">
                    <el-form-item label="WAF"><el-switch v-model="global.enabled" /></el-form-item>
                    <el-form-item label="标准 CRS 规则"><el-switch v-model="global.standardRules" /></el-form-item>
                    <el-form-item label="模式">
                        <el-radio-group v-model="global.mode">
                            <el-radio-button label="observe">观察</el-radio-button>
                            <el-radio-button label="block">拦截</el-radio-button>
                        </el-radio-group>
                    </el-form-item>
                    <el-form-item label="Paranoia Level">
                        <el-input-number v-model="global.paranoiaLevel" :min="1" :max="4" />
                    </el-form-item>
                    <el-form-item label="异常评分阈值">
                        <el-input-number v-model="global.inboundThreshold" :min="1" :max="99" />
                    </el-form-item>
                    <el-form-item label="请求体限制(MB)">
                        <el-input-number v-model="bodyLimitMB" :min="0" :max="10" />
                    </el-form-item>
                    <el-form-item><el-button type="primary" @click="saveGlobal">保存策略</el-button></el-form-item>
                </el-form>
                <div class="runtime-info">
                    运行时：{{ status?.runtime }}　 CRS：{{ status?.crs }}　内置规则：{{ standardRules.length }} 条
                </div>
            </el-card>

            <el-card v-if="activeTab === 'sites'" shadow="never" class="mb-4">
                <template #header><span>网站保护</span></template>
                <el-table :data="sites" stripe>
                    <el-table-column prop="alias" label="网站" min-width="180" />
                    <el-table-column label="模式" width="140">
                        <template #default="{ row }">
                            <el-select v-model="row.mode" size="small" @change="saveSite(row)">
                                <el-option label="观察" value="observe" />
                                <el-option label="拦截" value="block" />
                            </el-select>
                        </template>
                    </el-table-column>
                    <el-table-column label="启用" width="90">
                        <template #default="{ row }">
                            <el-switch v-model="row.enabled" @change="saveSite(row)" />
                        </template>
                    </el-table-column>
                    <el-table-column label="自定义规则" width="110">
                        <template #default="{ row }">{{ row.rules?.length || 0 }}</template>
                    </el-table-column>
                    <el-table-column label="操作" width="220">
                        <template #default="{ row }">
                            <el-button link type="primary" @click="openRules(row)">规则管理</el-button>
                            <el-button link type="primary" @click="openTest(row)">拦截测试</el-button>
                        </template>
                    </el-table-column>
                </el-table>
            </el-card>

            <el-card v-if="activeTab === 'sites'" shadow="never">
                <template #header><span>标准规则覆盖</span></template>
                <el-table :data="standardRules" stripe size="small">
                    <el-table-column prop="id" label="规则 ID" width="140" />
                    <el-table-column prop="category" label="类别" width="140" />
                    <el-table-column prop="description" label="检测内容" />
                    <el-table-column prop="locations" label="检查位置" width="220">
                        <template #default="{ row }">{{ row.locations.join('、') }}</template>
                    </el-table-column>
                </el-table>
            </el-card>

            <el-dialog v-model="rulesOpen" :title="`规则管理：${selected?.alias || ''}`" width="900px">
                <div class="toolbar"><el-button type="primary" @click="newRule">新增自定义规则</el-button></div>
                <el-table :data="rules" stripe>
                    <el-table-column prop="name" label="名称" min-width="160" />
                    <el-table-column prop="location" label="位置" width="90" />
                    <el-table-column prop="operator" label="匹配" width="90" />
                    <el-table-column prop="action" label="动作" width="90" />
                    <el-table-column prop="priority" label="优先级" width="80" />
                    <el-table-column label="启用" width="80">
                        <template #default="{ row }">
                            <el-switch v-model="row.enabled" @change="saveRule(row)" />
                        </template>
                    </el-table-column>
                    <el-table-column label="操作" width="90">
                        <template #default="{ row }">
                            <el-button link type="danger" @click="removeRule(row)">删除</el-button>
                        </template>
                    </el-table-column>
                </el-table>
            </el-dialog>

            <el-dialog v-model="ruleEditorOpen" title="自定义 WAF 规则" width="560px">
                <el-form :model="editingRule" label-width="90px">
                    <el-form-item label="名称"><el-input v-model="editingRule.name" maxlength="120" /></el-form-item>
                    <el-form-item label="位置">
                        <el-select v-model="editingRule.location">
                            <el-option v-for="item in locations" :key="item" :label="item" :value="item" />
                        </el-select>
                    </el-form-item>
                    <el-form-item
                        v-if="editingRule.location === 'header' || editingRule.location === 'cookie'"
                        label="字段"
                    >
                        <el-input v-model="editingRule.key" placeholder="User-Agent" />
                    </el-form-item>
                    <el-form-item label="匹配方式">
                        <el-select v-model="editingRule.operator">
                            <el-option label="包含" value="contains" />
                            <el-option label="正则" value="regex" />
                            <el-option label="等于" value="equals" />
                            <el-option label="IP 网段" value="ip-cidr" />
                        </el-select>
                    </el-form-item>
                    <el-form-item label="值">
                        <el-input v-model="editingRule.value" type="textarea" maxlength="2048" />
                    </el-form-item>
                    <el-form-item label="动作">
                        <el-radio-group v-model="editingRule.action">
                            <el-radio-button label="log">记录</el-radio-button>
                            <el-radio-button label="block">拦截</el-radio-button>
                            <el-radio-button label="allow">放行</el-radio-button>
                        </el-radio-group>
                    </el-form-item>
                    <el-form-item label="优先级">
                        <el-input-number v-model="editingRule.priority" :min="1" :max="10000" />
                    </el-form-item>
                </el-form>
                <template #footer>
                    <el-button @click="ruleEditorOpen = false">取消</el-button>
                    <el-button type="primary" @click="saveNewRule">保存</el-button>
                </template>
            </el-dialog>

            <el-dialog v-model="testOpen" title="WAF 拦截测试" width="720px">
                <el-form :model="testRequest" label-width="90px">
                    <el-form-item label="方法">
                        <el-select v-model="testRequest.method">
                            <el-option
                                v-for="item in ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'TRACE']"
                                :key="item"
                                :label="item"
                                :value="item"
                            />
                        </el-select>
                    </el-form-item>
                    <el-form-item label="URI"><el-input v-model="testRequest.uri" /></el-form-item>
                    <el-form-item label="参数">
                        <el-input v-model="testRequest.args" placeholder="id=1%20union%20select%201" />
                    </el-form-item>
                    <el-form-item label="User-Agent"><el-input v-model="testUserAgent" /></el-form-item>
                    <el-form-item label="请求体">
                        <el-input v-model="testRequest.body" type="textarea" :rows="3" />
                    </el-form-item>
                </el-form>
                <el-alert
                    v-if="testResult"
                    :type="testResult.blocked ? 'error' : 'success'"
                    :title="testResult.blocked ? `预期拦截（${testResult.status}）` : '未触发拦截（200）'"
                    :closable="false"
                />
                <el-table v-if="testResult" :data="testResult.matches" class="mt-3" size="small">
                    <el-table-column prop="id" label="规则" width="150" />
                    <el-table-column prop="source" label="来源" width="100" />
                    <el-table-column prop="name" label="说明" />
                </el-table>
                <template #footer>
                    <el-button @click="testOpen = false">关闭</el-button>
                    <el-button type="primary" @click="runTest">执行测试</el-button>
                </template>
            </el-dialog>
        </template>
    </LayoutContent>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { ElMessage } from 'element-plus';
import {
    deleteWafRule,
    getWafStatus,
    listWafRules,
    listWafSites,
    listWafStandardRules,
    testWafRules,
    updateWafGlobal,
    updateWafSite,
    upsertWafRule,
    getWafAccessLists,
    updateWafAccessLists,
    getWafAudit,
    type WafGlobalConfig,
    type WafRule,
    type WafSiteConfig,
    type WafStandardRule,
    type WafStatus,
    type WafTestRequest,
    type WafTestResult,
} from '@/api/modules/waf';

const loading = ref(false);
const activeTab = ref('dashboard');
const status = ref<WafStatus>();
const sites = ref<WafSiteConfig[]>([]);
const standardRules = ref<WafStandardRule[]>([]);
const rules = ref<WafRule[]>([]);
const selected = ref<WafSiteConfig>();
const rulesOpen = ref(false);
const ruleEditorOpen = ref(false);
const testOpen = ref(false);
const testResult = ref<WafTestResult>();
const global = reactive<WafGlobalConfig>({
    enabled: true,
    standardRules: true,
    mode: 'observe',
    paranoiaLevel: 1,
    inboundThreshold: 5,
    requestBodyLimit: 1048576,
});
const bodyLimitMB = ref(1);
const locations = ['ip', 'uri', 'args', 'header', 'cookie', 'method', 'body'];
const editingRule = reactive<WafRule>({
    websiteID: 0,
    name: '',
    location: 'uri',
    operator: 'contains',
    value: '',
    action: 'block',
    priority: 100,
    enabled: true,
});
const testRequest = reactive<WafTestRequest>({
    websiteID: 0,
    method: 'GET',
    uri: '/',
    args: '',
    headers: {},
    cookies: {},
    body: '',
});
const testUserAgent = ref('');
const auditRecords = ref<Array<Record<string, any>>>([]);
const listForm = reactive({ whitelistText: '', blacklistText: '' });
const auditCards = computed(() => {
    const total = auditRecords.value.length;
    const blocked = auditRecords.value.filter((item) => item.action === 'block' || item.disruptive === true).length;
    const ips = new Set(auditRecords.value.map((item) => item.client_ip || item.ip).filter(Boolean)).size;
    return [
        { label: '攻击总数', value: total },
        { label: '拦截次数', value: blocked },
        { label: '攻击 IP', value: ips },
        { label: '规则命中', value: total },
    ];
});
const blockedRecords = computed(() =>
    auditRecords.value.filter((item) => item.action === 'block' || item.disruptive === true),
);
const load = async () => {
    loading.value = true;
    try {
        const [s, ws, sr, audit, lists] = await Promise.all([
            getWafStatus(),
            listWafSites(),
            listWafStandardRules(),
            getWafAudit('attack'),
            getWafAccessLists(),
        ]);
        status.value = s.data;
        sites.value = ws.data || [];
        standardRules.value = sr.data || [];
        auditRecords.value = audit.data?.items || [];
        listForm.whitelistText = (lists.data?.whitelist || []).join('\n');
        listForm.blacklistText = (lists.data?.blacklist || []).join('\n');
        Object.assign(global, s.data);
        bodyLimitMB.value = Math.round(((global.requestBodyLimit || 1048576) / 1024 / 1024) * 10) / 10;
    } finally {
        loading.value = false;
    }
};
const saveLists = async () => {
    await updateWafAccessLists({
        whitelist: listForm.whitelistText
            .split(/\r?\n|,|;/)
            .map((item) => item.trim())
            .filter(Boolean),
        blacklist: listForm.blacklistText
            .split(/\r?\n|,|;/)
            .map((item) => item.trim())
            .filter(Boolean),
    });
    ElMessage.success('黑白名单已保存');
};
const saveGlobal = async () => {
    global.requestBodyLimit = Math.round(bodyLimitMB.value * 1024 * 1024);
    await updateWafGlobal(global);
    ElMessage.success('WAF 全局策略已保存');
    await load();
};
const saveSite = async (site: WafSiteConfig) => {
    await updateWafSite({ websiteID: site.websiteID, enabled: site.enabled, mode: site.mode });
    ElMessage.success('网站 WAF 策略已保存');
};
const openRules = async (site: WafSiteConfig) => {
    selected.value = site;
    rules.value = (await listWafRules(site.websiteID)).data || [];
    rulesOpen.value = true;
};
const newRule = () => {
    if (!selected.value) return;
    Object.assign(editingRule, {
        id: undefined,
        websiteID: selected.value.websiteID,
        name: '',
        location: 'uri',
        operator: 'contains',
        value: '',
        action: 'block',
        priority: 100,
        enabled: true,
    });
    ruleEditorOpen.value = true;
};
const saveNewRule = async () => {
    if (!editingRule.name || !editingRule.value) {
        ElMessage.warning('请填写规则名称和值');
        return;
    }
    const result = await upsertWafRule({ ...editingRule });
    rules.value = rules.value.filter((item) => item.id !== result.data.id);
    rules.value.push(result.data);
    ruleEditorOpen.value = false;
    ElMessage.success('规则已保存');
};
const saveRule = async (rule: WafRule) => {
    await upsertWafRule({ ...rule, websiteID: selected.value!.websiteID });
};
const removeRule = async (rule: WafRule) => {
    await deleteWafRule({ websiteID: selected.value!.websiteID, id: rule.id! });
    rules.value = rules.value.filter((item) => item.id !== rule.id);
};
const openTest = (site: WafSiteConfig) => {
    testRequest.websiteID = site.websiteID;
    testResult.value = undefined;
    testOpen.value = true;
};
const runTest = async () => {
    testRequest.headers = { 'User-Agent': testUserAgent.value };
    testResult.value = (await testWafRules({ ...testRequest })).data;
};
onMounted(load);
</script>

<style scoped>
.runtime-info {
    color: var(--el-text-color-secondary);
    font-size: 13px;
}
.toolbar {
    margin-bottom: 12px;
}
</style>
