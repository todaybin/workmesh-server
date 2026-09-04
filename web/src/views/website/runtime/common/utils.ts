import { ElMessageBox } from 'element-plus';
import i18n from '@/lang';
import { OperateRuntime, updateRemark } from '@/api/modules/runtime';
import { Ref } from 'vue';
import { MsgError, MsgSuccess } from '@/utils/message';
import { Runtime } from '@/api/interface/runtime';

export const operateRuntime = async (operate: string, ID: string | number, loading: Ref<boolean>, search: () => void) => {
    try {
        const action = await ElMessageBox.confirm(
            i18n.global.t('runtime.operatorHelper', [i18n.global.t('commons.operate.' + operate)]),
            i18n.global.t('commons.operate.' + operate),
            {
                confirmButtonText: i18n.global.t('commons.button.confirm'),
                cancelButtonText: i18n.global.t('commons.button.cancel'),
                type: 'info',
            },
        );

        if (action === 'confirm') {
            loading.value = true;
            // Runtime IDs are string identifiers (for example `php74`).
            // Keep the legacy uppercase field for older nodes, but never send
            // a numeric value that can be coerced or lost by transport code.
            await OperateRuntime({ operate: operate, ID: String(ID) });
            search();
        }
    } catch (error) {
    } finally {
        loading.value = false;
    }
};

export const updateRuntimeRemark = async (row: Runtime.Runtime, bulr: Function) => {
    bulr();
    if (row.remark && row.remark.length > 128) {
        MsgError(i18n.global.t('commons.rule.length128Err'));
        return;
    }
    try {
        await updateRemark({
            id: row.id,
            remark: row.remark,
        }).then(() => {
            MsgSuccess(i18n.global.t('commons.msg.updateSuccess'));
        });
    } catch (error) {}
};

// Runtime 列表统一使用后端返回的 1Panel 规范 Compose 路径，缺失时退回容器日志模式。
export const runtimeComposePath = (row: any): string => {
    const composePath = typeof row?.composePath === 'string' ? row.composePath.trim() : '';
    if (composePath && !/undefined|null/i.test(composePath)) {
        return composePath;
    }
    const installPath = typeof row?.path === 'string' ? row.path.trim() : '';
    if (!installPath || /undefined|null/i.test(installPath)) {
        return '';
    }
    return installPath.replace(/[\\/]+$/, '') + '/docker-compose.yml';
};
