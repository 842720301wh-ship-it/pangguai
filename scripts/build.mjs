import fs from 'node:fs'
import path from 'node:path'
import { root, metadata, run, webBuild, backendCheck, resolvePowerShell } from './lib/process.mjs'

const args = new Set(process.argv.slice(2))
const modes = ['--web', '--backend', '--android'].filter((flag) => args.has(flag))
if (
  modes.length > 1 ||
  [...args].some((flag) => !['--web', '--backend', '--android', '--offline'].includes(flag))
)
  throw new Error('Use one of --web, --backend, --android, with optional --offline.')
const mode = modes[0] ?? '--all'
try {
  console.log(`[build] ${metadata.panguai.displayName} ${metadata.version} (${mode})`)
  if (mode !== '--web') backendCheck()
  if (mode !== '--backend') {
    console.log('[build] Vue frontend')
    webBuild()
  }
  if (mode === '--all' || mode === '--backend') {
    const output = path.join(
      root,
      'dist/backend',
      process.platform === 'win32' ? 'panguai-api.exe' : 'panguai-api',
    )
    fs.mkdirSync(path.dirname(output), { recursive: true })
    run(
      'go',
      [
        'build',
        '-buildvcs=false',
        '-trimpath',
        '-ldflags',
        `-s -w -X panguai/internal/buildinfo.Version=${metadata.version}`,
        '-o',
        output,
        './cmd/panguai',
      ],
      { cwd: path.join(root, 'backend') },
    )
    console.log('[build] dist/backend ready')
  }
  if (mode === '--all' || mode === '--android') {
    console.log('[build] Android APK (Go library + Gradle + release/Android)')
    const command = resolvePowerShell()
    const parameters = [
      '-NoProfile',
      '-File',
      path.join(root, 'scripts/build-android.ps1'),
      '-SkipWebBuild',
    ]
    if (args.has('--offline')) parameters.push('-Offline')
    run(command, parameters)
    const apk = path.join(
      root,
      'release/Android',
      `${metadata.panguai.displayName}-${metadata.version}-debug.apk`,
    )
    if (!fs.existsSync(apk) || fs.statSync(apk).size === 0)
      throw new Error(`APK was not generated: ${apk}`)
    console.log(
      `[build] APK ready: ${apk} (${(fs.statSync(apk).size / 1024 / 1024).toFixed(1)} MB)`,
    )
  }
  console.log(`[build] Complete (${metadata.version})`)
} catch (error) {
  console.error(error.message)
  process.exitCode = 1
}
