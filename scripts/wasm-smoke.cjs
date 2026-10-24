// Execute the actual browser demo with a minimal canvas host under Node.
const fs = require('node:fs');
const assert = require('node:assert/strict');
globalThis.crypto = require('node:crypto').webcrypto;
require('../web/wasm_exec.js');
let painted = false;
const status = {};
const canvas = { getContext() { return {
  createImageData(w, h) { return { data: new Uint8ClampedArray(w * h * 4) }; },
  putImageData(data) {
    assert.equal(canvas.width, 800);
    assert.ok(canvas.height > 600);
    assert.equal(data.data.length, canvas.width * canvas.height * 4);
    assert.ok(data.data.some(v => v !== 0));
    painted = true;
  }
}; }};
globalThis.document = { getElementById(id) { return id === 'document' ? canvas : status; } };
(async () => {
  const go = new Go();
  go.argv = ['nthtml', '-demo', process.argv[2] || 'overview'];
  const result = await WebAssembly.instantiate(fs.readFileSync('web/app.wasm'), go.importObject);
  await go.run(result.instance);
  assert.ok(painted);
  assert.match(status.textContent, /Go WebAssembly/);
  console.log('WASM demo painted successfully:', process.argv[2] || 'overview');
})().catch(e => { console.error(e); process.exitCode = 1; });
