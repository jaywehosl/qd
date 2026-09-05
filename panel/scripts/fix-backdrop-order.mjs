// One-off: normalise backdrop-filter declaration order so the CSS minifier
// (esbuild) keeps the STANDARD `backdrop-filter` instead of collapsing the
// duplicate-valued pair to `-webkit-backdrop-filter` only (which Chrome does
// not apply). Rule proven by ds.css (webkit-first → works) vs LoginPage.css
// (standard-first → broken). We rewrite every adjacent
//   backdrop-filter: V;
//   -webkit-backdrop-filter: V;
// pair into webkit-first order. Pass --write to apply; default is dry-run.
import fs from 'node:fs';
import path from 'node:path';

const WRITE = process.argv.includes('--write');

function walk(dir) {
  let out = [];
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) out = out.concat(walk(p));
    else if (e.name.endsWith('.css')) out.push(p);
  }
  return out;
}

const stdRe = /^(\s*)backdrop-filter(\s*:.*;)\s*$/;
const webkitRe = /^(\s*)-webkit-backdrop-filter\s*:.*;\s*$/;

let totalPairs = 0;
const touched = [];

for (const file of walk('src')) {
  const lines = fs.readFileSync(file, 'utf8').split('\n');
  let count = 0;
  for (let i = 0; i < lines.length - 1; i++) {
    if (stdRe.test(lines[i]) && webkitRe.test(lines[i + 1])) {
      // swap so -webkit- comes first, standard last
      const tmp = lines[i];
      lines[i] = lines[i + 1];
      lines[i + 1] = tmp;
      count++;
      i++; // skip the swapped pair
    }
  }
  if (count) {
    totalPairs += count;
    touched.push(`${file.replace(/\\/g, '/')}: ${count}`);
    if (WRITE) fs.writeFileSync(file, lines.join('\n'));
  }
}

console.log(`${WRITE ? 'REWROTE' : 'WOULD REWRITE'} ${totalPairs} standard-first pairs in ${touched.length} files`);
console.log(touched.join('\n'));
