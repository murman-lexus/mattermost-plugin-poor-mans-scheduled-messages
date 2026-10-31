import './setup';
import React from 'react';
import ReactDOM from 'react-dom';
import {act, Simulate} from 'react-dom/test-utils';
import {afterEach, test} from 'node:test';
import assert from 'node:assert/strict';
import {EditorAction} from '../src/index';
import {apiBase, Draft, FormInput, requestID, unsupported, showScheduledMessages} from '../src/api';
import {refreshScheduledMessages} from '../src/use_schedule_presence';

let container: HTMLDivElement;
let calls: FormInput[];
let cleared: string[];
let channelCounts: Record<string, number>;
let listCalls: Array<{url: string; input: Record<string, string>}>;
let listPost: () => Promise<unknown>;
let post: () => Promise<unknown>;
let hostGeometry: {right: number} | undefined;
const draft: Draft = {channelId: 'channel', rootId: 'thread', message: 'private draft'};
const settings = {locale: 'ru', use_24_hour: false, timezone: 'Europe/Kaliningrad', date: '2027-02-01', time: '13:10', destination: 'Town Square', in_thread: true, max_message_bytes: 51200, labels: {}};
function render(value = draft) {
    const action = <EditorAction draft={value} updateText={(s) => cleared.push(s)} base='/plugins/test/api/v1' language='en' teamID='team'/>;
    act(() => {ReactDOM.render(hostGeometry ? <div className='AdvancedTextEditor__cell'><div data-testid='formattingBarContainer'>{action}<button aria-label='native more'/></div><span className='native-actions'><div className='splitSendButton'><button data-testid='SendMessageButton'/></div></span></div> : action, container);});
}
async function setup(value = draft, military = false) {
    container = document.createElement('div'); document.body.appendChild(container);
    hostGeometry = undefined;
    channelCounts = {channel: 0}; listCalls = []; listPost = async () => ({ok: true, json: async () => ({})});
    calls = []; cleared = []; post = async () => ({ok: true, json: async () => ({id: 'saved'})});
    globalThis.fetch = async (_url, options) => {
        const url = String(_url);
        if (url.includes('/schedule/count?')) { const channel = new URL(url, window.location.origin).searchParams.get('channel_id')!; return {ok:true, json:async () => ({count:channelCounts[channel] || 0})} as Response; }
        if (url.endsWith('/commands/execute')) { listCalls.push({url, input:JSON.parse(options!.body as string)}); return await listPost() as Response; }
        if (options?.method === 'POST') { calls.push(JSON.parse(options.body as string)); return await post() as Response; }
        return {ok: true, json: async () => ({...settings, use_24_hour: military})} as Response;
    };
    render(value);
    await openCreate();
}
async function openMenu() {
    if (!document.querySelector('[role=menu]')) {
        await act(async () => {Simulate.click(container.querySelector<HTMLButtonElement>('button[aria-haspopup=menu]')!);});
    }
}
async function openCreate() {
    const direct = container.querySelector<HTMLButtonElement>('button[aria-label="Schedule"]');
    if (direct) { await act(async () => {Simulate.click(direct);}); return; }
    await openMenu();
    await act(async () => {Simulate.click(document.querySelector<HTMLButtonElement>('[role=menuitem][aria-label="Schedule"]')!);});
}
async function openListMenu() { await openMenu(); }
async function clickList() { await openMenu(); await act(async () => {Simulate.click(listButton()!);}); }
async function submit() { await act(async () => {Simulate.submit(document.querySelector('form')!);}); }
function clickText(text: string) { const button = Array.from(document.querySelectorAll('button')).find((b) => b.textContent === text); assert.ok(button); act(() => Simulate.click(button)); }
afterEach(() => { if (container) {act(() => {ReactDOM.unmountComponentAtNode(container);}); container.remove();} });

test('prefills captured draft, date, explicit zone and personal 12-hour clock', async () => {
    await setup();
    assert.equal(document.querySelector('textarea')!.value, draft.message);
    assert.equal(document.querySelector<HTMLInputElement>('input[type=date]')!.value, settings.date);
    assert.equal(document.querySelector<HTMLInputElement>('input[list]')!.value, settings.timezone);
    assert.equal(document.querySelector<HTMLSelectElement>('select[aria-label="AM/PM"]')!.value, 'PM');
    await submit();
    assert.equal(calls.length, 1); assert.equal(calls[0].time, '13:10');
    assert.equal(calls[0].root_id, draft.rootId); assert.equal(calls[0].channel_id, draft.channelId);
    assert.deepEqual(cleared, ['']); assert.match(document.body.textContent!, /Message scheduled/);
});

test('24-hour preference displays 24 hours without AM/PM', async () => {
    await setup(draft, true);
    assert.equal(document.querySelector('select[aria-label="AM/PM"]'), null);
    const hour = document.querySelector('select')!;
    assert.equal(hour.options.length, 24); assert.equal(hour.value, '13');
});

test('cancel preserves edited form and editor draft', async () => {
    await setup();
    act(() => Simulate.change(document.querySelector('textarea')!, {target: {value: 'edited text'}}));
    clickText('Cancel'); assert.equal(document.querySelector('[role=dialog]'), null);
    await openCreate();
    assert.equal(document.querySelector('textarea')!.value, 'edited text'); assert.deepEqual(cleared, []);
});

test('unknown outcome retains fields and retries with same request ID', async () => {
    await setup(); post = async () => {throw new Error('sensitive response')};
    await submit(); assert.deepEqual(cleared, []); assert.equal(document.querySelector('textarea')!.value, draft.message);
    assert.doesNotMatch(document.body.textContent!, /sensitive response/);
    post = async () => ({ok: true, json: async () => ({id:'saved'})});
    await submit(); assert.equal(calls.length, 2); assert.equal(calls[0].request_id, calls[1].request_id);
    assert.deepEqual(cleared, ['']);
});

test('changing the current editor draft while form is open preserves it on success', async () => {
    await setup(); render({...draft, message:'new draft', channelId:'new-channel'});
    await submit(); assert.deepEqual(cleared, []);
    assert.equal(calls[0].message, draft.message); assert.equal(calls[0].channel_id, draft.channelId);
    assert.match(document.body.textContent!, /preserved/);
});

test('synchronous double submit sends one request', async () => {
    await setup(); let finish!: (value: unknown) => void;
    post = () => new Promise((resolve) => {finish = resolve;});
    act(() => { Simulate.submit(document.querySelector('form')!); Simulate.submit(document.querySelector('form')!); });
    assert.equal(calls.length, 1); assert.deepEqual(cleared, []);
    await act(async () => finish({ok: true, json: async () => ({id:'saved'})}));
    assert.deepEqual(cleared, ['']);
});

test('successful scheduling from an empty draft allows a new form after closing', async () => {
    await setup({...draft, message:''});
    act(() => Simulate.change(document.querySelector('textarea')!, {target:{value:'written in form'}}));
    await submit(); clickText('Close');
    await openCreate();
    assert.ok(document.querySelector('textarea')); assert.equal(document.querySelector('textarea')!.value, '');
});

test('attachments and priority cannot be silently discarded', () => {
    assert.equal(unsupported({...draft, fileInfos:[{}]}), true);
    assert.equal(unsupported({...draft, uploadsInProgress:[{}]}), true);
    assert.equal(unsupported({...draft, metadata:{priority:{requested_ack:true}}}), true);
    assert.equal(unsupported(draft), false);
});

test('API URL supports a Mattermost subpath and request IDs are UUIDs', () => {
    assert.equal(apiBase('https://chat.example.com/mattermost/'), '/mattermost/plugins/com.mattermost.plugin-poor-mans-scheduled-messages/api/v1');
    assert.match(requestID(), /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
});

test('a confirmed quota rejection allows retrying after capacity is freed', async () => {
    await setup();
    post = async () => ({ok:false, json:async () => ({error:{code:'limit',message:'Limit reached'}})});
    await submit(); assert.deepEqual(cleared, []);
    post = async () => ({ok:true, json:async () => ({id:'saved'})});
    await submit(); assert.equal(calls.length, 2); assert.notEqual(calls[0].request_id, calls[1].request_id);
    assert.equal(calls[0].message, calls[1].message); assert.deepEqual(cleared, ['']);
});


const listButton = () => document.querySelector<HTMLButtonElement>('[role=menuitem][aria-label="View scheduled messages"]');
async function refreshCount() { await act(async () => {refreshScheduledMessages();}); }

test('the list button is hidden without jobs and follows the current channel', async () => {
    await setup(); clickText('Cancel'); assert.equal(listButton(), null);
    channelCounts.channel = 1; await refreshCount(); await openListMenu();
    assert.ok(listButton()); assert.equal(listButton()!.textContent, 'View scheduled messages'); assert.equal(listButton()!.title, 'View scheduled messages');
    render({...draft, channelId:'another-channel'}); assert.equal(listButton(), null);
    await act(async () => {}); assert.equal(listButton(), null);
    channelCounts['another-channel'] = 1; await refreshCount(); await openListMenu(); assert.ok(listButton());
    channelCounts['another-channel'] = 0; await refreshCount(); assert.equal(listButton(), null);
});

test('opening the channel list preserves the draft and sends no draft text', async () => {
    await setup(); clickText('Cancel'); channelCounts.channel = 1; await refreshCount(); await openListMenu();
    await clickList();
    assert.deepEqual(cleared, []); assert.equal(listCalls.length, 1);
    assert.equal(listCalls[0].url, '/api/v4/commands/execute');
    assert.deepEqual(listCalls[0].input, {command:'/schedule list channel', channel_id:draft.channelId, root_id:draft.rootId, team_id:'team'});
    assert.doesNotMatch(JSON.stringify(listCalls[0]), /private draft/);
});

test('the list can be opened with attachments and double clicks send one request', async () => {
    await setup(); clickText('Cancel'); render({...draft, fileInfos:[{}]}); channelCounts.channel = 1; await refreshCount(); await openListMenu();
    assert.equal(document.querySelector<HTMLButtonElement>('[role=menuitem][aria-label="Schedule"]')!.disabled, true); assert.equal(listButton()!.disabled, false);
    let finish!: (value: unknown) => void; listPost = () => new Promise((resolve) => {finish = resolve;});
    act(() => {Simulate.click(listButton()!);Simulate.click(listButton()!);});
    assert.equal(listCalls.length, 1);
    await act(async () => {finish({ok:true, json:async () => ({})});});
    assert.deepEqual(cleared, []);
});

test('list failure preserves the draft and can be retried', async () => {
    await setup(); clickText('Cancel'); channelCounts.channel = 1; await refreshCount(); await openListMenu();
    listPost = async () => {throw new Error('private server error');};
    await clickList();
    assert.ok(document.querySelector('[role=alert]')); assert.doesNotMatch(document.body.textContent!, /private server error/);
    assert.deepEqual(cleared, []);
    listPost = async () => ({ok:true, json:async () => ({})});
    await clickList();
    assert.equal(document.querySelector('[role=alert]'), null); assert.equal(listCalls.length, 2);
});

test('successful creation immediately updates list visibility', async () => {
    await setup(); assert.equal(listButton(), null); channelCounts.channel = 1;
    await submit(); clickText('Close'); await openListMenu(); assert.ok(listButton());
});

test('the list command uses the Mattermost subpath', async () => {
    await setup();
    await showScheduledMessages(apiBase('https://chat.example.com/chat/'), 'channel', '', 'team');
    assert.equal(listCalls[0].url, '/chat/api/v4/commands/execute');
});

test('a delayed count from the previous channel cannot show the list button', async () => {
    await setup(); clickText('Cancel');
    const originalFetch = globalThis.fetch;
    let finish!: (value: Response) => void;
    globalThis.fetch = async (url, options) => String(url).includes('/schedule/count?channel_id=slow-channel') ? new Promise<Response>((resolve) => {finish = resolve;}) : originalFetch(url, options);
    render({...draft, channelId:'slow-channel'});
    render({...draft, channelId:'empty-channel'});
    await act(async () => {finish({ok:true, json:async () => ({count:5})} as Response);});
    assert.equal(listButton(), null);
});

test('scheduled actions use one menu when the host width cannot be measured', async () => {
    await setup(); clickText('Cancel'); channelCounts.channel = 1; await refreshCount(); await openListMenu();
    assert.equal(container.querySelectorAll('button').length, 1);
    assert.equal(container.querySelector('button')!.getAttribute('aria-haspopup'), 'menu');
});


test('the action menu supports arrow keys and Escape with focus restoration', async () => {
    await setup(); clickText('Cancel'); channelCounts.channel = 1; await refreshCount(); await openMenu();
    const create = document.querySelector<HTMLButtonElement>('[role=menuitem][aria-label="Schedule"]')!;
    assert.equal(document.activeElement, create);
    act(() => {Simulate.keyDown(document.querySelector('[role=menu]')!, {key:'ArrowDown'});});
    assert.equal(document.activeElement, listButton());
    act(() => {Simulate.keyDown(document.querySelector('[role=menu]')!, {key:'Escape'});});
    assert.equal(document.querySelector('[role=menu]'), null);
    assert.equal(document.activeElement, container.querySelector('button'));
});

test('an outside pointer closes the action menu without changing the draft', async () => {
    await setup(); clickText('Cancel'); channelCounts.channel = 1; await refreshCount(); await openMenu();
    act(() => {document.body.dispatchEvent(new window.MouseEvent('pointerdown', {bubbles:true}));});
    assert.equal(document.querySelector('[role=menu]'), null); assert.deepEqual(cleared, []);
});

test('removing the focused list item keeps keyboard focus in the menu', async () => {
    await setup(); clickText('Cancel'); channelCounts.channel = 1; await refreshCount(); await openMenu();
    act(() => {listButton()!.focus();});
    channelCounts.channel = 0; await refreshCount();
    assert.equal(listButton(), null);
    assert.equal(document.querySelector('[role=menu]'), null);
    assert.equal(document.activeElement, container.querySelector('button'));
});


test('without channel jobs the button directly opens creation', async () => {
    await setup(); clickText('Cancel');
    const button = container.querySelector('button')!;
    assert.equal(button.getAttribute('aria-label'), 'Schedule');
    assert.equal(button.hasAttribute('aria-haspopup'), false);
    await openCreate();
    assert.ok(document.querySelector('[role=dialog]')); assert.equal(document.querySelector('[role=menu]'), null);
});

test('actions switch between two buttons and a menu using space in their own editor', async () => {
    await setup(); clickText('Cancel'); channelCounts.channel = 1;
    hostGeometry = {right:220}; render(); await act(async () => {});
    const slot = container.querySelector<HTMLElement>('.scheduled-editor-actions')!;
    const cell = container.querySelector('.AdvancedTextEditor__cell')!;
    slot.getBoundingClientRect = () => ({left:100, right:166, width:66, top:100, bottom:132, height:32}) as DOMRect;
    container.querySelector<HTMLElement>('[aria-label="native more"]')!.getBoundingClientRect = () => ({width:32, height:32}) as DOMRect;
    container.querySelector<HTMLElement>('.native-actions')!.getBoundingClientRect = () => ({left:hostGeometry!.right, width:160}) as DOMRect;
    const resize = async () => {await act(async () => {window.dispatchEvent(new window.Event('resize')); await new Promise((resolve) => setTimeout(resolve, 20));});};
    await resize(); assert.equal(slot.querySelectorAll('button').length, 2); assert.equal(slot.querySelector('[aria-haspopup]'), null);
    hostGeometry.right = 180; await resize(); assert.equal(slot.querySelectorAll('button').length, 1); assert.ok(slot.querySelector('[aria-haspopup=menu]')); assert.equal(cell.hasAttribute('data-scheduled-controls-stacked'), false);
    hostGeometry.right = 140; await resize(); assert.equal(cell.hasAttribute('data-scheduled-controls-stacked'), true);
    hostGeometry.right = 220; await resize(); assert.equal(slot.querySelectorAll('button').length, 2); assert.equal(cell.hasAttribute('data-scheduled-controls-stacked'), false);
    channelCounts.channel = 0; await refreshCount(); assert.equal(slot.querySelectorAll('button').length, 1); assert.equal(slot.querySelector('[aria-haspopup]'), null);
});

test('successful confirmation fades and closes automatically', async () => {
    await setup();
    const schedule = window.setTimeout; const clear = window.clearTimeout;
    const timers = new Map<number, {callback: () => void; delay: number}>(); let next = 1000000;
    window.setTimeout = ((callback: () => void, delay: number) => {const id = next++; timers.set(id, {callback, delay}); return id;}) as typeof window.setTimeout;
    window.clearTimeout = (id) => {if (id! >= 1000000) {timers.delete(id!);} else {clear.call(window, id);}};
    try {
        await submit(); assert.ok(document.querySelector('[role=status]')); assert.equal(document.querySelector('.scheduled-fading'), null);
        const fade = Array.from(timers.values()).find((timer) => timer.delay === 1500)!;
        const finish = Array.from(timers.values()).find((timer) => timer.delay === 1750)!;
        assert.ok(fade); assert.ok(finish);
        act(() => {fade.callback();}); assert.ok(document.querySelector('.scheduled-fading')); assert.ok(document.querySelector('[role=dialog]'));
        act(() => {finish.callback();}); assert.equal(document.querySelector('[role=dialog]'), null); assert.equal(document.activeElement, container.querySelector('button'));
    } finally {window.setTimeout = schedule; window.clearTimeout = clear;}
});

for (const failure of ['network', 'access denied']) {
    test(`reopening retries defaults after ${failure} without losing the original text`, async () => {
        await setup(); clickText('Cancel');
        const originalText = 'draft preserved across a temporary defaults failure';
        render({...draft, message:originalText});
        const originalFetch = globalThis.fetch;
        let attempts = 0;
        globalThis.fetch = async (url, options) => {
            if (String(url).includes('/schedule/form?')) {
                attempts++;
                if (attempts === 1) {
                    if (failure === 'network') {throw new Error('temporary network failure');}
                    return {ok:false, json:async () => ({error:{code:'access', message:'Access unavailable'}})} as Response;
                }
            }
            return originalFetch(url, options);
        };
        await openCreate();
        assert.ok(document.querySelector('[role=alert]'));
        assert.equal(document.querySelector<HTMLButtonElement>('button[type=submit]')!.disabled, true);
        clickText('Cancel'); assert.equal(attempts, 1);
        await openCreate();
        assert.equal(attempts, 2);
        assert.equal(document.querySelector('[role=alert]'), null);
        assert.equal(document.querySelector('textarea')!.value, originalText);
        assert.equal(document.querySelector<HTMLButtonElement>('button[type=submit]')!.disabled, false);
        assert.deepEqual(cleared, []); assert.equal(calls.length, 0);
    });
}

test('reopening an already loaded form keeps edited fields without reloading defaults', async () => {
    await setup();
    act(() => {
        Simulate.change(document.querySelector('textarea')!, {target:{value:'edited form text'}});
        Simulate.change(document.querySelector('input[type=date]')!, {target:{value:'2027-03-04'}});
        Simulate.change(document.querySelector('input[list]')!, {target:{value:'UTC'}});
    });
    clickText('Cancel');
    const originalFetch = globalThis.fetch; let attempts = 0;
    globalThis.fetch = async (url, options) => {
        if (String(url).includes('/schedule/form?')) {attempts++;}
        return originalFetch(url, options);
    };
    await openCreate();
    assert.equal(attempts, 0);
    assert.equal(document.querySelector('textarea')!.value, 'edited form text');
    assert.equal(document.querySelector<HTMLInputElement>('input[type=date]')!.value, '2027-03-04');
    assert.equal(document.querySelector<HTMLInputElement>('input[list]')!.value, 'UTC');
    assert.deepEqual(cleared, []);
});

test('a late defaults response from a closed form cannot replace the retry result', async () => {
    await setup(); clickText('Cancel'); render({...draft, message:'fresh draft'});
    const originalFetch = globalThis.fetch; let attempts = 0;
    let finish!: (value: Response) => void;
    globalThis.fetch = async (url, options) => {
        if (String(url).includes('/schedule/form?') && ++attempts === 1) {
            return new Promise<Response>((resolve) => {finish = resolve;});
        }
        return originalFetch(url, options);
    };
    await openCreate(); clickText('Cancel'); await openCreate();
    await act(async () => {finish({ok:true, json:async () => ({...settings, date:'2030-12-31', timezone:'Asia/Tokyo'})} as Response);});
    assert.equal(document.querySelector<HTMLInputElement>('input[type=date]')!.value, settings.date);
    assert.equal(document.querySelector<HTMLInputElement>('input[list]')!.value, settings.timezone);
    assert.equal(document.querySelector('textarea')!.value, 'fresh draft');
});
