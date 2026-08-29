<template>
    <LayoutContent :title="$t('menu.multiNode')" v-loading="loading">
        <template #rightToolBar>
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
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { listNodes } from '@/utils/node';
import { useGlobalStore } from '@/composables/useGlobalStore';

const { globalStore, currentNode } = useGlobalStore();
const loading = ref(false);
const nodes = ref<any[]>([]);
const activeTab = ref('dashboard');
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
</script>
