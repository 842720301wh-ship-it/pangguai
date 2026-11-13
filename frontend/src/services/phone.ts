import { aboutImages, groupNumber } from '../config/about'

export interface ActualLocation {
  latitude: number
  longitude: number
  accuracy: number
  timestamp: number
  coordinate_system: 'wgs84'
}
interface NativeBridge {
  requestLocation(id: string): void
  scanCode(id: string): void
  saveImage?(id: string, image: string): void
  copyGroupNumber?(id: string): void
}
declare global {
  interface Window {
    PanguaiNative?: NativeBridge
  }
}

function nativeRequest<T>(method: keyof NativeBridge, argument?: string): Promise<T> {
  return new Promise((resolve, reject) => {
    const bridge = window.PanguaiNative
    if (!bridge || typeof bridge[method] !== 'function') {
      reject(new Error('请在水到渠成安装版使用此功能'))
      return
    }
    const id = crypto.randomUUID()
    const listener = (event: Event) => {
      const message = (event as CustomEvent).detail
      if (message?.id !== id) return
      cleanup()
      if (message.error) reject(new Error(message.error))
      else resolve(message.data as T)
    }
    const cleanup = () => {
      clearTimeout(timer)
      window.removeEventListener('panguai-native-result', listener)
    }
    const timer = setTimeout(() => {
      cleanup()
      reject(new Error('操作超时，请重试'))
    }, 135000)
    window.addEventListener('panguai-native-result', listener)
    try {
      if (method === 'saveImage') bridge.saveImage!(id, argument!)
      else bridge[method]!(id)
    } catch {
      cleanup()
      reject(new Error('手机功能调用失败，请重试'))
    }
  })
}
export async function locate(): Promise<ActualLocation> {
  const position = window.PanguaiNative
    ? await nativeRequest<ActualLocation>('requestLocation')
    : await new Promise<ActualLocation>((resolve, reject) => {
        if (!navigator.geolocation) {
          reject(new Error('当前环境无法定位，请使用安装版'))
          return
        }
        navigator.geolocation.getCurrentPosition(
          (p) =>
            resolve({
              latitude: p.coords.latitude,
              longitude: p.coords.longitude,
              accuracy: p.coords.accuracy,
              timestamp: p.timestamp,
              coordinate_system: 'wgs84',
            }),
          () => reject(new Error('未取得实际位置，请允许精确定位后重试')),
          { enableHighAccuracy: true, timeout: 25000, maximumAge: 0 },
        )
      })
  const age = Date.now() - position.timestamp
  if (
    !Number.isFinite(position.latitude) ||
    !Number.isFinite(position.longitude) ||
    Math.abs(position.latitude) > 90 ||
    Math.abs(position.longitude) > 180 ||
    (position.latitude === 0 && position.longitude === 0) ||
    !Number.isFinite(position.accuracy) ||
    position.accuracy > 200 ||
    position.accuracy < 0 ||
    age < -5000 ||
    age > 60000 ||
    position.coordinate_system !== 'wgs84'
  )
    throw new Error('定位精度不足或位置已过期，请到信号较好的位置重试')
  return position
}
export const scanCode = () => nativeRequest<{ text: string }>('scanCode')

export async function saveAboutImage(image: keyof typeof aboutImages): Promise<void> {
  if (window.PanguaiNative) {
    await nativeRequest('saveImage', image)
    return
  }
  const item = aboutImages[image]
  const response = await fetch(item.url)
  if (!response.ok) throw new Error('图片加载失败，请重试')
  const blob = await response.blob()
  if (blob.type !== 'image/jpeg') throw new Error('图片加载失败，请重试')
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = item.filename
  document.body.append(link)
  link.click()
  link.remove()
  setTimeout(() => URL.revokeObjectURL(url), 60000)
}

export async function copyGroupNumber(): Promise<void> {
  if (window.PanguaiNative?.copyGroupNumber) {
    await nativeRequest('copyGroupNumber')
    return
  }
  try {
    await navigator.clipboard.writeText(groupNumber)
  } catch {
    const input = document.createElement('textarea')
    input.value = groupNumber
    input.setAttribute('readonly', '')
    input.style.cssText = 'position:fixed;left:-9999px;top:0'
    const focused = document.activeElement as HTMLElement | null
    document.body.append(input)
    input.select()
    try {
      if (!document.execCommand('copy')) throw new Error('复制失败')
    } finally {
      input.remove()
      focused?.focus({ preventScroll: true })
    }
  }
}
