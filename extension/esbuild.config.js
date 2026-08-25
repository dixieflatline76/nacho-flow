const esbuild = require('esbuild');

async function main() {
  const isWatch = process.argv.includes('--watch');
  const opts = {
    entryPoints: ['src/extension.ts'],
    bundle: true,
    outfile: 'out/extension.js',
    external: ['vscode'],
    format: 'cjs',
    platform: 'node',
    sourcemap: true,
    minify: false,
  };

  if (isWatch) {
    const ctx = await esbuild.context(opts);
    await ctx.watch();
    console.log('Watching extension for changes...');
  } else {
    await esbuild.build(opts);
  }
}

main().catch(() => process.exit(1));