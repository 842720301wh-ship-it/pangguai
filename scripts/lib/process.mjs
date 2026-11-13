import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

export const root = fileURLToPath(new URL('../../', import.meta.url))
export const metadata = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8'))
export function buildEnvironment() {
  const cache = path.join(root, '.cache')
  const directories = {
    GOCACHE: 'go-build',
    GOTMPDIR: 'go-tmp',
    GOMODCACHE: 'go-mod',
    TEMP: 'build-tmp',
    TMP: 'build-tmp',
    npm_config_cache: 'npm',
  }
  const env = { ...process.env }
  for (const [name, directory] of Object.entries(directories)) {
    env[name] = path.join(cache, directory)
    fs.mkdirSync(env[name], { recursive: true })
  }
  return env
}
export function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: root,
    env: buildEnvironment(),
    stdio: 'inherit',
    windowsHide: true,
    ...options,
  })
  if (result.error)
    throw new Error(
      `Unable to start ${path.basename(command)}: ${result.error.code ?? 'process error'}`,
    )
  if (result.status !== 0)
    throw new Error(`${path.basename(command)} failed with exit code ${result.status ?? 'unknown'}`)
}
export function webBuild() {
  run(process.execPath, ['--run', 'build'], { cwd: path.join(root, 'frontend') })
}
export function backendCheck() {
  const cwd = path.join(root, 'backend')
  run('go', ['test', '-buildvcs=false', './...'], { cwd })
  run('go', ['vet', '-buildvcs=false', './...'], { cwd })
}
