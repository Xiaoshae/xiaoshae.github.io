const assert = require('node:assert/strict');
const { test } = require('node:test');
const { uploadToOss } = require('./index');

const config = {
  Url: 'https://example.invalid/upload?signature=secret',
  OSSAccessKeyId: 'key-id',
  Signature: 'secret',
  Key: 'test.zip',
  Policy: 'test-policy',
};

function mockUpload(t, results) {
  const log = t.mock.method(console, 'log', () => {});
  t.mock.method(globalThis, 'setTimeout', (callback) => queueMicrotask(callback));
  const fetch = t.mock.method(globalThis, 'fetch', async (_url, { signal }) => {
    assert.ok(signal instanceof AbortSignal);
    const result = results.shift();
    if (result instanceof Error) throw result;
    return new Response(null, { status: result });
  });
  return { fetch: fetch.mock, log: log.mock };
}

test('retries a transient response and then succeeds', async (t) => {
  const { fetch, log } = mockUpload(t, [503, 204]);
  await uploadToOss(config, Buffer.from('zip'));
  assert.equal(fetch.callCount(), 2);
  assert.match(log.calls[0].arguments[0], /attempt 1\/3: HTTP 503 in /);
  assert.match(log.calls[1].arguments[0], /attempt 2\/3: HTTP 204 in /);
});

test('does not retry an invalid signed request', async (t) => {
  const { fetch } = mockUpload(t, [403]);
  await assert.rejects(uploadToOss(config, Buffer.from('zip')), /after 1 attempt\(s\): HTTP 403/);
  assert.equal(fetch.callCount(), 1);
});

test('limits timeouts to three attempts without logging the signed URL', async (t) => {
  const timeout = t.mock.method(AbortSignal, 'timeout', () => AbortSignal.abort());
  const error = new Error(`request aborted: ${config.Url}`);
  const { fetch, log } = mockUpload(t, [error, error, error]);
  await assert.rejects(uploadToOss(config, Buffer.from('zip')), {
    message: 'OSS upload failed after 3 attempt(s): timeout',
  });
  assert.equal(fetch.callCount(), 3);
  assert.deepEqual(timeout.mock.calls.map(({ arguments: args }) => args), [[30_000], [30_000], [30_000]]);
  assert.doesNotMatch(log.calls.map(({ arguments: args }) => args[0]).join('\n'), /secret/);
});
