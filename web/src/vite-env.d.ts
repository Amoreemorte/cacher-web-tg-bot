/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Базовый URL REST API. В dev-режиме пусто — работает через Vite proxy. */
  readonly VITE_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
