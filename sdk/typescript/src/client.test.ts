import assert from 'node:assert/strict'
import { test } from 'node:test'

import { AxmipicClient, AxmipicError, transformUrl } from './client.ts'

/** 构造一个返回统一信封的 mock fetch。 */
function mockFetch(
  handler: (url: string, init: RequestInit) => { status?: number; code?: number; message?: string; data?: unknown },
): typeof fetch {
  return (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input.toString()
    const { status = 200, code = 0, message = 'ok', data = null } = handler(url, init ?? {})
    return new Response(JSON.stringify({ code, message, data }), {
      status,
      headers: { 'Content-Type': 'application/json' },
    })
  }) as typeof fetch
}

test('login stores token and me sends it', async () => {
  const seenAuth: string[] = []
  const client = new AxmipicClient({
    baseUrl: 'http://x',
    fetch: mockFetch((url, init) => {
      seenAuth.push((init.headers as Record<string, string>)?.Authorization ?? '')
      if (url.endsWith('/auth/login')) {
        return { data: { token: 't1', expires_at: '2099-01-01T00:00:00Z', user: { id: 'u1', username: 'alice' } } }
      }
      return { data: { id: 'u1', username: 'alice' } }
    }),
  })
  const session = await client.login('alice', 'pw')
  assert.equal(session.token, 't1')
  assert.equal(client.getToken(), 't1')
  const me = await client.me()
  assert.equal((me as { username: string }).username, 'alice')
  assert.equal(seenAuth[1], 'Bearer t1')
})

test('listImages builds query params', async () => {
  let captured = ''
  const client = new AxmipicClient({
    baseUrl: 'http://x',
    fetch: mockFetch((url) => {
      captured = url
      return { data: { items: [{ id: 'i1' }], total: 1, page: 1, page_size: 20 } }
    }),
  })
  const result = await client.listImages({ page: 2, pageSize: 20, permission: 'public' })
  assert.equal(result.total, 1)
  assert.ok(captured.includes('permission=public'))
  assert.ok(captured.includes('page=2'))
})

test('error envelope throws AxmipicError', async () => {
  const client = new AxmipicClient({
    baseUrl: 'http://x',
    fetch: mockFetch(() => ({ status: 404, code: 404, message: 'image not found' })),
  })
  await assert.rejects(
    () => client.getImage('missing'),
    (err: unknown) => {
      assert.ok(err instanceof AxmipicError)
      assert.equal((err as AxmipicError).isNotFound(), true)
      return true
    },
  )
})

test('transformUrl appends parameters', () => {
  const url = transformUrl('http://x/i/k.png', { w: 400, fit: 'cover', f: 'webp', wm: 'AXmiPic' })
  assert.ok(url.includes('w=400'))
  assert.ok(url.includes('fit=cover'))
  assert.ok(url.includes('wm=AXmiPic'))
})
