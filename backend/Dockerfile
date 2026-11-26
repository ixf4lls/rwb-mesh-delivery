FROM golang:1.25-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /mesh ./cmd/mesh

FROM alpine:3.23
RUN apk add --no-cache tzdata
WORKDIR /app/backend
COPY --from=build /mesh /usr/local/bin/mesh
COPY docs/openapi/ /app/docs/openapi/
COPY backend/testdata/demo/ /app/backend/testdata/demo/
ENTRYPOINT ["mesh"]
COPY datasets/ /app/datasets/
