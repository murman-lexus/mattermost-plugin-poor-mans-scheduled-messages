import React, {useEffect, useRef, useState} from 'react';
import {createPortal} from 'react-dom';
import {APIError, Draft, FormInput, Settings, request, requestID} from './api';

const fallback = {
    title: 'Schedule message', message: 'Message', date: 'Date', time: 'Time', timezone: 'Time zone',
    destination: 'Destination', thread: 'Reply in thread', submit: 'Schedule', cancel: 'Cancel', close: 'Close',
    loading: 'Loading…', success: 'Message scheduled', error: 'Could not complete the request.',
    unsupported: 'Only text messages without files or priority are supported.',
    draft_changed: 'The editor draft changed and has been preserved.',
    retry: 'If the result is unknown, retry without changing the fields or check /schedule list.',
};
export interface Session { id: string; draft: Draft }
interface Props {
    open: boolean;
    session: Session;
    base: string;
    onClose: () => void;
    onSuccess: () => boolean;
}

export default function ScheduleForm({open, session, base, onClose, onSuccess}: Props) {
    const [settings, setSettings] = useState<Settings>();
    const [message, setMessage] = useState(session.draft.message);
    const [date, setDate] = useState('');
    const [time, setTime] = useState('');
    const [timezone, setTimezone] = useState('');
    const [error, setError] = useState<APIError>();
    const [busy, setBusy] = useState(false);
    const [done, setDone] = useState(false);
    const [fading, setFading] = useState(false);
    const [preserved, setPreserved] = useState(false);
    const inFlight = useRef(false);
    const operationID = useRef(session.id);
    const dialog = useRef<HTMLDivElement>(null);
    const closeCallback = useRef(onClose);
    closeCallback.current = onClose;
    const labels = {...fallback, ...settings?.labels};
    const errorLabel = labels.error;

    useEffect(() => {
        if (!open || settings) { return undefined; }
        const controller = new AbortController();
        setError(undefined);
        const query = new URLSearchParams({channel_id: session.draft.channelId, root_id: session.draft.rootId || ''});
        request<Settings>(base, `/schedule/form?${query}`, undefined, controller.signal).then((value) => {
            if (controller.signal.aborted) { return; }
            setSettings(value); setDate(value.date); setTime(value.time); setTimezone(value.timezone);
        }).catch((reason: unknown) => {
            if (!controller.signal.aborted) { setError(reason instanceof APIError ? reason : new APIError(fallback.error)); }
        });
        return () => controller.abort();
    }, [base, session, open, settings]);

    useEffect(() => {
        if (!open) { return undefined; }
        const previous = document.activeElement as HTMLElement | null;
        const overflow = document.body.style.overflow;
        document.body.style.overflow = 'hidden';
        dialog.current?.focus();
        return () => {
            document.body.style.overflow = overflow;
            if (previous?.isConnected) { previous.focus(); }
        };
    }, [open]);

    useEffect(() => {
        if (!done || !open) { return undefined; }
        const delay = preserved ? 3500 : 1500;
        const duration = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ? 0 : 250;
        const fade = window.setTimeout(() => setFading(true), delay);
        const finish = window.setTimeout(() => closeCallback.current(), delay + duration);
        return () => { window.clearTimeout(fade); window.clearTimeout(finish); };
    }, [done, open, preserved]);

    const close = () => { if (!inFlight.current) { onClose(); } };
    const keyDown = (event: React.KeyboardEvent) => {
        event.stopPropagation();
        if (event.key === 'Escape') { event.preventDefault(); close(); }
        if (event.key !== 'Tab' || !dialog.current) { return; }
        const controls = Array.from(dialog.current.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled)'));
        const first = controls[0]; const last = controls[controls.length - 1];
        if (!first) { event.preventDefault(); return; }
        if (event.shiftKey && (document.activeElement === first || document.activeElement === dialog.current)) { event.preventDefault(); last.focus(); }
        else if (!event.shiftKey && (document.activeElement === last || document.activeElement === dialog.current)) { event.preventDefault(); first.focus(); }
    };

    const submit = async (event: React.FormEvent) => {
        event.preventDefault(); event.stopPropagation();
        if (!settings || inFlight.current || done) { return; }
        inFlight.current = true; setBusy(true); setError(undefined);
        const input: FormInput = {request_id: operationID.current, channel_id: session.draft.channelId, root_id: session.draft.rootId || '', message, date, time, timezone};
        try {
            await request<{id: string}>(base, '/schedule', input);
            let cleared = false;
            try { cleared = onSuccess(); } catch { /* Scheduling succeeded even if the editor callback failed. */ }
            setPreserved(!cleared); setDone(true);
        } catch (reason) {
            // A quota rejection is a confirmed cancelled creation. A new ID
            // allows retrying after capacity is freed; unknown outcomes keep it.
            if (reason instanceof APIError && reason.code === 'limit') { operationID.current = requestID(); }
            setError(reason instanceof APIError ? reason : new APIError(errorLabel));
        } finally { inFlight.current = false; setBusy(false); }
    };

    if (!open) { return null; }
    const bytes = new TextEncoder().encode(message).length;
    const [hour = '00', minute = '00'] = time.split(':');
    const hour24 = Number(hour);
    const twelve = !settings?.use_24_hour;
    const changeHour = (value: string) => {
        const h = twelve ? Number(value) % 12 + (hour24 >= 12 ? 12 : 0) : Number(value);
        setTime(`${String(h).padStart(2, '0')}:${minute}`);
    };
    const zones = (Intl as typeof Intl & {supportedValuesOf?: (key: string) => string[]}).supportedValuesOf?.('timeZone') || [timezone, 'UTC'];
    const fieldError = (field: string) => error?.field === field ? `${session.id}-error` : undefined;

    return createPortal(
        <div className={`scheduled-overlay${fading ? ' scheduled-fading' : ''}`} onClick={(event) => event.stopPropagation()} onMouseDown={(event) => event.stopPropagation()} onKeyDown={keyDown} onKeyUp={(event) => event.stopPropagation()}>
            <div ref={dialog} className='scheduled-dialog' role='dialog' aria-modal='true' aria-labelledby={`${session.id}-title`} tabIndex={-1} lang={settings?.locale || 'en'}>
                <header><h2 id={`${session.id}-title`}>{labels.title}</h2><button type='button' className='scheduled-icon-close' aria-label={labels.close} disabled={busy} onClick={close}>×</button></header>
                {done ? <section role='status'><p className='scheduled-success'>✓ {labels.success}</p><p>{date} · {settings?.use_24_hour ? time : `${hour24 % 12 || 12}:${minute} ${hour24 >= 12 ? 'PM' : 'AM'}`} · {timezone}</p>{preserved && <p>{labels.draft_changed}</p>}<button type='button' className='scheduled-primary' onClick={onClose}>{labels.close}</button></section> :
                    <form onSubmit={submit}>
                        {!settings && !error && <p role='status'>{labels.loading}</p>}
                        {settings && <>
                            <p className='scheduled-destination'><strong>{labels.destination}:</strong> {settings.destination}{settings.in_thread && <span> · {labels.thread}</span>}</p>
                            <label>{labels.message}<textarea autoFocus value={message} required disabled={busy} aria-describedby={fieldError('message')} onChange={(event) => setMessage(event.target.value)}/></label>
                            <p className={bytes > settings.max_message_bytes ? 'scheduled-error' : 'scheduled-count'}>{bytes} / {settings.max_message_bytes}</p>
                            <div className='scheduled-fields'>
                                <label>{labels.date}<input type='date' required value={date} disabled={busy} aria-describedby={fieldError('date')} onChange={(event) => setDate(event.target.value)}/></label>
                                <fieldset><legend>{labels.time}</legend><div className='scheduled-time'>
                                    <select aria-label={`${labels.time}: ${twelve ? '1–12' : '0–23'}`} value={twelve ? String(hour24 % 12 || 12) : hour} disabled={busy} onChange={(event) => changeHour(event.target.value)}>{Array.from({length: twelve ? 12 : 24}, (_, i) => twelve ? String(i + 1) : String(i).padStart(2, '0')).map((h) => <option key={h}>{h}</option>)}</select>
                                    <span>:</span><select aria-label={`${labels.time}: 00–59`} value={minute} disabled={busy} onChange={(event) => setTime(`${hour}:${event.target.value}`)}>{Array.from({length: 60}, (_, i) => String(i).padStart(2, '0')).map((m) => <option key={m}>{m}</option>)}</select>
                                    {twelve && <select aria-label='AM/PM' value={hour24 >= 12 ? 'PM' : 'AM'} disabled={busy} onChange={(event) => setTime(`${String(hour24 % 12 + (event.target.value === 'PM' ? 12 : 0)).padStart(2, '0')}:${minute}`)}><option>AM</option><option>PM</option></select>}
                                </div></fieldset>
                            </div>
                            <label>{labels.timezone}<input list={`${session.id}-zones`} required value={timezone} disabled={busy} aria-describedby={fieldError('timezone')} onChange={(event) => setTimezone(event.target.value)}/><datalist id={`${session.id}-zones`}>{Array.from(new Set([timezone, 'UTC', ...zones])).map((zone) => <option key={zone} value={zone}/>)}</datalist></label>
                        </>}
                        {error && <div className='scheduled-error' role='alert' id={`${session.id}-error`}><p>{error.message}</p>{(!error.code || ['storage', 'pending'].includes(error.code)) && <p>{labels.retry}</p>}</div>}
                        <footer><button type='button' disabled={busy} onClick={close}>{labels.cancel}</button><button type='submit' className='scheduled-primary' disabled={busy || !settings || !message.trim() || bytes > settings.max_message_bytes}>{busy ? labels.loading : labels.submit}</button></footer>
                    </form>}
            </div>
        </div>, document.body,
    );
}
