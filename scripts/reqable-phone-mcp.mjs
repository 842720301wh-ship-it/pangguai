import { spawnSync } from 'node:child_process'
import http from 'node:http'
import { readFile, writeFile, mkdir } from 'node:fs/promises'
import { createInterface } from 'node:readline'
import path from 'node:path'

const root = path.resolve(import.meta.dirname, '..')
const adb =
  process.env.PANGUAI_ADB ?? 'C:/Users/84272/AppData/Local/Android/Sdk/platform-tools/adb.exe'
const serial = process.env.PANGUAI_PHONE_SERIAL ?? '3B666401BLM00000'
const localPort = Number(process.env.PANGUAI_REQABLE_LOCAL_PORT ?? 19001)
if (!Number.isInteger(localPort) || localPort < 1024 || localPort > 65535)
  throw new Error('Invalid local MCP port')
const tokenFile =
  process.env.PANGUAI_REQABLE_TOKEN_FILE ?? path.join(root, '.tools/reqable-phone-token.txt')
const token = (await readFile(tokenFile, 'utf8')).trim()
if (!token) throw new Error('Phone Reqable MCP token is missing')
const forward = spawnSync(adb, ['-s', serial, 'forward', `tcp:${localPort}`, 'tcp:9000'], {
  windowsHide: true,
  timeout: 10000,
  encoding: 'utf8',
})
if (forward.status !== 0)
  throw new Error(
    'Connect the authorized USB phone and open Reqable before starting this MCP server',
  )
let session = ''
async function rpc(message) {
  const body = JSON.stringify(message)
  const response = await new Promise((resolve, reject) => {
    const req = http.request(
      {
        host: '127.0.0.1',
        port: localPort,
        path: '/mcp?scope=all',
        method: 'POST',
        headers: {
          Host: '127.0.0.1:9000',
          'Content-Type': 'application/json',
          Accept: 'application/json, text/event-stream',
          Authorization: `Bearer ${token}`,
          'Content-Length': Buffer.byteLength(body),
          Connection: 'close',
          ...(session ? { 'Mcp-Session-Id': session } : {}),
        },
      },
      resolve,
    )
    req.setTimeout(30000, () => req.destroy(new Error('Phone Reqable MCP timeout')))
    req.on('error', reject)
    req.end(body)
  })
  if (response.statusCode < 200 || response.statusCode >= 300) {
    response.destroy()
    throw new Error(`Phone Reqable MCP HTTP ${response.statusCode}`)
  }
  session = response.headers['mcp-session-id'] ?? session
  if (response.statusCode === 202 || response.statusCode === 204 || message.id === undefined) {
    response.destroy()
    return null
  }
  let pending = ''
  const streaming = response.headers['content-type']?.includes('text/event-stream')
  response.setEncoding('utf8')
  try {
    for await (const chunk of response) {
      pending += chunk
      if (!streaming) continue
      pending = pending.replaceAll('\r\n', '\n')
      let end
      while ((end = pending.indexOf('\n\n')) >= 0) {
        const event = pending.slice(0, end)
        pending = pending.slice(end + 2)
        const data = event
          .split('\n')
          .filter((l) => l.startsWith('data:'))
          .map((l) => l.slice(5).trimStart())
          .join('\n')
        if (!data) continue
        const result = JSON.parse(data)
        if (result.id === message.id) return result
      }
    }
    return !streaming && pending.trim() ? JSON.parse(pending) : null
  } finally {
    response.destroy()
  }
}

if (process.argv.includes('--stdio')) {
  // Only JSON-RPC goes to stdout. No token or captured request diagnostics.
  for await (const line of createInterface({ input: process.stdin })) {
    if (!line.trim()) continue
    let message
    try {
      message = JSON.parse(line)
      const result = await rpc(message)
      if (result) process.stdout.write(JSON.stringify(result) + '\n')
    } catch {
      if (message?.id !== undefined)
        process.stdout.write(
          JSON.stringify({
            jsonrpc: '2.0',
            id: message.id,
            error: {
              code: -32603,
              message: 'Phone Reqable MCP connection failed; check USB, Reqable and access token',
            },
          }) + '\n',
        )
    }
  }
} else {
  await rpc({
    jsonrpc: '2.0',
    id: 1,
    method: 'initialize',
    params: {
      protocolVersion: '2024-11-05',
      capabilities: {},
      clientInfo: { name: 'panguai-local-analysis', version: '0.1.0' },
    },
  })
  await rpc({ jsonrpc: '2.0', method: 'notifications/initialized' })
  const index = process.argv.indexOf('--batch')
  if (index >= 0) {
    const calls = JSON.parse(await readFile(process.argv[index + 1], 'utf8'))
    if (!Array.isArray(calls) || calls.length > 50)
      throw new Error('Batch must contain at most 50 tool calls')
    const results = []
    for (const call of calls)
      results.push({
        tool: call.tool,
        result: await rpc({
          jsonrpc: '2.0',
          id: results.length + 2,
          method: 'tools/call',
          params: { name: call.tool, arguments: call.arguments ?? {} },
        }),
      })
    const outputIndex = process.argv.indexOf('--output')
    const target =
      outputIndex >= 0
        ? path.resolve(process.argv[outputIndex + 1])
        : path.join(root, 'work/pangguai-login/evidence/phone-mcp-result.json')
    const evidenceRoot = path.join(root, 'work/pangguai-login/evidence')
    if (!target.startsWith(evidenceRoot + path.sep))
      throw new Error('Raw MCP output must stay in work/pangguai-login/evidence/')
    await mkdir(path.dirname(target), { recursive: true })
    await writeFile(target, JSON.stringify(results, null, 2))
    const failures = results.filter((r) => r.result?.error || r.result?.result?.isError).length
    console.log(JSON.stringify({ saved: target, calls: results.length, failures }))
  } else {
    const reply = await rpc({ jsonrpc: '2.0', id: 2, method: 'tools/list', params: {} })
    console.log(
      JSON.stringify({
        connected: true,
        tools: reply.result.tools.map((t) => ({
          name: t.name,
          description: t.description,
          inputSchema: t.inputSchema,
        })),
      }),
    )
  }
}
