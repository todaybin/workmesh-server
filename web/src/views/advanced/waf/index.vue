<template>
    <LayoutContent :title="$t('menu.waf')" v-loading="loading">
        <template #rightToolBar><TableRefresh @search="load" /></template>
        <template #main>
            <el-tabs v-model="activeTab" class="waf-tabs">
                <el-tab-pane :label="$t('xpack.waf.todayStatus')" name="dashboard">
                    <el-row :gutter="12" class="mb-4">
                        <el-col v-for="card in auditCards" :key="card.label" :span="6">
                            <el-card shadow="never"><el-statistic :title="card.label" :value="card.value" /></el-card>
                        </el-col>
                    </el-row>
                    <el-card shadow="never">
                        <template #header>{{ $t('xpack.waf.attackLog') }}</template>
                        <el-table :data="auditRecords" stripe>
                            <el-table-column prop="time" :label="$t('xpack.waf.time')" width="180" />
                            <el-table-column prop="client_ip" :label="$t('xpack.waf.resource')" width="150" />
                            <el-table-column prop="host" :label="$t('menu.website')" width="180" />
                            <el-table-column prop="uri" :label="$t('xpack.waf.request')" min-width="240" />
                            <el-table-column prop="rule" :label="$t('xpack.waf.rule')" width="150" />
                            <el-table-column prop="action" :label="$t('xpack.waf.action')" width="90" />
                        </el-table>
                    </el-card>
                </el-tab-pane>
                <el-tab-pane :label="$t('xpack.waf.attackLog')" name="attack">
                    <el-card shadow="never">
                        <el-table :data="auditRecords" stripe>
                            <el-table-column prop="time" :label="$t('xpack.waf.time')" width="180" />
                            <el-table-column prop="client_ip" :label="$t('xpack.waf.resource')" width="150" />
                            <el-table-column prop="host" :label="$t('menu.website')" width="180" />
                            <el-table-column prop="rule" :label="$t('xpack.waf.rule')" width="150" />
                            <el-table-column prop="message" :label="$t('serverPages.websiteMonitor.detail')" min-width="260" />
                        </el-table>
                    </el-card>
                </el-tab-pane>
                <el-tab-pane :label="$t('xpack.waf.intercept')" name="logs">
                    <el-card shadow="never">
                        <el-table :data="blockedRecords" stripe>
                            <el-table-column prop="time" :label="$t('xpack.waf.time')" width="180" />
                            <el-table-column prop="client_ip" :label="$t('xpack.waf.resource')" width="150" />
                            <el-table-column prop="uri" label="URI" min-width="260" />
                            <el-table-column prop="status" :label="$t('commons.table.status')" width="90" />
                            <el-table-column prop="rule" :label="$t('xpack.waf.rule')" width="150" />
                        </el-table>
                    </el-card>
                </el-tab-pane>
                <el-tab-pane :label="$t('xpack.waf.blockRecords')" name="blocks">
                    <el-card shadow="never">
                        <el-table :data="blockedRecords" stripe>
                            <el-table-column prop="time" :label="$t('xpack.waf.time')" width="180" />
                            <el-table-column prop="client_ip" :label="$t('xpack.waf.resource')" width="160" />
                            <el-table-column prop="host" :label="$t('menu.website')" width="180" />
                            <el-table-column prop="reason" :label="$t('xpack.waf.blockTime')" min-width="240" />
                            <el-table-column :label="$t('commons.table.status')" width="100">
                                <template #default>{{ $t('xpack.waf.blockRecords') }}</template>
                            </el-table-column>
                        </el-table>
                    </el-card>
                </el-tab-pane>
                <el-tab-pane :label="$t('xpack.waf.blackWhite')" name="lists">
                    <el-card shadow="never">
                        <el-alert
                            type="info"
                            :closable="false"
                            :title="$t('serverPages.waf.whitelistHelper')"
                            class="mb-4"
                        />
                        <el-row :gutter="24">
                            <el-col :span="12">
                                    <el-form-item :label="$t('xpack.waf.whiteList')">
                                    <el-input
                                        v-model="listForm.whitelistText"
                                        type="textarea"
                                        :rows="12"
                                        placeholder="192.168.1.0/24"
                                    />
                                </el-form-item>
                            </el-col>
                            <el-col :span="12">
                                    <el-form-item :label="$t('xpack.waf.blackList')">
                                    <el-input
                                        v-model="listForm.blacklistText"
                                        type="textarea"
                                        :rows="12"
                                        placeholder="203.0.113.10"
                                    />
                                </el-form-item>
                            </el-col>
                        </el-row>
                        <el-button type="primary" @click="saveLists">{{ $t('commons.button.save') }}</el-button>
                    </el-card>
                </el-tab-pane>
                <el-tab-pane :label="$t('xpack.waf.websiteSetting')" name="sites" />
                <el-tab-pane :label="$t('xpack.waf.globalSetting')" name="global" />
            </el-tabs>
            <el-alert
                v-if="status && !status.available"
                type="error"
                :closable="false"
                :title="$t('xpack.waf.runtimeUnavailable')"
            />
            <el-card v-else-if="activeTab === 'global'" class="mb-4" shadow="never">
                <template #header><span>{{ $t('serverPages.waf.globalPolicy') }}</span></template>
                <el-form :inline="true" :model="global">
                    <el-form-item label="WAF"><el-switch v-model="global.enabled" /></el-form-item>
                    <el-form-item :label="$t('serverPages.waf.standardRules')"><el-switch v-model="global.standardRules" /></el-form-item>
                    <el-form-item :label="$t('serverPages.waf.mode')">
                        <el-radio-group v-model="global.mode">
                            <el-radio-button label="observe">{{ $t('serverPages.waf.observe') }}</el-radio-button>
                            <el-radio-button label="block">{{ $t('serverPages.waf.block') }}</el-radio-button>
                        </el-radio-group>
                    </el-form-item>
                    <el-form-item label="Paranoia Level">
                        <el-input-number v-model="global.paranoiaLevel" :min="1" :max="4" />
                    </el-form-item>
                    <el-form-item :label="$t('serverPages.waf.anomalyThreshold')">
                        <el-input-number v-model="global.inboundThreshold" :min="1" :max="99" />
                    </el-form-item>
                    <el-form-item :label="$t('serverPages.waf.requestBodyLimit')">
                        <el-input-number v-model="bodyLimitMB" :min="0" :max="10" />
                    </el-form-item>
                    <el-form-item><el-button type="primary" @click="saveGlobal">{{ $t('serverPages.waf.savePolicy') }}</el-button></el-form-item>
                </el-form>
                <div class="runtime-info">
                    {{ $t('serverPages.waf.runtimeInfo', { 0: status?.runtime, 1: status?.crs, 2: standardRules.length }) }}
                </div>
            </el-card>

            <el-card v-if="activeTab === 'sites'" shadow="never" class="mb-4">
                <template #header><span>{{ $t('serverPages.waf.websiteProtection') }}</span></template>
                <el-table :data="sites" stripe>
                    <el-table-column prop="alias" :label="$t('menu.website')" min-width="180" />
                    <el-table-column :label="$t('xpack.waf.ruleType')" width="140">
                        <template #default="{ row }">
                            <el-select v-model="row.mode" size="small" @change="saveSite(row)">
                                <el-option :label="$t('serverPages.waf.observe')" value="observe" />
                                <el-option :label="$t('serverPages.waf.block')" value="block" />
                            </el-select>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('serverPages.waf.enabled')" width="90">
                        <template #default="{ row }">
                            <el-switch v-model="row.enabled" @change="saveSite(row)" />
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('serverPages.waf.customRule')" width="110">
                        <template #default="{ row }">{{ row.rules?.length || 0 }}</template>
                    </el-table-column>
                    <el-table-column :label="$t('serverPages.waf.operation')" width="220">
                        <template #default="{ row }">
                            <el-button link type="primary" @click="openRules(row)">{{ $t('serverPages.waf.ruleManage') }}</el-button>
                            <el-button link type="primary" @click="openTest(row)">{{ $t('serverPages.waf.test') }}</el-button>
                        </template>
                    </el-table-column>
                </el-table>
            </el-card>

            <el-card v-if="activeTab === 'sites'" shadow="never">
                <template #header><span>{{ $t('serverPages.waf.standardCoverage') }}</span></template>
                <el-table :data="standardRules" stripe size="small">
                    <el-table-column prop="id" :label="$t('serverPages.waf.ruleID')" width="140" />
                    <el-table-column prop="category" :label="$t('serverPages.waf.category')" width="140" />
                    <el-table-column prop="description" :label="$t('serverPages.waf.detection')" />
                    <el-table-column prop="locations" :label="$t('serverPages.waf.location')" width="220">
                        <template #default="{ row }">{{ row.locations.join('、') }}</template>
                    </el-table-column>
                </el-table>
            </el-card>

            <el-dialog v-model="rulesOpen" :title="$t('serverPages.waf.ruleManage') + ': ' + (selected?.alias || '')" width="900px">
                <div class="toolbar"><el-button type="primary" @click="newRule">{{ $t('serverPages.waf.addCustomRule') }}</el-button></div>
                <el-table :data="rules" stripe>
                    <el-table-column prop="name" :label="$t('xpack.waf.name')" min-width="160" />
                    <el-table-column prop="location" :label="$t('xpack.waf.ipLocation')" width="90" />
                    <el-table-column prop="operator" :label="$t('xpack.waf.ruleType')" width="90" />
                    <el-table-column prop="action" :label="$t('xpack.waf.action')" width="90" />
                    <el-table-column prop="priority" :label="$t('xpack.waf.frequencyLimit')" width="80" />
                    <el-table-column :label="$t('serverPages.waf.enabled')" width="80">
                        <template #default="{ row }">
                            <el-switch v-model="row.enabled" @change="saveRule(row)" />
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('serverPages.waf.operation')" width="90">
                        <template #default="{ row }">
                            <el-button link type="danger" @click="removeRule(row)">{{ $t('serverPages.waf.delete') }}</el-button>
                        </template>
                    </el-table-column>
                </el-table>
            </el-dialog>

            <el-dialog v-model="ruleEditorOpen" :title="$t('serverPages.waf.customRule')" width="560px">
                <el-form :model="editingRule" label-width="90px">
                    <el-form-item :label="$t('xpack.waf.name')"><el-input v-model="editingRule.name" maxlength="120" /></el-form-item>
                    <el-form-item :label="$t('xpack.waf.ipLocation')">
                        <el-select v-model="editingRule.location">
                            <el-option v-for="item in locations" :key="item" :label="item" :value="item" />
                        </el-select>
                    </el-form-item>
                    <el-form-item
                        v-if="editingRule.location === 'header' || editingRule.location === 'cookie'"
                        :label="$t('xpack.waf.rule')"
                    >
                        <el-input v-model="editingRule.key" placeholder="User-Agent" />
                    </el-form-item>
                    <el-form-item :label="$t('xpack.waf.ruleType')">
                        <el-select v-model="editingRule.operator">
                            <el-option :label="$t('serverPages.waf.observe')" value="contains" />
                            <el-option label="Regex" value="regex" />
                            <el-option label="Equals" value="equals" />
                            <el-option label="IP CIDR" value="ip-cidr" />
                        </el-select>
                    </el-form-item>
                    <el-form-item :label="$t('xpack.waf.resource')">
                        <el-input v-model="editingRule.value" type="textarea" maxlength="2048" />
                    </el-form-item>
                    <el-form-item :label="$t('xpack.waf.action')">
                        <el-radio-group v-model="editingRule.action">
                            <el-radio-button label="log">{{ $t('serverPages.waf.record') }}</el-radio-button>
                            <el-radio-button label="block">{{ $t('serverPages.waf.block') }}</el-radio-button>
                            <el-radio-button label="allow">{{ $t('serverPages.waf.allow') }}</el-radio-button>
                        </el-radio-group>
                    </el-form-item>
                    <el-form-item :label="$t('xpack.waf.frequencyLimit')">
                        <el-input-number v-model="editingRule.priority" :min="1" :max="10000" />
                    </el-form-item>
                </el-form>
                <template #footer>
                    <el-button @click="ruleEditorOpen = false">{{ $t('serverPages.waf.close') }}</el-button>
                    <el-button type="primary" @click="saveNewRule">{{ $t('serverPages.waf.save') }}</el-button>
                </template>
            </el-dialog>

            <el-dialog v-model="testOpen" :title="$t('serverPages.waf.test')" width="720px">
                <el-form :model="testRequest" label-width="90px">
                    <el-form-item :label="$t('serverPages.waf.method')">
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
                    <el-form-item :label="$t('serverPages.waf.parameter')">
                        <el-input v-model="testRequest.args" placeholder="id=1%20union%20select%201" />
                    </el-form-item>
                    <el-form-item label="User-Agent"><el-input v-model="testUserAgent" /></el-form-item>
                    <el-form-item :label="$t('serverPages.waf.requestBodyLimit')">
                        <el-input v-model="testRequest.body" type="textarea" :rows="3" />
                    </el-form-item>
                </el-form>
                <el-alert
                    v-if="testResult"
                    :type="testResult.blocked ? 'error' : 'success'"
                    :title="testResult.blocked ? $t('serverPages.waf.expectedBlocked', { 0: testResult.status }) : $t('serverPages.waf.notBlocked')"
                    :closable="false"
                />
                <el-table v-if="testResult" :data="testResult.matches" class="mt-3" size="small">
                    <el-table-column prop="id" :label="$t('xpack.waf.rule')" width="150" />
                    <el-table-column prop="source" :label="$t('serverPages.waf.source')" width="100" />
                    <el-table-column prop="name" :label="$t('serverPages.websiteMonitor.detail')" />
                </el-table>
                <template #footer>
                    <el-button @click="testOpen = false">{{ $t('serverPages.waf.close') }}</el-button>
                    <el-button type="primary" @click="runTest">{{ $t('serverPages.waf.executeTest') }}</el-button>
                </template>
            </el-dialog>
        </template>
    </LayoutContent>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { ElMessage } from 'element-plus';
import i18n from '@/lang';
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
        { label: i18n.global.t('serverPages.waf.attackTotal'), value: total },
        { label: i18n.global.t('serverPages.waf.blockedCount'), value: blocked },
        { label: i18n.global.t('serverPages.waf.attackIPs'), value: ips },
        { label: i18n.global.t('serverPages.waf.ruleHits'), value: total },
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
    ElMessage.success(i18n.global.t('serverPages.waf.listsSaved'));
};
const saveGlobal = async () => {
    global.requestBodyLimit = Math.round(bodyLimitMB.value * 1024 * 1024);
    await updateWafGlobal(global);
    ElMessage.success(i18n.global.t('serverPages.waf.globalSaved'));
    await load();
};
const saveSite = async (site: WafSiteConfig) => {
    await updateWafSite({ websiteID: site.websiteID, enabled: site.enabled, mode: site.mode });
    ElMessage.success(i18n.global.t('serverPages.waf.siteSaved'));
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
        ElMessage.warning(i18n.global.t('serverPages.waf.requiredRule'));
        return;
    }
    const result = await upsertWafRule({ ...editingRule });
    rules.value = rules.value.filter((item) => item.id !== result.data.id);
    rules.value.push(result.data);
    ruleEditorOpen.value = false;
    ElMessage.success(i18n.global.t('serverPages.waf.ruleSaved'));
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
