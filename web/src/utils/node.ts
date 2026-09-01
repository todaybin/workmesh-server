import { Setting } from '@/api/interface/setting';
import { getCurrentNodeRole, listNodeOptions } from '@/api/modules/setting';
import { GlobalStore } from '@/store';
import type { NodeRole } from './node-display';

export const changeToLocal = async () => {
    const globalStore = GlobalStore();
    let nodes = await listNodes('all');
    if (nodes.length === 0) {
        setDefaultNodeInfo();
        return;
    }
    if (globalStore.isAdmin) {
        for (const item of nodes) {
            if (item.name === 'local') {
                globalStore.currentNode = 'local';
                globalStore.currentNodeAddr = item.addr;
                globalStore.currentNodeRole = normalizeNodeRole(item.role);
                return;
            }
        }
    }
    globalStore.currentNode = nodes[0].name;
    globalStore.currentNodeAddr = nodes[0].addr;
    globalStore.currentNodeRole = normalizeNodeRole(nodes[0].role);
};

export async function listNodes(type: string): Promise<Array<Setting.NodeItem>> {
    try {
        const res = await listNodeOptions(type);
        return res.data || [];
    } catch (error) {
        return [];
    }
}

export const setDefaultNodeInfo = () => {
    const globalStore = GlobalStore();
    globalStore.currentNode = 'local';
    globalStore.currentNodeAddr = '127.0.0.1';
    globalStore.currentNodeRole = '';
};

export const normalizeNodeRole = (value: unknown): NodeRole => {
    const role = String(value || '').trim().toLowerCase();
    return role === 'primary' || role === 'secondary' ? role : '';
};

export const loadCurrentNodeRole = async () => {
    const globalStore = GlobalStore();
    try {
        const res = await getCurrentNodeRole();
        const role = normalizeNodeRole(res.data?.role);
        if (role) {
            globalStore.currentNodeRole = role;
        }
        const nodeID = String(res.data?.nodeId || res.data?.node_id || '').trim();
        if (nodeID && globalStore.currentNode === 'local') {
            // 保留 local 作为旧接口兼容值，仅同步角色，不改变请求使用的节点标识。
            globalStore.currentNodeRole = role;
        }
        return role;
    } catch {
        return globalStore.currentNodeRole;
    }
};
