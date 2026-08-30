<template>
    <LayoutContent :title="$t('menu.multiNode')" v-loading="loading">
        <template #rightToolBar>
            <el-button type="primary" :icon="Plus" @click="openAddDialog">添加节点</el-button>
            <TableRefresh @search="loadNodes" />
        </template>
        <template #main>
            <el-tabs v-model="activeTab">
                <el-tab-pane label="概况" name="dashboard">
                    <el-row :gutter="12" class="mb-4">
                        <el-col :span="6">
                            <el-card shadow="never"><el-statistic title="节点总数" :value="nodes.length" /></el-card>
                        </el-col>
                        <el-col :span="6">
                            <el-card shadow="never"><el-statistic title="在线节点" :value="onlineCount" /></el-card>
                        </el-col>
                        <el-col :span="6">
                            <el-card shadow="never">
                                <el-statistic title="离线节点" :value="nodes.length - onlineCount" />
                            </el-card>
                        </el-col>
                        <el-col :span="6">
                            <el-card shadow="never">
                                <el-statistic title="当前节点" :value="currentNode || '-'" />
                            </el-card>
                        </el-col>
                    </el-row>
                    <el-alert
                        type="info"
                        :closable="false"
                        title="节点状态来自控制面实时心跳，切换节点后网站、应用和任务操作会作用于选中节点。"
                    />
                </el-tab-pane>
                <el-tab-pane label="节点管理" name="nodes">
                    <el-table :data="nodes" stripe>
                        <el-table-column prop="name" :label="$t('xpack.node.node')" min-width="180">
                            <template #default="{ row }">
                                {{ row.name === 'local' ? globalStore.getMasterAlias() : row.name }}
                            </template>
                        </el-table-column>
                        <el-table-column prop="addr" :label="$t('xpack.node.addr')" min-width="180" />
                        <el-table-column prop="status" :label="$t('commons.table.status')" min-width="120" />
                        <el-table-column :label="$t('commons.table.operate')" width="140" fixed="right">
                            <template #default="{ row }">
                                <el-button
                                    link
                                    type="primary"
                                    :disabled="currentNode === row.name"
                                    @click="selectNode(row)"
                                >
                                    {{
                                        currentNode === row.name
                                            ? $t('xpack.node.currentNode')
                                            : $t('xpack.node.switchNode')
                                    }}
                                </el-button>
                            </template>
                        </el-table-column>
                    </el-table>
                </el-tab-pane>
                <el-tab-pane label="面板管理" name="panels">
                    <el-card shadow="never">
                        <template #header>面板连接</template>
                        <el-table :data="nodes" stripe>
                            <el-table-column prop="name" label="节点" min-width="180" />
                            <el-table-column prop="addr" label="面板地址" min-width="220" />
                            <el-table-column prop="version" label="版本" width="140">
                                <template #default="{ row }">{{ row.version || 'workmesh-node' }}</template>
                            </el-table-column>
                            <el-table-column label="健康状态" width="120">
                                <template #default="{ row }">
                                    <el-tag :type="isOnline(row) ? 'success' : 'danger'">
                                        {{ isOnline(row) ? '正常' : '离线' }}
                                    </el-tag>
                                </template>
                            </el-table-column>
                            <el-table-column label="操作" width="120">
                                <template #default="{ row }">
                                    <el-button link type="primary" @click="selectNode(row)">进入面板</el-button>
                                </template>
                            </el-table-column>
                        </el-table>
                    </el-card>
                </el-tab-pane>
            </el-tabs>
        </template>
    </LayoutContent>
    <el-dialog v-model="addVisible" title="添加部署节点" width="520px" destroy-on-close>
        <el-form ref="formRef" :model="form" :rules="rules" label-width="96px">
            <el-form-item label="节点 ID" prop="nodeId">
                <el-input v-model="form.nodeId" autocomplete="off" placeholder="例如 node-secondary" />
            </el-form-item>
            <el-form-item label="节点名称" prop="name">
                <el-input v-model="form.name" autocomplete="off" placeholder="可选" />
            </el-form-item>
            <el-form-item label="服务地址" prop="addr">
                <el-input v-model="form.addr" autocomplete="url" placeholder="http://host:9999" />
            </el-form-item>
            <el-form-item label="节点角色" prop="role">
                <el-select v-model="form.role" class="w-full">
                    <el-option label="主节点" value="primary" />
                    <el-option label="次节点" value="secondary" />
                </el-select>
            </el-form-item>
            <el-form-item label="描述" prop="description">
                <el-input v-model="form.description" type="textarea" :rows="2" maxlength="200" show-word-limit />
            </el-form-item>
        </el-form>
        <template #footer>
            <el-button @click="addVisible = false">取消</el-button>
            <el-button type="primary" :loading="saving" @click="submitAdd">保存</el-button>
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

const { globalStore, currentNode } = useGlobalStore();
const loading = ref(false);
const nodes = ref<any[]>([]);
const activeTab = ref('dashboard');
const addVisible = ref(false);
const saving = ref(false);
const formRef = ref();
const form = reactive({ nodeId: '', name: '', addr: '', role: 'secondary' as 'primary' | 'secondary', description: '' });
const rules = {
    nodeId: [{ required: true, message: '请输入节点 ID', trigger: 'blur' }],
    addr: [
        { required: true, message: '请输入节点服务地址', trigger: 'blur' },
        { pattern: /^https?:\/\/[^\s/]+(?::\d{1,5})?\/?$/, message: '请输入有效的 HTTP(S) 地址', trigger: 'blur' },
    ],
    role: [{ required: true, message: '请选择节点角色', trigger: 'change' }],
};
const isOnline = (node: any) =>
    ['online', 'Online', '正常', 'running', 'active'].includes(node.status) ||
    node.status === 1 ||
    node.online === true;
const onlineCount = computed(() => nodes.value.filter(isOnline).length);

const loadNodes = async () => {
    loading.value = true;
    try {
        nodes.value = await listNodes('all');
    } finally {
        loading.value = false;
    }
};

const selectNode = (node: { name: string; addr: string }) => {
    globalStore.currentNode = node.name;
    globalStore.currentNodeAddr = node.addr;
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
        ElMessage.success('节点已添加');
        addVisible.value = false;
        await loadNodes();
    } finally {
        saving.value = false;
    }
};
</script>
