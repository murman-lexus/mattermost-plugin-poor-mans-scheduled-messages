import React, {useEffect, useRef, useState} from 'react';
import {createPortal} from 'react-dom';
import ScheduleForm, {Session} from './form';
import {Draft, apiBase, pluginID, requestID, sameDraft, showScheduledMessages, unsupported} from './api';
import css from './style.css';
import buttonLabels from './labels.json';
import useSchedulePresence, {refreshScheduledMessages} from './use_schedule_presence';
import EditorMenu from './editor_menu';
import useEditorActionsLayout from './use_editor_actions_layout';

interface Store { getState(): {entities?: {general?: {config?: {SiteURL?: string}}; teams?: {currentTeamId?: string}; users?: {currentUserId?: string; profiles?: Record<string, {locale?: string}>}}} }
interface Registry {
    registerPostEditorActionComponent(component: React.ComponentType<EditorProps>): string;
    registerWebSocketEventHandler(event: string, handler: () => void): void;
}
interface EditorProps { draft: Draft; updateText: (text: string) => void }

export function EditorAction({draft, updateText, base, language, teamID = ''}: EditorProps & {base: string; language: string; teamID?: string}) {
    const [session, setSession] = useState<Session>();
    const [open, setOpen] = useState(false);
    const [listing, setListing] = useState(false);
    const [listError, setListError] = useState(false);
    const listRequest = useRef<AbortController>();
    const latest = useRef({draft, updateText});
    const completed = useRef(false);
    const hasScheduledMessages = useSchedulePresence(base, draft.channelId);
    const {slot, compact} = useEditorActionsLayout(hasScheduledMessages);
    latest.current = {draft, updateText};
    useEffect(() => {
        setListing(false); setListError(false);
        return () => { listRequest.current?.abort(); listRequest.current = undefined; };
    }, [base, draft.channelId, draft.rootId, teamID]);
    const translations: Record<string, {menu: string; button: string; unsupported: string; list: string; error: string; close: string}> = buttonLabels;
    const labels = translations[language] || translations[language.split('-')[0]] || translations.en;
    const label = labels.button;
    const blocked = unsupported(draft);
    const openList = async () => {
        if (listRequest.current || !draft.channelId) { return; }
        const controller = new AbortController();
        listRequest.current = controller;
        setListing(true); setListError(false);
        try {
            await showScheduledMessages(base, draft.channelId, draft.rootId || '', teamID, controller.signal);
        } catch {
            if (!controller.signal.aborted) { setListError(true); }
        } finally {
            if (!controller.signal.aborted) { setListing(false); }
            if (listRequest.current === controller) { listRequest.current = undefined; }
        }
    };
    const openForm = () => {
        if (blocked || !draft.channelId) { return; }
        if (!session || !sameDraft(session.draft, draft)) { completed.current = false; setSession({id: requestID(), draft: {...draft}}); }
        setOpen(true);
    };
    const success = () => {
        completed.current = true;
        if (session) { refreshScheduledMessages(session.draft.channelId); }
        if (!session || !sameDraft(session.draft, latest.current.draft)) { return false; }
        latest.current.updateText('');
        return true;
    };
    const closeForm = () => {
        setOpen(false);
        if (completed.current) { setSession(undefined); }
        slot.current?.querySelector('button')?.focus();
    };
    return <>
        <span ref={slot} className='scheduled-editor-actions'>
        {hasScheduledMessages && compact ? <EditorMenu key={`${base}:${draft.channelId}:${draft.rootId || ''}`} label={labels.menu} disabled={!draft.channelId}>
            {(close) => <>
                <button type='button' role='menuitem' disabled={blocked} aria-label={label} title={blocked ? labels.unsupported : label} onClick={() => {close(); openForm();}}>{label}</button>
                {hasScheduledMessages && <button type='button' role='menuitem' disabled={listing} aria-label={labels.list} aria-busy={listing} title={labels.list} onClick={() => {close(); void openList();}}>{labels.list}</button>}
            </>}
        </EditorMenu> : <>
            <button type='button' className='scheduled-editor-action' disabled={blocked || !draft.channelId} aria-label={label} title={blocked ? labels.unsupported : label} onClick={openForm}>
                <svg viewBox='0 0 24 24' width='18' height='18' aria-hidden='true'><circle cx='12' cy='12' r='9' fill='none' stroke='currentColor' strokeWidth='1.7'/><path d='M12 6v6l4 2' fill='none' stroke='currentColor' strokeWidth='1.7'/></svg>
            </button>
            {hasScheduledMessages && <button type='button' className='scheduled-editor-action' disabled={listing} aria-label={labels.list} aria-busy={listing} title={labels.list} onClick={() => {void openList();}}>
                <svg viewBox='0 0 24 24' width='18' height='18' aria-hidden='true'><rect x='4' y='5' width='16' height='16' rx='2' fill='none' stroke='currentColor' strokeWidth='1.7'/><path d='M8 3v4m8-4v4M8 11h8M8 15h8M8 18h5' fill='none' stroke='currentColor' strokeWidth='1.7'/></svg>
            </button>}
        </>}
        </span>
        {listError && createPortal(<div className='scheduled-list-error' role='alert'><span>{labels.error}</span><button type='button' aria-label={labels.close} title={labels.close} onClick={() => setListError(false)}>×</button></div>, document.body)}
        {session && <ScheduleForm key={session.id} session={session} open={open} base={base} onSuccess={success} onClose={closeForm}/>}
    </>;
}

class Plugin {
    private style?: HTMLStyleElement;
    initialize(registry: Registry, store: Store) {
        this.style = document.createElement('style');
        this.style.textContent = css;
        document.head.appendChild(this.style);
        const Button = (props: EditorProps) => {
            const state = store.getState();
            const users = state.entities?.users;
            const locale = users?.profiles?.[users.currentUserId || '']?.locale || 'en';
            return <EditorAction {...props} base={apiBase(state.entities?.general?.config?.SiteURL || '')} language={locale} teamID={state.entities?.teams?.currentTeamId || ''}/>;
        };
        registry.registerPostEditorActionComponent(Button);
        registry.registerWebSocketEventHandler('ephemeral_message', () => refreshScheduledMessages());
    }
    uninitialize() { this.style?.remove(); }
}
window.registerPlugin(pluginID, new Plugin());
