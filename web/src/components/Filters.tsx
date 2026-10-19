import type { ChangeEvent, FormEvent, ReactNode } from 'react'
import type { PeriodKey } from '../api/types'
import { CATEGORIES, categoryLabel } from '../api/types'
import type { FiltersState } from '../hooks/useExpenses'

export interface FiltersProps {
  readonly value: FiltersState
  readonly onChange: (next: FiltersState) => void
  readonly onReset: () => void
  readonly disabled?: boolean
}

const PERIOD_OPTIONS: ReadonlyArray<{ value: PeriodKey; label: string }> = [
  { value: 'all', label: 'Всё время' },
  { value: 'today', label: 'Сегодня' },
  { value: 'week', label: 'Неделя' },
  { value: 'month', label: 'Месяц' },
]

export function Filters({ value, onChange, onReset, disabled = false }: FiltersProps): ReactNode {
  const handleChange =
    (key: keyof FiltersState) =>
    (event: ChangeEvent<HTMLInputElement | HTMLSelectElement>): void => {
      onChange({ ...value, [key]: event.target.value })
    }

  const handleSubmit = (event: FormEvent<HTMLFormElement>): void => {
    event.preventDefault()
  }

  const isDirty =
    value.userId !== '' || value.category !== '' || value.period !== 'all'

  return (
    <section className="filters" aria-labelledby="filters-title">
      <header className="filters__header">
        <h2 id="filters-title" className="filters__title">
          Фильтры
        </h2>
        {isDirty ? (
          <button
            type="button"
            className="button button--ghost"
            onClick={onReset}
            disabled={disabled}
          >
            Сбросить
          </button>
        ) : null}
      </header>

      <form className="filters__grid" onSubmit={handleSubmit}>
        <label className="filters__field" htmlFor="filter-user">
          <span className="filters__label">Член семьи</span>
          <input
            id="filter-user"
            type="number"
            inputMode="numeric"
            min={1}
            step={1}
            placeholder="Telegram ID"
            value={value.userId}
            onChange={handleChange('userId')}
            disabled={disabled}
            autoComplete="off"
          />
        </label>

        <label className="filters__field" htmlFor="filter-category">
          <span className="filters__label">Категория</span>
          <select
            id="filter-category"
            value={value.category}
            onChange={handleChange('category')}
            disabled={disabled}
          >
            <option value="">Все</option>
            {CATEGORIES.map((category) => (
              <option key={category} value={category}>
                {categoryLabel(category)}
              </option>
            ))}
          </select>
        </label>

        <label className="filters__field" htmlFor="filter-period">
          <span className="filters__label">Период</span>
          <select
            id="filter-period"
            value={value.period}
            onChange={handleChange('period')}
            disabled={disabled}
          >
            {PERIOD_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
      </form>
    </section>
  )
}
