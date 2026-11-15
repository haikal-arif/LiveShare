# syntax=docker/dockerfile:1

# ---- build: static SPA (adapter-static) ----
FROM node:24-alpine AS build
WORKDIR /src

COPY package.json package-lock.json .npmrc ./
RUN --mount=type=cache,target=/root/.npm npm ci

COPY . .

# Baked into the bundle at build time ($env/static/public). Empty = same
# origin, which is what the gateway provides: it routes /api to the backend.
ARG PUBLIC_API_BASE_URL=""
ENV PUBLIC_API_BASE_URL=$PUBLIC_API_BASE_URL
RUN npm run build

# ---- runtime: nginx serving the static files ----
FROM nginx:1.30.3-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/build /usr/share/nginx/html

EXPOSE 80
