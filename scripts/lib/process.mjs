import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

export const root = fileURLToPath(new URL('../../', import.meta.url))
export const metadata = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8'))
export function resolvePowerShell() {
  if (process.env.PANGUAI_POWERSHELL) return process.env.PANGUAI_POWERSHELL
  const probe = spawnSync('pwsh', ['-NoProfile', '-Command', '$PSVersionTable.PSVersion.Major'], {
    windowsHide: true,
    timeout: 10000,
    encoding: 'utf8',
  })
  if (probe.status === 0 && Number(probe.stdout.trim()) >= 7) return 'pwsh'
  if (process.platform === 'win32') {
    const programFiles = process.env.ProgramFiles || 'C:/Program Files'
    const standard = path.join(programFiles, 'PowerShell/7/pwsh.exe')
    if (fs.existsSync(standard)) return standard
    const builtin = path.join(
      process.env.SystemRoot || 'C:/Windows',
      'System32/WindowsPowerShell/v1.0/powershell.exe',
    )
    if (fs.existsSync(builtin)) return builtin
  }
  throw new Error(
    'PowerShell was not found. Install PowerShell 7 or set PANGUAI_POWERSHELL to the full path of pwsh.exe.',
  )
}
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
