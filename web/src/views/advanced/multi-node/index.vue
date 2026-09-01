<template>
    <LayoutContent :title="$t('menu.multiNode')" v-loading="loading">
        <template #rightToolBar>
            <el-button type="primary" :icon="Plus" @click="openAddDialog">{{ $t('serverPages.multiNode.addNode') }}</el-button>
            <TableRefresh @search="loadNodes" />
        </template>
        <template #main>
            <el-tabs v-model="activeTab">
                <el-tab-pane :label="$t('serverPages.multiNode.overview')" name="dashboard">
                    <el-row :gutter="12" class="mb-4">
                        <el-col :span="6">
                            <el-card shadow="never"><el-statistic :title="$t('serverPages.multiNode.nodeTotal')" :value="nodes.length" /></el-card>
                        </el-col>
                        <el-col :span="6">
                            <el-card shadow="never"><el-statistic :title="$t('serverPages.multiNode.onlineNodes')" :value="onlineCount" /></el-card>
                        </el-col>
                        <el-col :span="6">
                            <el-card shadow="never">
                                <el-statistic :title="$t('serverPages.multiNode.offlineNodes')" :value="nodes.length - onlineCount" />
                            </el-card>
                        </el-col>
                        <el-col :span="6">
                            <el-card shadow="never">
                                <el-statistic :title="$t('serverPages.multiNode.currentNode')" :value="currentNodeLabel" />
                            </el-card>
                        </el-col>
                    </el-row>
                    <el-alert
                        type="info"
                        :closable="false"
                        :title="$t('serverPages.multiNode.statusHelper')"
                    />
                </el-tab-pane>
                <el-tab-pane :label="$t('serverPages.multiNode.nodeManagement')" name="nodes">
                    <el-table :data="nodes" stripe>
                        <el-table-column prop="name" :label="$t('xpack.node.node')" min-width="180">
                            <template #default="{ row }">
                                {{ displayNodeName(row) }}
                            </template>
                        </el-table-column>
                        <el-table-column prop="addr" :label="$t('xpack.node.addr')" min-width="180" />
                        <el-table-column prop="status" :label="$t('commons.table.status')" min-width="120" />
                        <el-table-column :label="$t('commons.table.operate')" width="140" fixed="right">
                            <template #default="{ row }">
                            <el-button
                                    link
                                    type="primary"
                                    :disabled="isCurrentNode(row)"
                                    @click="selectNode(row)"
                                >
                                    {{
                                        isCurrentNode(row)
                                            ? $t('xpack.node.currentNode')
                                            : $t('xpack.node.switchNode')
                                    }}
                                </el-button>
                            </template>
                        </el-table-column>
                    </el-table>
                </el-tab-pane>
                <el-tab-pane :label="$t('serverPages.multiNode.panelManagement')" name="panels">
                    <el-card shadow="never">
                        <template #header>{{ $t('serverPages.multiNode.panelConnection') }}</template>
                        <el-table :data="nodes" stripe>
                            <el-table-column prop="name" :label="$t('xpack.node.node')" min-width="180">
                                <template #default="{ row }">{{ displayNodeName(row) }}</template>
                            </el-table-column>
                            <el-table-column prop="addr" :label="$t('serverPages.multiNode.panelAddress')" min-width="220" />
                            <el-table-column prop="version" :label="$t('serverPages.multiNode.version')" width="140">
                                <template #default="{ row }">{{ row.version || 'workmesh-node' }}</template>
                            </el-table-column>
                            <el-table-column :label="$t('serverPages.multiNode.healthStatus')" width="120">
                                <template #default="{ row }">
                                    <el-tag :type="isOnline(row) ? 'success' : 'danger'">
                                        {{ isOnline(row) ? $t('serverPages.multiNode.normal') : $t('serverPages.multiNode.offline') }}
                                    </el-tag>
                                </template>
                            </el-table-column>
                            <el-table-column :label="$t('serverPages.multiNode.operation')" width="120">
                                <template #default="{ row }">
                                    <el-button link type="primary" @click="selectNode(row)">{{ $t('serverPages.multiNode.enterPanel') }}</el-button>
                                </template>
                            </el-table-column>
                        </el-table>
                    </el-card>
                </el-tab-pane>
            </el-tabs>
        </template>
    </LayoutContent>
    <el-dialog v-model="addVisible" :title="$t('serverPages.multiNode.addDeploymentNode')" width="520px" destroy-on-close>
        <el-form ref="formRef" :model="form" :rules="rules" label-width="96px">
            <el-form-item :label="$t('serverPages.multiNode.nodeID')" prop="nodeId">
                <el-input v-model="form.nodeId" autocomplete="off" :placeholder="$t('serverPages.multiNode.nodeIDPlaceholder')" />
            </el-form-item>
            <el-form-item :label="$t('serverPages.multiNode.nodeName')" prop="name">
                <el-input v-model="form.name" autocomplete="off" :placeholder="$t('serverPages.multiNode.optional')" />
            </el-form-item>
            <el-form-item :label="$t('serverPages.multiNode.serviceAddress')" prop="addr">
                <el-input v-model="form.addr" autocomplete="url" placeholder="http://host:9999" />
            </el-form-item>
            <el-form-item :label="$t('serverPages.multiNode.nodeRole')" prop="role">
                <el-select v-model="form.role" class="w-full">
                    <el-option :label="$t('serverPages.multiNode.primary')" value="primary" />
                    <el-option :label="$t('serverPages.multiNode.secondary')" value="secondary" />
                </el-select>
            </el-form-item>
            <el-form-item :label="$t('serverPages.multiNode.description')" prop="description">
                <el-input v-model="form.description" type="textarea" :rows="2" maxlength="200" show-word-limit />
            </el-form-item>
        </el-form>
        <template #footer>
            <el-button @click="addVisible = false">{{ $t('serverPages.multiNode.cancel') }}</el-button>
            <el-button type="primary" :loading="saving" @click="submitAdd">{{ $t('serverPages.multiNode.save') }}</el-button>
        </template>
    </el-dialog>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { ElMessage } from 'element-plus';
import { Plus } from '@element-plus/icons-vue';
import { addNode } from '@/api/modules/setting';
import { listNodes } from '@/utils/node';
import { useGlobalStore } from '@/composables/useGlobalStore';
import i18n from '@/lang';
import { getNodeDisplayName } from '@/utils/node-display';
import { normalizeNodeRole } from '@/utils/node';

const { globalStore, currentNode, currentNodeRole } = useGlobalStore();
const loading = ref(false);
const nodes = ref<any[]>([]);
const activeTab = ref('dashboard');
const addVisible = ref(false);
const saving = ref(false);
const formRef = ref();
const form = reactive({ nodeId: '', name: '', addr: '', role: 'secondary' as 'primary' | 'secondary', description: '' });
const rules = {
    nodeId: [{ required: true, message: i18n.global.t('serverPages.multiNode.nodeIDRequired'), trigger: 'blur' }],
    addr: [
        { required: true, message: i18n.global.t('serverPages.multiNode.serviceAddressRequired'), trigger: 'blur' },
        { pattern: /^https?:\/\/[^\s/]+(?::\d{1,5})?\/?$/, message: i18n.global.t('serverPages.multiNode.serviceAddressInvalid'), trigger: 'blur' },
    ],
    role: [{ required: true, message: i18n.global.t('serverPages.multiNode.nodeRoleRequired'), trigger: 'change' }],
};
const isOnline = (node: any) =>
    ['online', 'Online', '正常', 'running', 'active'].includes(node.status) ||
    node.status === 1 ||
    node.online === true;
const onlineCount = computed(() => nodes.value.filter(isOnline).length);
const displayNodeName = (node: any) => getNodeDisplayName(node, globalStore.getMasterAlias());
const isCurrentNode = (node: any) => currentNode.value === node.name || currentNode.value === node.nodeId || node.isCurrent === true;
const currentNodeLabel = computed(() => {
    const node = nodes.value.find((item) => isCurrentNode(item));
    return displayNodeName(node) || getNodeDisplayName(undefined, globalStore.getMasterAlias(), currentNodeRole.value);
});

const loadNodes = async () => {
    loading.value = true;
    try {
        nodes.value = await listNodes('all');
    } finally {
        loading.value = false;
    }
};

const selectNode = (node: { name: string; nodeId?: string; addr: string; role?: string }) => {
    globalStore.currentNode = node.name;
    globalStore.currentNodeAddr = node.addr;
    globalStore.currentNodeRole = normalizeNodeRole(node.role);
};

onMounted(loadNodes);

const openAddDialog = () => {
    form.nodeId = '';
    form.name = '';
    form.addr = '';
    form.role = 'secondary';
    form.description = '';
    addVisible.value = true;
};

const submitAdd = async () => {
    const valid = await formRef.value?.validate().catch(() => false);
    if (!valid) return;
    saving.value = true;
    try {
        await addNode({ ...form });
        ElMessage.success(i18n.global.t('serverPages.multiNode.nodeAdded'));
        addVisible.value = false;
        await loadNodes();
    } finally {
        saving.value = false;
    }
};
</script>
