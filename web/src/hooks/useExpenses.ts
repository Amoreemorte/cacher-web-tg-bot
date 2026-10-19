import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ApiError,
  DEFAULT_LIMIT,
  deleteExpense,
  listExpenses,
  MAX_LIMIT,
} from '../api/client'
import type {
  Category,
  Expense,
  ExpenseFilters,
  PeriodKey,
} from '../api/types'
import { periodToRange } from '../api/types'

export interface UseExpensesResult {
  /** Текущая страница трат. */
  readonly expenses: readonly Expense[]
  /** Индикатор загрузки. */
  readonly loading: boolean
  /** Последняя ошибка, если запрос завершился неудачно. */
  readonly error: Error | null
  /** Активные фильтры. */
  readonly filters: FiltersState
  /** Номер текущей страницы (начиная с 1). */
  readonly page: number
  /** Есть ли ещё страницы после текущей. */
  readonly hasMore: boolean
  /** Обновить фильтры (сбрасывает страницу на 1). */
  setFilters: (next: FiltersState) => void
  /** Сбросить все фильтры. */
  resetFilters: () => void
  /** Перейти на указанную страницу. */
  setPage: (page: number) => void
  /** Повторить последний запрос (например, после ошибки или изменения данных). */
  refresh: () => void
  /** Удалить трату и обновить список. */
  remove: (id: number) => Promise<void>
  /** Сообщение о результате последней операции (например, после удаления). */
  readonly flash: FlashMessage | null
  /** Сбросить сообщение. */
  clearFlash: () => void
}

export interface FiltersState {
  readonly userId: string
  readonly category: Category | ''
  readonly period: PeriodKey
}

export interface FlashMessage {
  readonly kind: 'success' | 'error'
  readonly text: string
}

const DEFAULT_FILTERS: FiltersState = {
  userId: '',
  category: '',
  period: 'all',
}

export function useExpenses(): UseExpensesResult {
  const [expenses, setExpenses] = useState<Expense[]>([])
  const [loading, setLoading] = useState<boolean>(false)
  const [error, setError] = useState<Error | null>(null)
  const [filters, setFiltersState] = useState<FiltersState>(DEFAULT_FILTERS)
  const [page, setPageState] = useState<number>(1)
  const [hasMore, setHasMore] = useState<boolean>(false)
  const [flash, setFlash] = useState<FlashMessage | null>(null)

  const buildApiFilters = useCallback(
    (state: FiltersState, currentPage: number): ExpenseFilters => {
      const { from, to } = periodToRange(state.period)
      const result: ExpenseFilters = {
        limit: DEFAULT_LIMIT,
        offset: Math.max(0, (currentPage - 1) * DEFAULT_LIMIT),
      }
      if (state.userId.trim() !== '') {
        const parsed = Number(state.userId.trim())
        if (Number.isInteger(parsed) && parsed > 0) {
          return { ...result, user_id: parsed, category: state.category || undefined, from, to }
        }
      }
      return { ...result, category: state.category || undefined, from, to }
    },
    [],
  )

  const load = useCallback(
    async (state: FiltersState, currentPage: number): Promise<void> => {
      setLoading(true)
      setError(null)
      try {
        const apiFilters = buildApiFilters(state, currentPage)
        const data = await listExpenses(apiFilters)
        // Бэкенд возвращает ровно limit записей, если есть ещё страница —
        // размер ответа равен MAX_LIMIT (или DEFAULT_LIMIT, если он меньше).
        const requestedLimit = apiFilters.limit ?? DEFAULT_LIMIT
        setHasMore(data.length >= Math.min(requestedLimit, MAX_LIMIT))
        setExpenses(data)
      } catch (err) {
        setError(err instanceof Error ? err : new Error(String(err)))
        setExpenses([])
        setHasMore(false)
      } finally {
        setLoading(false)
      }
    },
    [buildApiFilters],
  )

  useEffect(() => {
    void load(filters, page)
  }, [filters, page, load])

  const setFilters = useCallback((next: FiltersState): void => {
    setFiltersState(next)
    setPageState(1)
  }, [])

  const resetFilters = useCallback((): void => {
    setFiltersState(DEFAULT_FILTERS)
    setPageState(1)
  }, [])

  const setPage = useCallback((nextPage: number): void => {
    if (!Number.isInteger(nextPage) || nextPage < 1) return
    setPageState(nextPage)
  }, [])

  const refresh = useCallback((): void => {
    void load(filters, page)
  }, [load, filters, page])

  const remove = useCallback(
    async (id: number): Promise<void> => {
      try {
        await deleteExpense(id)
        setFlash({ kind: 'success', text: 'Трата удалена' })
        // Если на странице осталась одна запись и это не первая страница —
        // вернуться на предыдущую.
        await load(filters, page)
        setExpenses((prev) => {
          if (prev.length === 0 && page > 1) {
            setPageState(page - 1)
          }
          return prev
        })
      } catch (err) {
        const message =
          err instanceof ApiError
            ? err.details
              ? `${err.message}`
              : 'Не удалось удалить трату'
            : 'Не удалось удалить трату'
        setFlash({ kind: 'error', text: message })
        throw err instanceof Error ? err : new Error(String(err))
      }
    },
    [filters, page, load],
  )

  const clearFlash = useCallback((): void => {
    setFlash(null)
  }, [])

  return useMemo(
    () => ({
      expenses,
      loading,
      error,
      filters,
      page,
      hasMore,
      setFilters,
      resetFilters,
      setPage,
      refresh,
      remove,
      flash,
      clearFlash,
    }),
    [
      expenses,
      loading,
      error,
      filters,
      page,
      hasMore,
      setFilters,
      resetFilters,
      setPage,
      refresh,
      remove,
      flash,
      clearFlash,
    ],
  )
}
