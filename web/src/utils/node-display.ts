import i18n from '@/lang';
import type { Setting } from '@/api/interface/setting';

export type NodeRole = 'primary' | 'secondary' | '';

const normalizeRole = (value: unknown): NodeRole => {
    const role = String(value || '').trim().toLowerCase();
    return role === 'primary' || role === 'secondary' ? role : '';
};

/** 根据服务端角色显示节点名称，避免把机器节点 ID 暴露到界面。 */
export const getNodeDisplayName = (
    item: Partial<Setting.NodeItem> | undefined,
    masterAlias = '',
    roleOverride?: string,
) => {
    const role = normalizeRole(roleOverride || item?.role);
    if (!item) {
        if (role === 'secondary') {
            return i18n.global.t('xpack.cluster.slave');
        }
        return masterAlias || i18n.global.t('xpack.node.master');
    }
    const name = String(item.name || '').trim();
    const nodeID = String(item.nodeId || '').trim();
    if (name === 'local' && masterAlias.trim()) {
        return masterAlias.trim();
    }
    // 控制面默认以 nodeId 作为名称；此时使用角色翻译，保留自定义名称不变。
    const isGeneratedName = !name || name === nodeID || (name === 'local' && !masterAlias.trim());
    if (isGeneratedName && role === 'primary') {
        return i18n.global.t('xpack.node.master');
    }
    if (isGeneratedName && role === 'secondary') {
        return i18n.global.t('xpack.cluster.slave');
    }
    return name || nodeID || i18n.global.t('xpack.node.node');
};

export const getNodeRoleLabel = (role: unknown) => {
    return normalizeRole(role) === 'primary'
        ? i18n.global.t('xpack.node.master')
        : i18n.global.t('xpack.cluster.slave');
};
