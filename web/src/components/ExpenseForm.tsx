import {
  type ChangeEvent,
  type FormEvent,
  type ReactNode,
  useEffect,
  useState,
} from 'react'
import type { Category, Expense, ExpensePayload } from '../api/types'
import { CATEGORIES, categoryLabel, toDateInput } from '../api/types'

export interface ExpenseFormProps {
  readonly initial?: Expense
  readonly submitLabel?: string
  readonly busy?: boolean
  readonly errorMessage?: string | null
  readonly onSubmit: (payload: ExpensePayload) => void | Promise<void>
  readonly onCancel: () => void
}

interface FormState {
  userId: string
  amount: string
  category: Category | ''
  spentAt: string
  comment: string
}

interface FieldErrors {
  userId?: string
  amount?: string
  category?: string
  spentAt?: string
}

function expenseToState(expense?: Expense): FormState {
  const today = toDateInput(new Date())
  if (!expense) {
    return {
      userId: '',
      amount: '',
      category: '',
      spentAt: today,
      comment: '',
    }
  }
  const date = new Date(expense.spent_at)
  const spentAt = Number.isNaN(date.getTime()) ? today : toDateInput(date)
  return {
    userId: String(expense.user_id),
    amount: String(expense.amount),
    category: expense.category,
    spentAt,
    comment: expense.comment,
  }
}

function validate(state: FormState): { payload: ExpensePayload | null; errors: FieldErrors } {
  const errors: FieldErrors = {}
  const userId = Number(state.userId.trim())
  if (!Number.isInteger(userId) || userId <= 0) {
    errors.userId = 'Введите положительный Telegram ID'
  }
  const amount = Number(state.amount)
  if (!Number.isFinite(amount) || amount <= 0) {
    errors.amount = 'Сумма должна быть больше нуля'
  } else if (amount > 1e12) {
    errors.amount = 'Сумма слишком велика'
  }
  if (!state.category) {
    errors.category = 'Выберите категорию'
  }
  if (!state.spentAt) {
    errors.spentAt = 'Укажите дату'
  }
  if (Object.keys(errors).length > 0) {
    return { payload: null, errors }
  }
  const payload: ExpensePayload = {
    user_id: userId,
    amount,
    category: state.category as Category,
    spent_at: state.spentAt,
    comment: state.comment.trim(),
  }
  return { payload, errors }
}

interface FieldProps {
  readonly label: string
  readonly htmlFor: string
  readonly error?: string
  readonly hint?: string
  readonly className?: string
  readonly children: ReactNode
}

function Field({ label, htmlFor, error, hint, className, children }: FieldProps): ReactNode {
  return (
    <label
      className={`expense-form__field${error ? ' expense-form__field--invalid' : ''}${
        className ? ` ${className}` : ''
      }`}
      htmlFor={htmlFor}
    >
      <span className="expense-form__label">{label}</span>
      {children}
      {hint && !error ? <span className="expense-form__hint">{hint}</span> : null}
      {error ? <span className="expense-form__error">{error}</span> : null}
    </label>
  )
}

export function ExpenseForm({
  initial,
  submitLabel,
  busy = false,
  errorMessage = null,
  onSubmit,
  onCancel,
}: ExpenseFormProps): ReactNode {
  const [state, setState] = useState<FormState>(() => expenseToState(initial))
  const [errors, setErrors] = useState<FieldErrors>({})

  const editing = initial !== undefined
  const title = editing ? 'Редактировать трату' : 'Добавить трату'
  const buttonLabel = submitLabel ?? (editing ? 'Сохранить' : 'Добавить')

  useEffect(() => {
    setState(expenseToState(initial))
    setErrors({})
  }, [initial])

  const handleChange = (
    event: ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>,
  ): void => {
    const { name, value } = event.target
    setState((prev) => ({ ...prev, [name]: value }))
    setErrors((prev) => ({ ...prev, [name]: undefined }))
  }

  const handleSubmit = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
    event.preventDefault()
    const result = validate(state)
    if (!result.payload) {
      setErrors(result.errors)
      return
    }
    await onSubmit(result.payload)
  }

  return (
    <section className="expense-form" aria-labelledby="expense-form-title">
      <header className="expense-form__header">
        <h2 id="expense-form-title">{title}</h2>
        <button
          type="button"
          className="button button--ghost"
          onClick={onCancel}
          disabled={busy}
          aria-label="Закрыть форму"
        >
          ✕
        </button>
      </header>

      {errorMessage ? (
        <div className="status-view status-view--error" role="alert">
          <div className="status-view__body">{errorMessage}</div>
        </div>
      ) : null}

      <form className="expense-form__body" onSubmit={(e) => void handleSubmit(e)} noValidate>
        <div className="expense-form__grid">
          <Field
            label="Telegram ID"
            htmlFor="userId"
            error={errors.userId}
            hint="Числовой ID члена семьи"
          >
            <input
              id="userId"
              name="userId"
              type="number"
              inputMode="numeric"
              min={1}
              step={1}
              value={state.userId}
              onChange={handleChange}
              autoComplete="off"
              disabled={busy}
              aria-invalid={errors.userId ? true : undefined}
              required
            />
          </Field>

          <Field label="Сумма" htmlFor="amount" error={errors.amount}>
            <input
              id="amount"
              name="amount"
              type="number"
              inputMode="decimal"
              min={0.01}
              step="0.01"
              value={state.amount}
              onChange={handleChange}
              autoComplete="off"
              disabled={busy}
              aria-invalid={errors.amount ? true : undefined}
              required
            />
          </Field>

          <Field label="Категория" htmlFor="category" error={errors.category}>
            <select
              id="category"
              name="category"
              value={state.category}
              onChange={handleChange}
              disabled={busy}
              aria-invalid={errors.category ? true : undefined}
              required
            >
              <option value="">— выберите —</option>
              {CATEGORIES.map((category) => (
                <option key={category} value={category}>
                  {categoryLabel(category)}
                </option>
              ))}
            </select>
          </Field>

          <Field label="Дата" htmlFor="spentAt" error={errors.spentAt}>
            <input
              id="spentAt"
              name="spentAt"
              type="date"
              value={state.spentAt}
              onChange={handleChange}
              disabled={busy}
              aria-invalid={errors.spentAt ? true : undefined}
              required
            />
          </Field>

          <Field
            label="Комментарий"
            htmlFor="comment"
            className="expense-form__full"
            hint="Необязательно, до 500 символов"
          >
            <textarea
              id="comment"
              name="comment"
              rows={2}
              maxLength={500}
              value={state.comment}
              onChange={handleChange}
              disabled={busy}
              autoComplete="off"
            />
          </Field>
        </div>

        <div className="expense-form__actions">
          <button
            type="button"
            className="button button--ghost"
            onClick={onCancel}
            disabled={busy}
          >
            Отмена
          </button>
          <button type="submit" className="button button--primary" disabled={busy}>
            {busy ? 'Подождите…' : buttonLabel}
          </button>
        </div>
      </form>
    </section>
  )
}

