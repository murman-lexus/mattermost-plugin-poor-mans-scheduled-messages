export const pluginID = 'com.mattermost.plugin-poor-mans-scheduled-messages';

export interface Draft {
    channelId: string;
    rootId?: string;
    message: string;
    fileInfos?: unknown[];
    uploadsInProgress?: unknown[];
    metadata?: {priority?: {priority?: string; requested_ack?: boolean; persistent_notifications?: boolean}};
}
export interface Settings {
    locale: string;
    use_24_hour: boolean;
    timezone: string;
    date: string;
    time: string;
    destination: string;
    in_thread: boolean;
    max_message_bytes: number;
    labels: Record<string, string>;
}
export interface FormInput {
    request_id: string;
    channel_id: string;
    root_id: string;
    message: string;
    date: string;
    time: string;
    timezone: string;
}
export class APIError extends Error {
    constructor(message: string, public readonly field = '', public readonly code = '') { super(message); }
}
export function apiBase(siteURL: string): string {
    const path = new URL(siteURL || '/', window.location.origin).pathname.replace(/\/$/, '');
    return `${path}/plugins/${pluginID}/api/v1`;
}
export async function request<T>(base: string, path: string, input?: object, signal?: AbortSignal): Promise<T> {
    const controller = new AbortController();
    const abort = () => controller.abort();
    signal?.addEventListener('abort', abort);
    if (signal?.aborted) { abort(); }
    const timeout = setTimeout(abort, 30000);
    try {
        const csrf = document.cookie.split('; ').find((part) => part.startsWith('MMCSRF='))?.slice(7) || '';
        const response = await fetch(base + path, {
            method: input ? 'POST' : 'GET',
            credentials: 'same-origin',
            headers: {'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest', 'X-CSRF-Token': decodeURIComponent(csrf)},
            body: input ? JSON.stringify(input) : undefined,
            signal: controller.signal,
        });
        const data = await response.json();
        if (!response.ok) { throw new APIError(data.error?.message || 'Request failed', data.error?.field, data.error?.code); }
        return data as T;
    } finally {
        clearTimeout(timeout);
        signal?.removeEventListener('abort', abort);
    }
}
export async function showScheduledMessages(base: string, channelID: string, rootID: string, teamID: string, signal?: AbortSignal): Promise<void> {
    const serverPath = base.slice(0, base.lastIndexOf('/plugins/'));
    await request(serverPath + '/api/v4', '/commands/execute', {
        command: '/schedule list channel', channel_id: channelID, root_id: rootID, team_id: teamID,
    }, signal);
}
export function requestID(): string {
    const bytes = crypto.getRandomValues(new Uint8Array(16));
    bytes[6] = (bytes[6] & 15) | 64;
    bytes[8] = (bytes[8] & 63) | 128;
    const hex = Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('');
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}
export function unsupported(draft: Draft): boolean {
    const priority = draft.metadata?.priority;
    return Boolean(draft.fileInfos?.length || draft.uploadsInProgress?.length || priority?.priority || priority?.requested_ack || priority?.persistent_notifications);
}
export function sameDraft(left: Draft, right: Draft): boolean {
    return left.channelId === right.channelId && (left.rootId || '') === (right.rootId || '') && left.message === right.message && !unsupported(right);
}
