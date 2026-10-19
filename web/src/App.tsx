import { useCallback, useState, type ReactNode } from 'react'
import { ApiError, createExpense, replaceExpense } from './api/client'
import type { Expense, ExpensePayload } from './api/types'
import { ConfirmDialog } from './components/ConfirmDialog'
import { EmptyState, ErrorBanner, Flash, Loading } from './components/StatusViews'
import { ExpenseForm } from './components/ExpenseForm'
import { ExpenseTable } from './components/ExpenseTable'
import { Filters } from './components/Filters'
import { useExpenses, type FiltersState } from './hooks/useExpenses'

import './components/Filters.css'
import './components/ExpenseForm.css'
import './components/ExpenseTable.css'
import './components/StatusViews.css'
import './components/ConfirmDialog.css'

type FormMode =
  | { kind: 'closed' }
  | { kind: 'create' }
  | { kind: 'edit'; expense: Expense }

interface DeleteTarget {
  readonly expense: Expense
  readonly busy: boolean
}

function formatAmount(amount: number): string {
  return new Intl.NumberFormat('ru-RU', {
    style: 'currency',
    currency: 'RUB',
    minimumFractionDigits: 2,
  }).format(amount)
}

function describeError(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    if (error.details) return error.message
    if (error.status === 429) return 'Слишком много запросов. Подождите немного.'
    if (error.status === 0) return 'Не удалось связаться с сервером. Проверьте, запущен ли бэкенд.'
    return error.message || fallback
  }
  if (error instanceof Error) return error.message || fallback
  return fallback
}

interface PaginationProps {
  readonly page: number
  readonly hasMore: boolean
  readonly disabled: boolean
  readonly onChange: (page: number) => void
}

function Pagination({ page, hasMore, disabled, onChange }: PaginationProps): ReactNode {
  if (page <= 1 && !hasMore) return null
  return (
    <nav className="pagination" aria-label="Постраничная навигация">
      <button
        type="button"
        className="button button--ghost"
        disabled={disabled || page <= 1}
        onClick={() => onChange(page - 1)}
      >
        ← Назад
      </button>
      <span className="pagination__info">Страница {page}</span>
      <button
        type="button"
        className="button button--ghost"
        disabled={disabled || !hasMore}
        onClick={() => onChange(page + 1)}
      >
        Вперёд →
      </button>
    </nav>
  )
}

export function App(): ReactNode {
  const state = useExpenses()
  const [formMode, setFormMode] = useState<FormMode>({ kind: 'closed' })
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null)
  const [formError, setFormError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const submitForm = useCallback(
    async (payload: ExpensePayload): Promise<void> => {
      setSubmitting(true)
      setFormError(null)
      try {
        if (formMode.kind === 'edit') {
          await replaceExpense(formMode.expense.id, payload)
        } else {
          await createExpense(payload)
        }
        setFormMode({ kind: 'closed' })
        state.refresh()
      } catch (err) {
        const message = describeError(err, 'Не удалось сохранить трату')
        setFormError(message)
        throw err
      } finally {
        setSubmitting(false)
      }
    },
    [formMode, state],
  )

  const handleDelete = useCallback(async (): Promise<void> => {
    if (!deleteTarget) return
    setDeleteTarget({ ...deleteTarget, busy: true })
    try {
      await state.remove(deleteTarget.expense.id)
      setDeleteTarget(null)
    } catch (err) {
      setDeleteTarget({ ...deleteTarget, busy: false })
      void err
    }
  }, [deleteTarget, state])

  const handleFiltersChange = useCallback(
    (next: FiltersState): void => {
      state.setFilters(next)
      setFormMode({ kind: 'closed' })
    },
    [state],
  )

  const openCreateForm = useCallback((): void => {
    setFormError(null)
    setFormMode({ kind: 'create' })
  }, [])

  const openEditForm = useCallback((expense: Expense): void => {
    setFormError(null)
    setFormMode({ kind: 'edit', expense })
  }, [])

  const closeForm = useCallback((): void => {
    if (submitting) return
    setFormMode({ kind: 'closed' })
    setFormError(null)
  }, [submitting])

  const requestDelete = useCallback((expense: Expense): void => {
    setDeleteTarget({ expense, busy: false })
  }, [])

  const cancelDelete = useCallback((): void => {
    if (deleteTarget?.busy) return
    setDeleteTarget(null)
  }, [deleteTarget])

  return (
    <div className="app">
      <header className="app__header">
        <div>
          <h1 className="app__title">Учёт семейных расходов</h1>
          <p className="app__subtitle">
            Все траты членов семьи в одном месте. Добавляйте, редактируйте и
            фильтруйте записи без перезагрузки.
          </p>
        </div>
        <button
          type="button"
          className="button button--primary"
          onClick={openCreateForm}
          disabled={formMode.kind !== 'closed' || state.loading}
        >
          + Добавить трату
        </button>
      </header>

      {state.flash ? (
        <Flash
          kind={state.flash.kind}
          message={state.flash.text}
          onClose={state.clearFlash}
        />
      ) : null}

      <Filters
        value={state.filters}
        onChange={handleFiltersChange}
        onReset={state.resetFilters}
        disabled={state.loading}
      />

      <main className="app__content">
        {formMode.kind !== 'closed' ? (
          <ExpenseForm
            initial={formMode.kind === 'edit' ? formMode.expense : undefined}
            onSubmit={submitForm}
            onCancel={closeForm}
            busy={submitting}
            errorMessage={formError}
          />
        ) : null}

        {state.error ? (
          <ErrorBanner
            message={describeError(state.error, 'Не удалось загрузить список трат')}
            onRetry={state.refresh}
          />
        ) : null}

        {state.loading && state.expenses.length === 0 && !state.error ? (
          <Loading message="Загружаем траты…" />
        ) : null}

        {!state.loading && !state.error && state.expenses.length === 0 ? (
          <EmptyState
            title="Трат пока нет"
            message="Добавьте первую трату через кнопку выше или напишите боту в Telegram."
            action={
              <button
                type="button"
                className="button button--primary"
                onClick={openCreateForm}
              >
                Добавить трату
              </button>
            }
          />
        ) : null}

        {state.expenses.length > 0 ? (
          <>
            <ExpenseTable
              expenses={state.expenses}
              onEdit={openEditForm}
              onDelete={requestDelete}
              disabled={state.loading}
            />
            <Pagination
              page={state.page}
              hasMore={state.hasMore}
              disabled={state.loading}
              onChange={state.setPage}
            />
          </>
        ) : null}
      </main>

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Удалить трату?"
        description={
          deleteTarget ? (
            <>
              Трата №{deleteTarget.expense.id} на сумму{' '}
              <strong>{formatAmount(deleteTarget.expense.amount)}</strong>{' '}
              будет удалена без возможности восстановления.
            </>
          ) : null
        }
        confirmLabel="Удалить"
        cancelLabel="Отмена"
        destructive
        busy={deleteTarget?.busy ?? false}
        onConfirm={() => void handleDelete()}
        onCancel={cancelDelete}
      />
    </div>
  )
}

