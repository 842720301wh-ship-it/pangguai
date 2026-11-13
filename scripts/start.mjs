import fs from 'node:fs'
import path from 'node:path'
import { root, run } from './lib/process.mjs'
const port = Number(process.env.PANGUAI_PORT ?? 18080)
if (!Number.isInteger(port) || port < 1024 || port > 65535)
  throw new Error('PANGUAI_PORT must be 1024-65535.')
const binary = path.join(
  root,
  'dist/backend',
  process.platform === 'win32' ? 'panguai-api.exe' : 'panguai-api',
)
if (!fs.existsSync(binary))
  throw new Error('Build first with npm run build or npm run build:backend.')
try {
  run(binary, [
    '-addr',
    `127.0.0.1:${port}`,
    '-data',
    path.join(root, 'data'),
    '-web',
    path.join(root, 'dist/web'),
  ])
} catch (error) {
  console.error(error.message)
  process.exitCode = 1
}
