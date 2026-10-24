// Execute the actual browser demo with a minimal canvas host under Node.
const fs = require('node:fs');
const assert = require('node:assert/strict');
globalThis.crypto = require('node:crypto').webcrypto;
require('../web/wasm_exec.js');
let painted = false;
let paints = 0;
const status = {};
const tty = {};
const reference = {};
const canvas = { getContext() { return {
  createImageData(w, h) { return { data: new Uint8ClampedArray(w * h * 4) }; },
  putImageData(data) {
    assert.ok(canvas.width >= 32 && canvas.width <= 4096);
    assert.ok(canvas.height > 0);
    assert.equal(data.data.length, canvas.width * canvas.height * 4);
    assert.ok(data.data.some(v => v !== 0));
    painted = true;
    paints++;
  }
}; }};
globalThis.document = { getElementById(id) { return id === 'document' ? canvas : id === 'tty' ? tty : id === 'browser-reference' ? reference : status; } };
(async () => {
  const go = new Go();
  // A standalone Node host owns timer cleanup after the Go runtime exits.
  const scheduled = new Set();
  const hostSetTimeout = globalThis.setTimeout;
  const hostClearTimeout = globalThis.clearTimeout;
  globalThis.setTimeout = (fn, delay, ...args) => {
    const id = hostSetTimeout(() => { scheduled.delete(id); fn(...args); }, delay);
    scheduled.add(id);
    return id;
  };
  globalThis.clearTimeout = id => { scheduled.delete(id); hostClearTimeout(id); };
  go.exit = code => {
    for (const id of scheduled) hostClearTimeout(id);
    scheduled.clear();
    globalThis.setTimeout = hostSetTimeout;
    globalThis.clearTimeout = hostClearTimeout;
    if (code !== 0) process.exitCode = code;
  };
  go.argv = ['nthtml', '-demo', process.argv[2] || 'overview'];
  const result = await WebAssembly.instantiate(fs.readFileSync('web/app.wasm'), go.importObject);
  const running = go.run(result.instance);
  assert.ok(globalThis.nthtmlDemo);
  try {
  assert.ok(painted);
  const demo = process.argv[2] || 'overview';
  const sourcePath = demo === 'overview' ? 'examples/demo/sample.html' : `examples/demo/documents/${demo}.html`;
  assert.equal(reference.srcdoc, '<!doctype html><meta charset="utf-8">\n' + fs.readFileSync(sourcePath, 'utf8'));
  assert.ok(tty.textContent.length > 100);
  assert.ok(!tty.textContent.includes("<style>"));
  assert.match(status.textContent, /Go WebAssembly/);
  assert.equal(tty.nthtmlSegments.map(s => s.text).join(''),tty.textContent);
  const linkIDs = canvas.nthtmlLinks.map(l => l.id);
  if (demo === 'guide') {
    assert.equal(linkIDs.length,4);
    assert.ok(tty.nthtmlSegments.some(s => s.href === 'https://nimbleterminal.dev/'));
    assert.ok(tty.textContent.includes('1. Build'));
    assert.ok(tty.textContent.includes('    println(message)'));
    for (const link of canvas.nthtmlLinks) {
      assert.ok(link.regions.length > 0);
      assert.ok(link.regions.every(r => r.width > 0 && r.height > 0 && r.x >= 0 && r.x+r.width <= canvas.width));
    }
  }
  const initialHeight = canvas.height;
  const initialLines = tty.textContent.split('\n').length;
  assert.equal(nthtmlDemo.reflow(400,40),'');
  assert.equal(canvas.width,400);
  assert.deepEqual(canvas.nthtmlLinks.map(l => l.id),linkIDs);
  assert.equal(tty.nthtmlSegments.map(s => s.text).join(''),tty.textContent);
  assert.ok(canvas.height > initialHeight, 'narrower typeset layout must grow vertically');
  assert.ok(tty.textContent.split('\n').length > initialLines, 'TTY must rewrap');
  const stablePaints = paints;
  assert.equal(nthtmlDemo.reflow(400,40),'');
  assert.equal(paints,stablePaints,'unchanged size should not repaint');
  assert.ok(nthtmlDemo.reflow(NaN,40));
  assert.equal(canvas.width,400,'invalid dimensions must preserve the previous output');
  assert.equal(nthtmlDemo.reflow(800,80),'');
  assert.equal(canvas.width,800);
  assert.equal(canvas.height,initialHeight);
  console.log('WASM demo painted and reflowed successfully:', process.argv[2] || 'overview');
  } finally {
    globalThis.nthtmlDemo?.close();
  }
  await running;
})().catch(e => { console.error(e); process.exitCode = 1; });
