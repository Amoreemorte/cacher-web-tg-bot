import type {
  ApiErrorBody,
  Expense,
  ExpenseFilters,
  ExpensePayload,
} from './types'

/** Ошибка, пришедшая от REST API бэкенда. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly details?: string,
    readonly raw?: ApiErrorBody,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

/** Базовый URL REST API. Пустая строка = same-origin (через Vite proxy). */
const BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/+$/, '')

/** Максимальный размер страницы, совпадает с limit бэкенда. */
export const MAX_LIMIT = 500
/** Размер страницы по умолчанию. */
export const DEFAULT_LIMIT = 50

async function sendJson<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const url = BASE_URL + path
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...(init.headers as Record<string, string> | undefined),
  }
  if (init.body !== undefined && init.body !== null) {
    headers['Content-Type'] = 'application/json'
  }

  let response: Response
  try {
    response = await fetch(url, { ...init, headers })
  } catch (err) {
    throw new ApiError(
      err instanceof Error ? err.message : 'Ошибка сети',
      0,
    )
  }

  // DELETE /api/v1/expenses/{id} возвращает 204 без тела.
  if (response.status === 204) {
    return undefined as T
  }

  const text = await response.text()
  const contentType = response.headers.get('Content-Type') ?? ''
  const isJson = contentType.includes('application/json')
  const data: unknown = isJson && text.length > 0 ? safeJsonParse(text) : text

  if (!response.ok) {
    if (isJson && isApiErrorBody(data)) {
      throw new ApiError(
        data.details ? `${data.error}: ${data.details}` : data.error,
        data.code || response.status,
        data.details,
        data,
      )
    }
    throw new ApiError(
      typeof data === 'string' && data.length > 0 ? data : response.statusText,
      response.status,
    )
  }

  return data as T
}

function safeJsonParse(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

function isApiErrorBody(value: unknown): value is ApiErrorBody {
  return (
    typeof value === 'object' &&
    value !== null &&
    'error' in value &&
    typeof (value as ApiErrorBody).error === 'string' &&
    'code' in value &&
    typeof (value as ApiErrorBody).code === 'number'
  )
}

function buildQueryString(filters: ExpenseFilters): string {
  const params = new URLSearchParams()
  if (filters.user_id !== undefined) params.set('user_id', String(filters.user_id))
  if (filters.category !== undefined) params.set('category', filters.category)
  if (filters.from !== undefined) params.set('from', filters.from)
  if (filters.to !== undefined) params.set('to', filters.to)
  if (filters.limit !== undefined) params.set('limit', String(filters.limit))
  if (filters.offset !== undefined) params.set('offset', String(filters.offset))
  const qs = params.toString()
  return qs.length > 0 ? `?${qs}` : ''
}

/** GET /api/v1/expenses — список трат с фильтрами и пагинацией. */
export function listExpenses(filters: ExpenseFilters = {}): Promise<Expense[]> {
  const withDefaults: ExpenseFilters = {
    limit: DEFAULT_LIMIT,
    ...filters,
  }
  return sendJson<Expense[]>(`/api/v1/expenses${buildQueryString(withDefaults)}`)
}

/** GET /api/v1/expenses/{id} — одна трата. */
export function getExpense(id: number): Promise<Expense> {
  return sendJson<Expense>(`/api/v1/expenses/${id}`)
}

/** POST /api/v1/expenses — создание траты. Возвращает созданную запись (201). */
export function createExpense(payload: ExpensePayload): Promise<Expense> {
  return sendJson<Expense>('/api/v1/expenses', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

/** PUT /api/v1/expenses/{id} — полная замена траты. */
export function replaceExpense(id: number, payload: ExpensePayload): Promise<Expense> {
  return sendJson<Expense>(`/api/v1/expenses/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

/** DELETE /api/v1/expenses/{id} — удаление траты. */
export function deleteExpense(id: number): Promise<void> {
  return sendJson<void>(`/api/v1/expenses/${id}`, { method: 'DELETE' })
}

/** Проверка живости бэкенда — используется в healthcheck индикаторе. */
export async function checkHealth(): Promise<boolean> {
  try {
    const data = await sendJson<{ status?: string }>('/healthz')
    return data?.status === 'ok'
  } catch {
    return false
  }
}
