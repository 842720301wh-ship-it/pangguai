import fs from 'node:fs/promises'
import path from 'node:path'
import { root } from './lib/process.mjs'

// Generated files only. Account data, MCP configuration and research evidence
// are intentionally outside this allowlist.
const generated = [
  'dist',
  'frontend/dist',
  'android/app/build',
  'android/build',
  'android/app/src/main/jniLibs',
  'backend/cmd/android/dist',
  'backend/cmd/android/runtime-config.json',
]
const workspace = await fs.realpath(root)
const contained = (value) => value.toLowerCase().startsWith((workspace + path.sep).toLowerCase())
for (const relative of generated) {
  const target = path.resolve(root, relative)
  if (!contained(target)) throw new Error(`Refusing path outside workspace: ${relative}`)
  let resolved
  try {
    resolved = await fs.realpath(target)
  } catch (error) {
    if (error.code === 'ENOENT') continue
    throw error
  }
  if (!contained(resolved)) throw new Error(`Refusing external symlink: ${relative}`)
  await fs.rm(target, { recursive: true, force: true })
  console.log(`Removed generated output: ${relative}`)
}
