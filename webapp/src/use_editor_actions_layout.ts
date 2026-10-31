import {useLayoutEffect, useRef, useState} from 'react';

const buttonWidth = 32;
const buttonGap = 2;
const stackedAttribute = 'data-scheduled-controls-stacked';

// Isolate the Mattermost 10.11 editor layout adapter from the action workflow.
// Measure the editor, not the viewport: a thread can be narrow on a wide screen.
export default function useEditorActionsLayout(hasList: boolean) {
    const slot = useRef<HTMLSpanElement>(null);
    const [compact, setCompact] = useState(true);
    useLayoutEffect(() => {
        const element = slot.current;
        const bar = element?.closest<HTMLElement>('[data-testid="formattingBarContainer"]');
        const cell = element?.closest<HTMLElement>('.AdvancedTextEditor__cell');
        if (!element || !bar || !cell) { setCompact(true); return undefined; }
        const original = cell.getAttribute(stackedAttribute);
        let frame = 0;
        let watchedActions: HTMLElement | undefined;
        const resize = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(schedule);
        function measure() {
            frame = 0;
            const send = cell!.querySelector('[data-testid="SendMessageButton"]');
            const actions = send?.closest('.splitSendButton')?.parentElement;
            if (!actions || !element!.isConnected) { setCompact(true); cell!.removeAttribute(stackedAttribute); return; }
            if (actions !== watchedActions) {
                if (watchedActions) { resize?.unobserve(watchedActions); }
                resize?.observe(actions); watchedActions = actions;
            }
            const gap = parseFloat(getComputedStyle(bar!).columnGap) || buttonGap;
            let tailWidth = 0;
            for (let next = element!.nextElementSibling; next; next = next.nextElementSibling) {
                const box = next.getBoundingClientRect();
                const style = getComputedStyle(next);
                if (box.width && style.position !== 'fixed' && style.position !== 'absolute' && style.visibility !== 'hidden') { tailWidth += box.width + gap; }
            }
            const free = actions.getBoundingClientRect().left - element!.getBoundingClientRect().left - tailWidth - 4;
            setCompact(hasList && free < buttonWidth * 2 + buttonGap);
            // Even the native controls can collide in very narrow editors.
            // Give the absolute Send toolbar its own row rather than covering it.
            cell!.toggleAttribute(stackedAttribute, free < buttonWidth);
        }
        function schedule() {
            if (!frame) { frame = window.requestAnimationFrame(measure); }
        }
        resize?.observe(bar); resize?.observe(element); resize?.observe(cell);
        const mutations = new MutationObserver(schedule);
        mutations.observe(cell, {childList: true, subtree: true});
        window.addEventListener('resize', schedule);
        measure();
        return () => {
            resize?.disconnect(); mutations.disconnect();
            window.removeEventListener('resize', schedule);
            if (frame) { window.cancelAnimationFrame(frame); }
            if (original === null) { cell.removeAttribute(stackedAttribute); } else { cell.setAttribute(stackedAttribute, original); }
        };
    }, [hasList]);
    return {slot, compact};
}
