import {JSDOM} from 'jsdom';
import {webcrypto} from 'node:crypto';
const dom = new JSDOM('<!doctype html><html><body></body></html>', {url: 'https://chat.example.com/team'});
Object.assign(globalThis, {window: dom.window, document: dom.window.document, navigator: dom.window.navigator, HTMLElement: dom.window.HTMLElement, crypto: webcrypto, requestAnimationFrame: (f: () => void) => setTimeout(f, 0), cancelAnimationFrame: clearTimeout});
window.requestAnimationFrame = (callback) => setTimeout(callback, 0) as unknown as number;
window.cancelAnimationFrame = clearTimeout;
// React 17's browser scheduler otherwise keeps Node's MessageChannel alive.
Object.assign(globalThis, {MessageChannel: undefined});
Object.assign(globalThis, {MutationObserver: dom.window.MutationObserver, getComputedStyle: dom.window.getComputedStyle});
window.registerPlugin = () => {};
