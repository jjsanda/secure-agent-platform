# syntax=docker/dockerfile:1

# ---- Build stage: compile the SPA with Vite -------------------------------
FROM node:22 AS build
WORKDIR /app

# Install dependencies against the committed lockfile for reproducible builds.
COPY package.json package-lock.json ./
RUN npm ci

# The VITE_* values are baked into the bundle at build time. The defaults match
# the default docker-compose profile; override with --build-arg when needed.
ARG VITE_API_BASE_URL=http://localhost:8080
ARG VITE_OIDC_ISSUER=http://localhost:9000
ARG VITE_OIDC_CLIENT_ID=sap-dashboard
ARG VITE_OIDC_REDIRECT_URI=http://localhost:5173/callback
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL \
    VITE_OIDC_ISSUER=$VITE_OIDC_ISSUER \
    VITE_OIDC_CLIENT_ID=$VITE_OIDC_CLIENT_ID \
    VITE_OIDC_REDIRECT_URI=$VITE_OIDC_REDIRECT_URI

COPY . .
RUN npm run build

# ---- Runtime stage: serve the static bundle with nginx --------------------
FROM nginx:alpine AS runtime

# SPA fallback config so client-side routes resolve on refresh.
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
