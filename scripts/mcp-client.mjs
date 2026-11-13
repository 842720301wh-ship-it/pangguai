import { spawn } from 'node:child_process'
import { createInterface } from 'node:readline'
import { readFile, writeFile, mkdir } from 'node:fs/promises'
import path from 'node:path'

// A one-shot stdio MCP client for the installed local reverse-engineering tools.
// Raw capture output must go into the ignored evidence directory, never a log.
const flags = process.argv.slice(2)
const arg = (key, fallback = '') => {
  const i = flags.indexOf(key)
  return i >= 0 ? flags[i + 1] : fallback
}
const root = path.resolve(import.meta.dirname, '..')
const server = arg('--server', 'reqable')
const command =
  server === 'reqable'
    ? 'D:/Reqable/mcp-server.exe'
    : path.join(root, '.tools/runtime/node_modules/node/bin/node.exe')
const serverArgs =
  server === 'reqable'
    ? ['--scope', 'all']
    : [path.join(root, '.tools/runtime/node_modules/rea-agents/scripts/rea.mjs'), 'mcp']
const child = spawn(command, serverArgs, { stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true })
let id = 0
const pending = new Map()
const send = (message) => child.stdin.write(JSON.stringify(message) + '\n')
const rpc = (method, params = {}) =>
  new Promise((resolve, reject) => {
    const requestId = ++id
    pending.set(requestId, { resolve, reject })
    send({ jsonrpc: '2.0', id: requestId, method, params })
  })
createInterface({ input: child.stdout }).on('line', (line) => {
  let value
  try {
    value = JSON.parse(line)
  } catch {
    return
  }
  const entry = pending.get(value.id)
  if (!entry) return
  pending.delete(value.id)
  value.error ? entry.reject(new Error(JSON.stringify(value.error))) : entry.resolve(value.result)
})
let diagnostics = ''
child.stderr.on('data', (chunk) => {
  diagnostics = (diagnostics + chunk.toString()).slice(-2000)
})
child.on('error', (error) => {
  for (const p of pending.values()) p.reject(error)
})
child.on('exit', () => {
  for (const p of pending.values()) p.reject(new Error('MCP server exited: ' + diagnostics))
})
const timeout = setTimeout(() => {
  for (const p of pending.values()) p.reject(new Error('MCP operation timed out'))
  child.kill()
}, 90000)
try {
  await rpc('initialize', {
    protocolVersion: '2024-11-05',
    capabilities: {},
    clientInfo: { name: 'panguai-local-analysis', version: '0.1.0' },
  })
  send({ jsonrpc: '2.0', method: 'notifications/initialized' })
  const tool = arg('--tool')
  const batchFile = arg('--batch-file')
  let result
  if (batchFile) {
    const calls = JSON.parse(await readFile(path.resolve(batchFile), 'utf8'))
    if (!Array.isArray(calls) || calls.length > 100)
      throw new Error('Batch must contain at most 100 calls')
    const results = []
    for (const call of calls) {
      const value = await rpc('tools/call', { name: call.tool, arguments: call.arguments ?? {} })
      results.push({ tool: call.tool, arguments: call.arguments ?? {}, result: value })
    }
    result = { results, isError: results.some((r) => r.result.isError) }
  } else if (tool) {
    const argsFile = arg('--args-file')
    const args = argsFile ? JSON.parse(await readFile(path.resolve(argsFile), 'utf8')) : {}
    result = await rpc('tools/call', { name: tool, arguments: args })
  } else {
    result = await rpc('tools/list')
  }
  const output = arg('--output')
  if (output) {
    const target = path.resolve(output)
    const evidenceRoot = path.join(root, 'work/pangguai-login/evidence')
    if (!target.startsWith(evidenceRoot + path.sep))
      throw new Error('Raw MCP output must stay in work/pangguai-login/evidence/')
    await mkdir(path.dirname(target), { recursive: true })
    await writeFile(target, JSON.stringify(result, null, 2))
    console.log(
      JSON.stringify({ saved: path.relative(root, target), isError: result.isError ?? false }),
    )
  } else if (!tool && !batchFile) {
    const pattern = new RegExp(
      arg(
        '--filter',
        'tab_list|capture_live|capture_record|inspect_android|adb|binary_session|open_binary|list_tools|inspect_network',
      ),
    )
    console.log(
      JSON.stringify(
        result.tools
          .filter((t) => pattern.test(t.name))
          .map((t) => ({ name: t.name, description: t.description, inputSchema: t.inputSchema })),
        null,
        2,
      ),
    )
  } else {
    console.log(JSON.stringify(result, null, 2))
  }
} catch (error) {
  console.error(error.message)
  process.exitCode = 1
} finally {
  clearTimeout(timeout)
  child.kill()
}
