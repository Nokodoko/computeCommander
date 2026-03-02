FROM oven/bun:1 AS base
WORKDIR /app

# Install dependencies
FROM base AS install
COPY package.json bun.lock* ./
RUN bun install --frozen-lockfile 2>/dev/null || bun install

# Build stage
FROM base AS build
COPY --from=install /app/node_modules node_modules
COPY . .

# Production
FROM base AS release
COPY --from=build /app/src src
COPY --from=build /app/package.json .
COPY --from=build /app/node_modules node_modules

ENV WS_PORT=8080
EXPOSE 8080

USER bun
ENTRYPOINT ["bun", "run", "src/server.ts"]
