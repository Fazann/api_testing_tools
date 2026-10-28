// Fails when any frontend source line exceeds the maximum length. Prettier wraps
// code at 120 columns but cannot split long strings or template text, so this
// check catches what it leaves behind.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

const MAX = 120;
const roots = ['src', 'vite.config.ts'];
const problems = [];

function visit(path) {
  if (statSync(path).isDirectory()) {
    for (const name of readdirSync(path)) visit(join(path, name));
    return;
  }
  if (!/\.(ts|vue|css|mjs)$/.test(path)) return;
  readFileSync(path, 'utf8')
    .split('\n')
    .forEach((line, i) => {
      if (line.length > MAX) problems.push(`${path}:${i + 1} is ${line.length} characters`);
    });
}

roots.forEach(visit);
if (problems.length) {
  console.error(`Lines longer than ${MAX} characters:\n${problems.join('\n')}`);
  process.exit(1);
}
