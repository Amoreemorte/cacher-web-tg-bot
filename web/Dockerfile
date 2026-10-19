# ============================================================
# Этап 1: сборка фронтенда (React + TypeScript + Vite)
# ============================================================
FROM node:24-alpine AS build

WORKDIR /app

# Воспроизводимая установка зависимостей по lock-файлу.
COPY package.json package-lock.json ./
RUN npm ci

COPY . .

# Проверка типов и production-сборка (скрипт "build": "tsc -b && vite build").
# Результат — каталог dist/ со статикой.
RUN npm run build

# ============================================================
# Этап 2: раздача статики через Nginx (production web-сервер)
# ============================================================
FROM nginx:1.29-alpine

COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

# Nginx запускается от пользователя nginx (не root) и не демоном.
CMD ["nginx", "-g", "daemon off;"]
