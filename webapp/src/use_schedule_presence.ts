import {useEffect, useState} from 'react';
import {request} from './api';

const updated = 'scheduled-messages-updated';

export function refreshScheduledMessages(channelID?: string) {
    window.dispatchEvent(new window.CustomEvent(updated, {detail: channelID}));
}

export default function useSchedulePresence(base: string, channelID: string): boolean {
    const [presence, setPresence] = useState({base: '', channelID: '', hasMessages: false});
    useEffect(() => {
        let controller: AbortController | undefined;
        const refresh = () => {
            if (!channelID || document.visibilityState === 'hidden') { return; }
            controller?.abort();
            const current = new AbortController();
            controller = current;
            const query = new URLSearchParams({channel_id: channelID});
            request<{count: number}>(base, `/schedule/count?${query}`, undefined, current.signal).then((result) => {
                if (!current.signal.aborted) { setPresence({base, channelID, hasMessages: result.count > 0}); }
            }).catch(() => {
                if (!current.signal.aborted) { setPresence({base, channelID, hasMessages: false}); }
            });
        };
        const changed = (event: Event) => {
            if (!(event as CustomEvent<string>).detail || (event as CustomEvent<string>).detail === channelID) { refresh(); }
        };
        refresh();
        const timer = window.setInterval(refresh, 15000);
        window.addEventListener('focus', refresh);
        document.addEventListener('visibilitychange', refresh);
        window.addEventListener(updated, changed);
        return () => {
            controller?.abort();
            window.clearInterval(timer);
            window.removeEventListener('focus', refresh);
            document.removeEventListener('visibilitychange', refresh);
            window.removeEventListener(updated, changed);
        };
    }, [base, channelID]);
    return presence.base === base && presence.channelID === channelID && presence.hasMessages;
}
