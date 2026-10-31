import React, {useLayoutEffect, useRef, useState} from 'react';
import {createPortal} from 'react-dom';
import {requestID} from './api';

interface Props {
    label: string;
    disabled: boolean;
    children: (close: () => void) => React.ReactNode;
}

export default function EditorMenu({label, disabled, children}: Props) {
    const [id] = useState(() => `scheduled-menu-${requestID()}`);
    const [open, setOpen] = useState(false);
    const [position, setPosition] = useState<{top: number; left: number}>();
    const trigger = useRef<HTMLButtonElement>(null);
    const menu = useRef<HTMLDivElement>(null);
    const focusLast = useRef(false);
    const items = () => Array.from(menu.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)') || []);
    const focusItem = (last = false) => {
        const enabled = items();
        (last ? enabled[enabled.length - 1] : enabled[0])?.focus();
        if (!enabled.length) { menu.current?.focus(); }
    };
    const close = (restoreFocus = true) => {
        setOpen(false); setPosition(undefined);
        if (restoreFocus) { trigger.current?.focus(); }
    };
    const place = () => {
        if (!trigger.current || !menu.current) { return; }
        const button = trigger.current.getBoundingClientRect();
        const popup = menu.current.getBoundingClientRect();
        const left = Math.max(8, Math.min(button.left, window.innerWidth - popup.width - 8));
        const above = button.top - popup.height - 8;
        const top = Math.max(8, above >= 8 ? above : Math.min(button.bottom + 8, window.innerHeight - popup.height - 8));
        setPosition((previous) => previous?.top === top && previous.left === left ? previous : {top, left});
    };
    useLayoutEffect(() => {
        if (!open) { return undefined; }
        place();
        focusItem(focusLast.current);
        const outside = (event: Event) => {
            const target = event.target as Node;
            if (!trigger.current?.contains(target) && !menu.current?.contains(target)) { close(false); }
        };
        document.addEventListener('pointerdown', outside);
        document.addEventListener('focusin', outside);
        document.addEventListener('scroll', place, true);
        window.addEventListener('resize', place);
        const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(place);
        if (menu.current) { observer?.observe(menu.current); }
        return () => {
            document.removeEventListener('pointerdown', outside);
            document.removeEventListener('focusin', outside);
            document.removeEventListener('scroll', place, true);
            window.removeEventListener('resize', place);
            observer?.disconnect();
        };
    }, [open]);
    useLayoutEffect(() => {
        if (open && document.activeElement === document.body) { focusItem(); }
    });
    useLayoutEffect(() => () => {
        if (!menu.current?.contains(document.activeElement)) { return; }
        const parent = trigger.current?.parentElement;
        queueMicrotask(() => {
            if (parent?.isConnected && document.activeElement === document.body) { parent.querySelector('button')?.focus(); }
        });
    }, []);
    const keyDown = (event: React.KeyboardEvent) => {
        event.stopPropagation();
        if (event.key === 'Escape') { event.preventDefault(); close(); return; }
        if (event.key === 'Tab') { close(); return; }
        if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) { return; }
        event.preventDefault();
        const enabled = items();
        if (!enabled.length) { return; }
        const current = enabled.indexOf(document.activeElement as HTMLButtonElement);
        const next = event.key === 'Home' ? 0 : event.key === 'End' ? enabled.length - 1 : (current + (event.key === 'ArrowUp' ? -1 : 1) + enabled.length) % enabled.length;
        enabled[next].focus();
    };
    return <>
        <button ref={trigger} type='button' className='scheduled-editor-action' disabled={disabled} aria-label={label} title={label} aria-haspopup='menu' aria-expanded={open} aria-controls={open ? id : undefined} onClick={() => {focusLast.current = false; if (open) {close();} else {setOpen(true);}}} onKeyDown={(event) => {
            if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {event.preventDefault(); event.stopPropagation(); focusLast.current = event.key === 'ArrowUp'; if (open) {focusItem(focusLast.current);} else {setOpen(true);}}
        }}>
            <svg viewBox='0 0 24 24' width='18' height='18' aria-hidden='true'><circle cx='10' cy='11' r='8' fill='none' stroke='currentColor' strokeWidth='1.7'/><path d='M10 6v5l3 2m4 4 3 3 3-3' fill='none' stroke='currentColor' strokeWidth='1.7'/></svg>
        </button>
        {open && createPortal(<div ref={menu} id={id} className='scheduled-actions-menu' role='menu' aria-label={label} tabIndex={-1} style={{top: position?.top || 0, left: position?.left || 0, visibility: position ? 'visible' : 'hidden'}} onKeyDown={keyDown} onKeyUp={(event) => event.stopPropagation()} onClick={(event) => event.stopPropagation()} onMouseDown={(event) => event.stopPropagation()}>
            {children(close)}
        </div>, document.body)}
    </>;
}
