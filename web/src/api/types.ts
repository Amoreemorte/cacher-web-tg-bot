/** Запись о трате в ответе API. */
export interface Expense {
  readonly id: number
  readonly user_id: number
  readonly amount: number
  readonly category: Category
  /** Дата траты в ISO-8601 (RFC3339), например "2026-10-19T00:00:00Z". */
  readonly spent_at: string
  readonly comment: string
  readonly created_at: string
  readonly updated_at: string
}

/** Допустимые категории — соответствуют белому списку бэкенда. */
export const CATEGORIES: ReadonlyArray<Category> = [
  'еда',
  'транспорт',
  'жильё',
  'развлечения',
  'здоровье',
  'покупки',
  'другое',
] as const

export type Category =
  | 'еда'
  | 'транспорт'
  | 'жильё'
  | 'развлечения'
  | 'здоровье'
  | 'покупки'
  | 'другое'

/** Поля, которые клиент отправляет при создании/обновлении траты. */
export interface ExpensePayload {
  readonly user_id: number
  readonly amount: number
  readonly category: Category
  /** Дата траты: ISO-8601 (RFC3339) или "YYYY-MM-DD". */
  readonly spent_at: string
  readonly comment: string
}

/** Фильтры для GET /api/v1/expenses. */
export interface ExpenseFilters {
  readonly user_id?: number
  readonly category?: Category
  /** Начало периода (включительно) в ISO-8601 или YYYY-MM-DD. */
  readonly from?: string
  /** Конец периода (включительно) в ISO-8601 или YYYY-MM-DD. */
  readonly to?: string
  /** Размер страницы: 1..500, по умолчанию 50. */
  readonly limit?: number
  /** Сдвиг от начала выборки (≥ 0). */
  readonly offset?: number
}

/** Ошибка API в едином формате бэкенда. */
export interface ApiErrorBody {
  readonly error: string
  readonly code: number
  readonly details?: string
}

/** Предустановленные периоды фильтрации по дате. */
export type PeriodKey = 'all' | 'today' | 'week' | 'month'

export interface PeriodRange {
  readonly from?: string
  readonly to?: string
}

/** Вычисляет диапазон дат для предустановленного периода. */
export function periodToRange(period: PeriodKey, now: Date = new Date()): PeriodRange {
  if (period === 'all') return {}

  const startOfDay = (d: Date): Date =>
    new Date(d.getFullYear(), d.getMonth(), d.getDate())

  const todayStart = startOfDay(now)
  const todayEnd = new Date(todayStart)
  todayEnd.setDate(todayEnd.getDate() + 1)
  todayEnd.setMilliseconds(-1)

  switch (period) {
    case 'today':
      return { from: toDateInput(todayStart), to: toDateInput(todayStart) }
    case 'week': {
      const weekStart = new Date(todayStart)
      weekStart.setDate(weekStart.getDate() - 7)
      return { from: toDateInput(weekStart), to: toDateInput(todayStart) }
    }
    case 'month': {
      const monthStart = new Date(now.getFullYear(), now.getMonth(), 1)
      return { from: toDateInput(monthStart), to: toDateInput(todayStart) }
    }
  }
}

/** Преобразует дату в формат "YYYY-MM-DD" для `<input type="date">` и API. */
export function toDateInput(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

/** Форматирует ISO-дату в пользовательский вид: "19.10.2026". */
export function formatDate(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  const day = String(date.getUTCDate()).padStart(2, '0')
  const month = String(date.getUTCMonth() + 1).padStart(2, '0')
  const year = date.getUTCFullYear()
  return `${day}.${month}.${year}`
}

/** Форматирует сумму в рубли с копейками: "1 234,50 ₽". */
export function formatAmount(amount: number): string {
  return new Intl.NumberFormat('ru-RU', {
    style: 'currency',
    currency: 'RUB',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(amount)
}

/** Человекочитаемое название категории с заглавной буквы. */
export function categoryLabel(category: Category): string {
  return category.charAt(0).toUpperCase() + category.slice(1)
}
