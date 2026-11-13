import { root, run, backendCheck } from './lib/process.mjs'
import path from 'node:path'
try {
  run(process.execPath, ['--run', 'typecheck'], { cwd: path.join(root, 'frontend') })
  backendCheck()
} catch (error) {
  console.error(error.message)
  process.exitCode = 1
}
