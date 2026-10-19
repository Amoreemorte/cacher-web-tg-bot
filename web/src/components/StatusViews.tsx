import type { ReactNode } from 'react'

interface LoadingProps {
  readonly message?: string
}

export function Loading({ message = 'Загрузка…' }: LoadingProps): ReactNode {
  return (
    <div className="status-view status-view--loading" role="status" aria-live="polite">
      <span className="status-view__spinner" aria-hidden="true" />
      <span>{message}</span>
    </div>
  )
}

interface ErrorBannerProps {
  readonly title?: string
  readonly message: string
  readonly details?: string
  readonly onRetry?: () => void
}

export function ErrorBanner({
  title = 'Ошибка',
  message,
  details,
  onRetry,
}: ErrorBannerProps): ReactNode {
  return (
    <div className="status-view status-view--error" role="alert">
      <div className="status-view__body">
        <strong>{title}:</strong> {message}
        {details ? <div className="status-view__details">{details}</div> : null}
      </div>
      {onRetry ? (
        <button type="button" className="button button--primary" onClick={onRetry}>
          Повторить
        </button>
      ) : null}
    </div>
  )
}

interface EmptyStateProps {
  readonly title?: string
  readonly message: string
  readonly action?: ReactNode
}

export function EmptyState({
  title = 'Пусто',
  message,
  action,
}: EmptyStateProps): ReactNode {
  return (
    <div className="status-view status-view--empty">
      <h2>{title}</h2>
      <p>{message}</p>
      {action ? <div className="status-view__actions">{action}</div> : null}
    </div>
  )
}

interface FlashMessageProps {
  readonly kind: 'success' | 'error'
  readonly message: string
  readonly onClose: () => void
}

export function Flash({ kind, message, onClose }: FlashMessageProps): ReactNode {
  return (
    <div className={`status-view status-view--flash status-view--${kind}`} role="status">
      <span>{message}</span>
      <button type="button" className="button button--ghost" onClick={onClose} aria-label="Закрыть">
        ✕
      </button>
    </div>
  )
}
