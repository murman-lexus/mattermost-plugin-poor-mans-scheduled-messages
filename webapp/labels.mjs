import {readdirSync, readFileSync, writeFileSync} from 'node:fs';
const directory = new URL('../server/formatter/locales/', import.meta.url);
const labels = {};
for (const file of readdirSync(directory).filter((file) => file.endsWith('.json'))) {
    const {messages} = JSON.parse(readFileSync(new URL(file, directory), 'utf8'));
    labels[file.slice(0, -5)] = {menu: messages.list_header.replace(/^#+\s*/, ''), button: messages.form_button, unsupported: messages.form_unsupported, list: messages.form_list_button, error: messages.form_error, close: messages.form_close};
}
writeFileSync(new URL('./src/labels.json', import.meta.url), JSON.stringify(labels));
