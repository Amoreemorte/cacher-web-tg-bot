import { useEffect, useRef, type ReactNode } from 'react'

export interface ConfirmDialogProps {
  readonly open: boolean
  readonly title: string
  readonly description: ReactNode
  readonly confirmLabel?: string
  readonly cancelLabel?: string
  readonly destructive?: boolean
  readonly busy?: boolean
  readonly onConfirm: () => void
  readonly onCancel: () => void
}

/**
 * Модальный диалог подтверждения. Используется для подтверждения удаления
 * и других разрушительных действий. Фокус автоматически ставится на
 * кнопку «Отмена», чтобы избежать случайного подтверждения клавишей Enter.
 */
export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = 'Подтвердить',
  cancelLabel = 'Отмена',
  destructive = false,
  busy = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps): ReactNode {
  const cancelRef = useRef<HTMLButtonElement>(null)

  // Закрываем по Escape и фокусируем кнопку отмены при открытии.
  useEffect(() => {
    if (!open) return
    cancelRef.current?.focus()
    const handler = (event: KeyboardEvent): void => {
      if (event.key === 'Escape' && !busy) {
        event.preventDefault()
        onCancel()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [open, busy, onCancel])

  if (!open) return null

  return (
    <div
      className="modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="confirm-dialog-title"
      onClick={(event) => {
        if (event.target === event.currentTarget && !busy) onCancel()
      }}
    >
      <div className="modal__panel">
        <h2 id="confirm-dialog-title" className="modal__title">
          {title}
        </h2>
        <div className="modal__body">{description}</div>
        <div className="modal__actions">
          <button
            type="button"
            className="button button--ghost"
            onClick={onCancel}
            disabled={busy}
            ref={cancelRef}
          >
            {cancelLabel}
          </button>
          <button
            type="button"
            className={`button ${destructive ? 'button--danger' : 'button--primary'}`}
            onClick={onConfirm}
            disabled={busy}
          >
            {busy ? 'Подождите…' : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
