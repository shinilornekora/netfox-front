# Фронтенд Netfox: сборка webpack → статика в nginx, который же проксирует /api в gateway.
FROM node:22-alpine AS build
WORKDIR /app
# В образе нет .git — git-хуки husky ставить некуда.
ENV HUSKY=0
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
