import type { ReactNode } from 'react'
import type { Expense } from '../api/types'
import { categoryLabel, formatAmount, formatDate } from '../api/types'

export interface ExpenseTableProps {
  readonly expenses: readonly Expense[]
  readonly onEdit: (expense: Expense) => void
  readonly onDelete: (expense: Expense) => void
  readonly disabled?: boolean
}

export function ExpenseTable({
  expenses,
  onEdit,
  onDelete,
  disabled = false,
}: ExpenseTableProps): ReactNode {
  return (
    <div className="expense-table-wrapper">
      <table className="expense-table" aria-label="Список трат">
        <thead>
          <tr>
            <th scope="col">Дата</th>
            <th scope="col">Категория</th>
            <th scope="col">Комментарий</th>
            <th scope="col" className="expense-table__user">ID</th>
            <th scope="col" className="expense-table__amount">
              Сумма
            </th>
            <th scope="col" className="expense-table__actions">
              <span className="visually-hidden">Действия</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {expenses.map((expense) => (
            <tr key={expense.id}>
              <td data-label="Дата">{formatDate(expense.spent_at)}</td>
              <td data-label="Категория">
                <span className="category-badge">{categoryLabel(expense.category)}</span>
              </td>
              <td data-label="Комментарий" className="expense-table__comment">
                {expense.comment || <span className="expense-table__muted">—</span>}
              </td>
              <td data-label="ID" className="expense-table__user">
                {expense.user_id}
              </td>
              <td data-label="Сумма" className="expense-table__amount">
                {formatAmount(expense.amount)}
              </td>
              <td className="expense-table__actions">
                <button
                  type="button"
                  className="button button--ghost"
                  onClick={() => onEdit(expense)}
                  disabled={disabled}
                  aria-label={`Редактировать трату №${expense.id}`}
                >
                  Изменить
                </button>
                <button
                  type="button"
                  className="button button--danger"
                  onClick={() => onDelete(expense)}
                  disabled={disabled}
                  aria-label={`Удалить трату №${expense.id}`}
                >
                  Удалить
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
