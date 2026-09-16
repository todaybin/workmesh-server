export interface SseEvent {
    event: string;
    data: string;
    id?: string;
}

export function buildSseRequestHeaders(currentNode: string, lastEventId = 0): Record<string, string> {
    const headers: Record<string, string> = {
        Accept: 'text/event-stream',
        'Cache-Control': 'no-cache',
        CurrentNode: encodeURIComponent(String(currentNode || '')),
    };
    if (Number.isSafeInteger(lastEventId) && lastEventId > 0) {
        headers['Last-Event-ID'] = String(lastEventId);
    }
    return headers;
}

/**
 * Incremental SSE parser for fetch response bodies.
 * It preserves incomplete lines between chunks and supports named events,
 * CRLF, comments, repeated data fields and event IDs.
 */
export class SseParser {
    private buffer = '';
    private event = 'message';
    private data: string[] = [];
    private id: string | undefined;

    push(chunk: string): SseEvent[] {
        this.buffer += chunk;
        const events: SseEvent[] = [];
        const lines = this.buffer.split('\n');
        this.buffer = lines.pop() || '';
        for (const rawLine of lines) {
            this.consumeLine(rawLine.endsWith('\r') ? rawLine.slice(0, -1) : rawLine, events);
        }
        return events;
    }

    finish(): SseEvent[] {
        const events: SseEvent[] = [];
        if (this.buffer !== '') {
            this.consumeLine(this.buffer.endsWith('\r') ? this.buffer.slice(0, -1) : this.buffer, events);
            this.buffer = '';
        }
        this.dispatch(events);
        return events;
    }

    private consumeLine(line: string, events: SseEvent[]) {
        if (line === '') {
            this.dispatch(events);
            return;
        }
        if (line.startsWith(':')) return;

        const separator = line.indexOf(':');
        const field = separator >= 0 ? line.slice(0, separator) : line;
        let value = separator >= 0 ? line.slice(separator + 1) : '';
        if (value.startsWith(' ')) value = value.slice(1);
        switch (field) {
            case 'event':
                this.event = value || 'message';
                break;
            case 'data':
                this.data.push(value);
                break;
            case 'id':
                this.id = value;
                break;
        }
    }

    private dispatch(events: SseEvent[]) {
        if (this.data.length === 0) {
            this.event = 'message';
            return;
        }
        events.push({
            event: this.event || 'message',
            data: this.data.join('\n'),
            id: this.id,
        });
        this.event = 'message';
        this.data = [];
    }
}
