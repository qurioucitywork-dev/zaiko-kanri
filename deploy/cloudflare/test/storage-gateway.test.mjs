import test from 'node:test';
import assert from 'node:assert/strict';
import { storageGateway } from '../src/storage-gateway.ts';
const token = 'test-only-0123456789abcdef0123456789abcdef';
function fixture() {
  const values = new Map();
  return { R2_GATEWAY_TOKEN: token, IMAGES: {
    async put(key, bytes, options) { values.set(key, { bytes, type: options.httpMetadata.contentType }); },
    async get(key) {
      const value = values.get(key);
      if (!value) return null;
      return { size: value.bytes.length, body: value.bytes, writeHttpMetadata(headers) { headers.set('Content-Type', value.type); } };
    },
    async delete(key) { values.delete(key); },
  }};
}
function request(path, method = 'GET', body, extra = {}) {
  return new Request(`https://app.example/__storage/${path}`, { method, body, headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'image/png', ...extra } });
}
test('private objects reject missing and wrong credentials without touching R2', async () => {
  const env = fixture();
  env.IMAGES.get = () => { throw new Error('unauthorized object access'); };
  for (const header of ['', 'Bearer wrong']) {
    const result = await storageGateway(request('test.png', 'GET', undefined, { Authorization: header }), env);
    assert.equal(result.status, 404);
  }
});
test('encoded Japanese object keys round trip with exact bytes and content type', async () => {
  const env = fixture();
  const key = 'org/' + encodeURIComponent('商品 1.png');
  const uploaded = await storageGateway(request(key, 'PUT', new Uint8Array([1, 2, 3, 4])), env);
  assert.deepEqual(await uploaded.json(), { size: 4 });
  const download = await storageGateway(request(key), env);
  assert.equal(download.headers.get('Content-Type'), 'image/png');
  assert.equal(download.headers.get('Cache-Control'), 'no-store');
  assert.deepEqual([...new Uint8Array(await download.arrayBuffer())], [1, 2, 3, 4]);
  assert.equal((await storageGateway(request(key, 'DELETE'), env)).status, 204);
  assert.equal((await storageGateway(request(key), env)).status, 404);
});
test('invalid escaped and traversal keys are rejected', async () => {
  const env = fixture();
  for (const key of ['%E0%A4', 'a%2F..%2Fb', 'a%5Cb', '%00x']) {
    assert.equal((await storageGateway(request(key), env)).status, 400, key);
  }
});
test('unsupported methods and oversize uploads cannot write objects', async () => {
  const env = fixture();
  env.IMAGES.put = () => { throw new Error('unexpected write'); };
  assert.equal((await storageGateway(request('x', 'POST', 'x'), env)).status, 405);
  assert.equal((await storageGateway(request('x', 'PUT', 'x', { 'Content-Length': String(26 * 1024 * 1024) }), env)).status, 413);
});
test('missing/short deployment secret disables private object access', async () => {
  const env = fixture();
  env.R2_GATEWAY_TOKEN = '';
  assert.equal((await storageGateway(request('x'), env)).status, 404);
});
