# Stage 1: Build the frontend React app
FROM node:20-alpine AS build-frontend
WORKDIR /src
COPY web/package.json web/package-lock.json ./web/
RUN cd web && npm ci
COPY web/ ./web/
RUN cd web && npm run build

# Stage 2: Build the Go application
FROM golang:1.25-alpine AS build-backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o axis ./cmd/axis

# Stage 3: Final minimal image
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
# Copy the compiled binary
COPY --from=build-backend /app/axis /app/axis
# Copy the built frontend into web/dist
COPY --from=build-frontend /src/web/dist /app/web/dist

# Expose port (Cloud Run sets PORT env var)
ENV PORT=8080
EXPOSE 8080

CMD ["/app/axis"]
