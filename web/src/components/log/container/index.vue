<template>
    <div v-if="showControl" class="log-toolbar">
        <el-select @change="searchLogs" class="fetchClass" v-model="logSearch.mode">
            <template #prefix>{{ $t('container.fetch') }}</template>
            <el-option v-for="item in timeOptions" :key="item.label" :value="item.value" :label="item.label" />
        </el-select>
        <el-select @change="searchLogs" class="tailClass" v-model.number="logSearch.tail">
            <template #prefix>{{ $t('container.lines') }}</template>
            <el-option :value="0" :label="$t('commons.table.all')" />
            <el-option :value="100" :label="100" />
            <el-option :value="200" :label="200" />
            <el-option :value="500" :label="500" />
            <el-option :value="1000" :label="1000" />
        </el-select>
        <div class="margin-button float-left">
            <el-checkbox border @change="searchLogs" v-model="logSearch.isWatch">
                {{ $t('commons.button.watch') }}
            </el-checkbox>
        </div>
        <div class="margin-button float-left">
            <el-checkbox border @change="searchLogs" v-model="logSearch.isShowTimestamp">
                {{ $t('commons.table.date') }}
            </el-checkbox>
        </div>
        <el-button class="margin-button" @click="openDownloadDialog" icon="Download">
            {{ $t('commons.button.download') }}
        </el-button>
        <el-button v-permission="'container_manage'" class="margin-button" @click="onClean" icon="Delete">
            {{ $t('commons.button.clean') }}
        </el-button>
    </div>
    <div class="log-container" :style="styleVars">
        <div class="xterm-log-viewer" ref="terminalElement"></div>
    </div>
    <DialogPro
        v-model="downloadDialogVisible"
        :title="$t('commons.button.download')"
        size="small"
        :close-on-click-modal="true"
    >
        <el-form label-position="top">
            <el-form-item :label="$t('container.fetch')">
                <el-select v-model="downloadForm.mode" class="w-full">
                    <el-option v-for="item in timeOptions" :key="item.label" :value="item.value" :label="item.label" />
                </el-select>
            </el-form-item>
            <el-form-item :label="$t('container.lines')">
                <el-select
                    v-model="downloadForm.tail"
                    class="w-full"
                    filterable
                    allow-create
                    default-first-option
                    :reserve-keyword="false"
                >
                    <el-option :value="0" :label="$t('commons.table.all')" />
                    <el-option :value="100" :label="100" />
                    <el-option :value="200" :label="200" />
                    <el-option :value="500" :label="500" />
                    <el-option :value="1000" :label="1000" />
                </el-select>
                <div class="download-tail-helper">{{ $t('container.downloadLinesHelper') }}</div>
            </el-form-item>
        </el-form>
        <template #footer>
            <el-button @click="downloadDialogVisible = false">{{ $t('commons.button.cancel') }}</el-button>
            <el-button type="primary" @click="onDownload">{{ $t('commons.button.confirm') }}</el-button>
        </template>
    </DialogPro>
</template>

<script lang="ts" setup>
import { cleanComposeLog, cleanContainerLog, DownloadFile } from '@/api/modules/container';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import i18n from '@/lang';
import { dateFormatForName } from '@/utils/date';
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { ElMessageBox } from 'element-plus';
import { MsgError, MsgSuccess } from '@/utils/message';
import { useGlobalStore } from '@/composables/useGlobalStore';
import { buildSameOriginApiUrl } from '@/api/transport';
import { buildSseRequestHeaders, SseParser } from '@/utils/sse';
import { handleAuthResponseCode, handleAuthResponseStatus } from '@/utils/auth-response';
const { currentNode: globalCurrentNode } = useGlobalStore();

const em = defineEmits(['update:loading']);

const props = defineProps({
    container: {
        type: String,
        default: '',
    },
    compose: {
        type: String,
        default: '',
    },
    resource: {
        type: String,
        default: '',
    },
    highlightDiff: {
        type: Number,
        default: 320,
    },
    node: {
        type: String,
        default: '',
    },
    showControl: {
        type: Boolean,
        default: true,
    },
    defaultFollow: {
        type: Boolean,
        default: true,
    },
    defaultIsShowTimestamp: {
        type: Boolean,
        default: false,
    },
});

const styleVars = computed(() => ({
    '--custom-height': `${props.highlightDiff || 320}px`,
}));

const terminalElement = ref<HTMLDivElement | null>(null);
let streamAbortController: AbortController | null = null;
let reconnectTimer: number | null = null;
let reconnectResolver: (() => void) | null = null;
let streamGeneration = 0;
let lastEventId = 0;
let term: Terminal | null = null;
const fitAddon = new FitAddon();
let onScrollDisposable: { dispose: () => void } | null = null;
const MAX_VIEW_LINES = 20000;
const followBottom = ref(true);

const logSearch = reactive({
    isWatch: props.defaultFollow,
    isShowTimestamp: props.defaultIsShowTimestamp,
    container: '',
    mode: 'all',
    tail: 100,
    compose: '',
    resource: '',
});
const downloadDialogVisible = ref(false);
const downloadForm = reactive<{ mode: string; tail: number | string }>({
    mode: 'all',
    tail: 0,
});

const timeOptions = ref([
    { label: i18n.global.t('commons.table.all'), value: 'all' },
    {
        label: i18n.global.t('container.lastDay'),
        value: '24h',
    },
    {
        label: i18n.global.t('container.last4Hour'),
        value: '4h',
    },
    {
        label: i18n.global.t('container.lastHour'),
        value: '1h',
    },
    {
        label: i18n.global.t('container.last10Min'),
        value: '10m',
    },
]);

const stopListening = () => {
    streamGeneration++;
    if (streamAbortController) {
        streamAbortController.abort();
        streamAbortController = null;
    }
    if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer);
        reconnectTimer = null;
    }
    reconnectResolver?.();
    reconnectResolver = null;
};

const clearTerminal = () => {
    term?.reset();
    followBottom.value = true;
};

const writeLogLine = (data: string) => {
    if (!term) return;
    term.writeln(data);
    if (followBottom.value) {
        term.scrollToBottom();
    }
};

const bindXTermEvents = () => {
    if (!term) return;
    onScrollDisposable?.dispose();
    onScrollDisposable = term.onScroll(() => {
        if (!term) return;
        const active = term.buffer.active;
        followBottom.value = active.baseY + active.cursorY >= active.length - 2;
    });
};

const getPayloadMessage = (payload: unknown): string => {
    if (typeof payload === 'string') {
        try {
            return getPayloadMessage(JSON.parse(payload));
        } catch {
            return payload.trim();
        }
    }
    if (payload && typeof payload === 'object') {
        const value = payload as Record<string, unknown>;
        if (typeof value.message === 'string' && value.message.trim()) return value.message.trim();
        if (typeof value.error === 'string' && value.error.trim()) return value.error.trim();
        if (value.details && typeof value.details === 'object') {
            const details = value.details as Record<string, unknown>;
            if (typeof details.message === 'string' && details.message.trim()) return details.message.trim();
            if (typeof details.errCode === 'string' && details.errCode.trim()) return details.errCode.trim();
        }
    }
    return '';
};

const showStreamError = (message: string) => {
    const text = message || i18n.global.t('commons.msg.requestTimeout');
    MsgError(text);
    writeLogLine(`[error] ${text}`);
};

const initTerminal = () => {
    if (!terminalElement.value || term) return;
    term = new Terminal({
        cursorBlink: false,
        cursorStyle: 'block',
        disableStdin: true,
        convertEol: true,
        scrollback: MAX_VIEW_LINES,
        fontSize: 14,
        fontFamily: "'JetBrains Mono', Monaco, Menlo, Consolas, 'Courier New', monospace",
        fontWeight: '500',
        lineHeight: 1.2,
        theme: {
            background: '#111827',
            foreground: '#e5e7eb',
            cursor: '#e5e7eb',
            black: '#111827',
            brightBlack: '#6b7280',
            red: '#f87171',
            green: '#34d399',
            yellow: '#fbbf24',
            blue: '#60a5fa',
            magenta: '#c084fc',
            cyan: '#22d3ee',
            white: '#e5e7eb',
            brightWhite: '#f9fafb',
            selectionBackground: 'rgba(102, 178, 255, 0.30)',
            selectionInactiveBackground: 'rgba(102, 178, 255, 0.20)',
        },
    });
    term.open(terminalElement.value);
    term.loadAddon(fitAddon);
    fitAddon.fit();
    bindXTermEvents();
};

const handleClose = async () => {
    stopListening();
};

interface StreamReadResult {
    completed: boolean;
    serverError: boolean;
}

const handleSseEvent = (eventName: string, data: string, eventId?: string): StreamReadResult => {
    if (eventId && /^\d+$/.test(eventId)) {
        lastEventId = Number(eventId);
    }
    if (eventName === 'error') {
        showStreamError(getPayloadMessage(data) || data);
        return { completed: true, serverError: true };
    }
    if (eventName === 'close') {
        return { completed: true, serverError: false };
    }
    if (eventName === 'message' && data !== '') {
        writeLogLine(data);
    }
    return { completed: false, serverError: false };
};

const readSseStream = async (response: Response, generation: number): Promise<StreamReadResult> => {
    if (!response.body) {
        return { completed: true, serverError: false };
    }
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    const parser = new SseParser();
    let result: StreamReadResult = { completed: false, serverError: false };

    const dispatch = (events: Array<{ event: string; data: string; id?: string }>) => {
        for (const event of events) {
            const eventResult = handleSseEvent(event.event, event.data, event.id);
            result = {
                completed: result.completed || eventResult.completed,
                serverError: result.serverError || eventResult.serverError,
            };
            if (result.completed) return;
        }
    };

    try {
        while (generation === streamGeneration) {
            const { done, value } = await reader.read();
            if (done) break;
            // 切换容器或关闭窗口可能在 read() 等待期间发生；旧连接的数据不能写入新窗口。
            if (generation !== streamGeneration) return result;
            dispatch(parser.push(decoder.decode(value, { stream: true })));
            if (result.completed) return result;
        }
        if (generation !== streamGeneration) {
            return result;
        }
        dispatch(parser.push(decoder.decode()));
        if (!result.completed) dispatch(parser.finish());
        return result;
    } finally {
        await reader.cancel().catch(() => undefined);
        reader.releaseLock();
    }
};

const readNonStreamResponse = async (response: Response) => {
    const text = await response.text();
    if (!text.trim()) return;
    try {
        const payload = JSON.parse(text) as Record<string, unknown>;
        const authResult = handleAuthResponseCode(payload);
        if (authResult.handled) {
            showStreamError(authResult.message);
            return;
        }
        if (payload.code !== undefined && Number(payload.code) !== 200) {
            showStreamError(getPayloadMessage(payload) || text);
            return;
        }
        if (typeof payload.data === 'string') {
            payload.data.split(/\r?\n/).forEach((line) => writeLogLine(line));
            return;
        }
        if (payload.code !== undefined) return;
    } catch {
        text.split(/\r?\n/).forEach((line) => writeLogLine(line));
    }
};

const connectLogStream = async (url: string, currentNode: string, generation: number) => {
    let firstConnection = true;
    while (generation === streamGeneration && (firstConnection || logSearch.isWatch)) {
        firstConnection = false;
        const controller = new AbortController();
        streamAbortController = controller;
        let shouldReconnect = false;
        try {
            const response = await fetch(url, {
                credentials: 'include',
                headers: buildSseRequestHeaders(currentNode, lastEventId),
                signal: controller.signal,
            });
            if (!response.ok) {
                const authResult = handleAuthResponseStatus(response.status);
                if (authResult.handled) {
                    showStreamError(authResult.message);
                    return;
                }
                await readNonStreamResponse(response);
                if (response.status >= 500) shouldReconnect = true;
                else return;
            } else if (!(response.headers.get('content-type') || '').toLowerCase().includes('text/event-stream')) {
                await readNonStreamResponse(response);
                return;
            } else {
                const result = await readSseStream(response, generation);
                if (result.serverError || result.completed || !logSearch.isWatch) return;
                shouldReconnect = true;
            }
        } catch (error) {
            if (controller.signal.aborted || generation !== streamGeneration) return;
            shouldReconnect = true;
            writeLogLine('[log stream disconnected, retrying in 1 second]');
        } finally {
            if (streamAbortController === controller) streamAbortController = null;
        }
        if (!shouldReconnect || generation !== streamGeneration || !logSearch.isWatch) return;
        await new Promise<void>((resolve) => {
            reconnectResolver = resolve;
            reconnectTimer = window.setTimeout(() => {
                reconnectTimer = null;
                reconnectResolver = null;
                resolve();
            }, 1000);
        });
    }
};

const searchLogs = () => {
    if (Number(logSearch.tail) < 0) {
        MsgError(i18n.global.t('container.linesHelper'));
        return;
    }
    stopListening();
    clearTerminal();

    let currentNode = globalCurrentNode.value;
    if (props.node && props.node !== '') {
        currentNode = props.node;
    }

    const params = new URLSearchParams({
        container: logSearch.container,
        since: logSearch.mode,
        tail: String(logSearch.tail),
        follow: String(logSearch.isWatch),
        timestamp: String(logSearch.isShowTimestamp),
        operateNode: currentNode || '',
    });
    if (logSearch.compose !== '') {
        params.delete('container');
        params.set('compose', logSearch.compose);
    }
    const url = buildSameOriginApiUrl('/containers/search/log', params);
    lastEventId = 0;
    const generation = streamGeneration;
    void connectLogStream(url, currentNode, generation);
};

const syncPropsAndSearch = () => {
    if (!term) return;
    logSearch.container = props.container;
    logSearch.compose = props.compose;
    logSearch.resource = props.resource;
    logSearch.tail = 100;
    logSearch.mode = 'all';
    logSearch.isWatch = props.defaultFollow;
    logSearch.isShowTimestamp = props.defaultIsShowTimestamp;
    searchLogs();
};

const openDownloadDialog = () => {
    downloadForm.mode = logSearch.mode;
    downloadForm.tail = logSearch.tail;
    downloadDialogVisible.value = true;
};

const onDownload = async () => {
    const customTail = Number(downloadForm.tail);
    if (Number.isNaN(customTail) || customTail < 0) {
        MsgError(i18n.global.t('container.linesHelper'));
        return;
    }
    const container = logSearch.compose === '' ? logSearch.container : logSearch.compose;
    let resource = container;
    if (props.resource) {
        resource = props.resource;
    }
    const containerType = logSearch.compose === '' ? 'container' : 'compose';
    const params = {
        container: container,
        since: downloadForm.mode,
        tail: customTail,
        timestamp: logSearch.isShowTimestamp,
        containerType: containerType,
    };
    const addItem = {};
    addItem['name'] = resource + '-' + dateFormatForName(new Date()) + '.log';
    DownloadFile(params).then((res) => {
        const downloadUrl = window.URL.createObjectURL(new Blob([res]));
        const a = document.createElement('a');
        a.style.display = 'none';
        a.href = downloadUrl;
        a.download = addItem['name'];
        const event = new MouseEvent('click');
        a.dispatchEvent(event);
    });
    downloadDialogVisible.value = false;
};

const onClean = async () => {
    ElMessageBox.confirm(i18n.global.t('container.cleanLogHelper'), i18n.global.t('container.cleanLog'), {
        confirmButtonText: i18n.global.t('commons.button.confirm'),
        cancelButtonText: i18n.global.t('commons.button.cancel'),
        type: 'info',
    }).then(async () => {
        let currentNode = globalCurrentNode.value;
        if (props.node && props.node !== '') {
            currentNode = props.node;
        }
        if (logSearch.compose !== '') {
            em('update:loading', true);
            await cleanComposeLog(logSearch.resource, logSearch.compose, currentNode)
                .then(() => {
                    em('update:loading', false);
                    searchLogs();
                    MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
                })
                .finally(() => {
                    em('update:loading', false);
                });
            return;
        }
        await cleanContainerLog(logSearch.container, currentNode);
        searchLogs();
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
    });
};

const resizeObserver = ref<ResizeObserver | null>(null);

onMounted(() => {
    logSearch.container = props.container;
    logSearch.compose = props.compose;
    logSearch.resource = props.resource;

    logSearch.tail = 100;
    logSearch.mode = 'all';
    logSearch.isWatch = props.defaultFollow;

    nextTick(() => {
        initTerminal();
        if (terminalElement.value) {
            resizeObserver.value = new ResizeObserver(() => {
                fitAddon.fit();
            });
            resizeObserver.value.observe(terminalElement.value);
        }
        searchLogs();
    });
});

watch(
    () => [
        props.container,
        props.compose,
        props.resource,
        props.node,
        props.defaultFollow,
        props.defaultIsShowTimestamp,
    ],
    () => {
        syncPropsAndSearch();
    },
);

onUnmounted(() => {
    handleClose();
    onScrollDisposable?.dispose();
    if (term) {
        term.dispose();
        term = null;
    }
    resizeObserver.value?.disconnect();
});
</script>

<style scoped lang="scss">
.margin-button {
    margin-left: 0;
}
.fullScreen {
    border: none;
}
.tailClass {
    width: 160px;
}
.fetchClass {
    width: 220px;
}

.log-toolbar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
}

.log-toolbar :deep(.el-button),
.log-toolbar :deep(.el-checkbox) {
    white-space: nowrap;
    flex-shrink: 0;
}

.download-tail-helper {
    margin-top: 6px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
}

.log-container {
    height: calc(100vh - var(--custom-height, 320px));
    overflow: hidden;
    position: relative;
    background-color: #111827;
    border: 1px solid #374151;
    border-radius: 6px;
    box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.03);
    margin-top: 10px;
}

.xterm-log-viewer {
    width: 100%;
    height: 100%;
}

:deep(.xterm) {
    padding: 6px 8px !important;
}

:deep(.xterm-viewport) {
    background-color: #111827 !important;
}
</style>
