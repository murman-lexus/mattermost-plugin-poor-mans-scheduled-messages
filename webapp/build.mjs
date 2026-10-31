import {build, context} from 'esbuild';
import './labels.mjs';

const check = process.argv.includes('--check');
const options = {
    entryPoints: ['src/index.tsx'],
    outfile: 'dist/main.js',
    bundle: true,
    format: 'iife',
    target: ['chrome87', 'firefox91', 'safari14'],
    minify: !process.argv.includes('--debug'),
    write: !check,
    loader: {'.css': 'text'},
    plugins: [{
        name: 'mattermost-react',
        setup(builder) {
            builder.onResolve({filter: /^(react|react-dom)$/}, ({path}) => ({path, namespace: 'host'}));
            builder.onLoad({filter: /.*/, namespace: 'host'}, ({path}) => ({
                contents: `module.exports = window.${path === 'react' ? 'React' : 'ReactDOM'};`,
            }));
        },
    }],
};
if (process.argv.includes('--watch')) {
    const builder = await context(options);
    await builder.watch();
} else {
    await build(options);
}
