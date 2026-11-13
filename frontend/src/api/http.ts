export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}
export async function request<T>(path: string, body?: unknown, method?: string): Promise<T> {
  const response = await fetch(`/api${path}`, {
    method: method ?? (body === undefined ? 'GET' : 'POST'),
    headers: { 'Content-Type': 'application/json', 'X-Panguai-Client': 'web' },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(30000),
  })
  const result = response.status === 204 ? undefined : await response.json()
  if (!response.ok)
    throw new ApiError(result?.message ?? '连接失败，请稍后重试', response.status, result?.code)
  return result as T
}
