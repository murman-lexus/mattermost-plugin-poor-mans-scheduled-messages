import {build} from 'esbuild';
import {spawnSync} from 'node:child_process';
import './labels.mjs';
await build({entryPoints: ['tests/editor.test.tsx'], outdir: 'dist-test', bundle: true, platform: 'node', format: 'cjs', packages: 'external', loader: {'.css': 'text'}});
const result = spawnSync(process.execPath, ['dist-test/editor.test.js'], {stdio: 'inherit'});
process.exitCode = result.status ?? 1;
