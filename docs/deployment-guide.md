# Deployment Guide

## Prerequisites

- Docker 24+ and Docker Compose v2
- Kubernetes cluster (for k8s deployment path)
- `kubectl` configured for target cluster

## Docker Deployment

### Local Development (Compose)

Start full stack:

```bash
docker compose up --build
```

Services started:

- `server` on `localhost:8080`
- `postgres` on `localhost:5432`
- `redis` on `localhost:6379`
- `nats` on `localhost:4222`

Stop:

```bash
docker compose down
```

Remove volumes too:

```bash
docker compose down -v
```

### Standalone Docker Image

Build image:

```bash
docker build -t mmorp-server:latest .
```

Run container (example):

```bash
docker run --rm -p 8080:8080 \
  -e JWT_SECRET='replace-with-strong-secret' \
  -e POSTGRES_URL='postgres://postgres:postgres@host.docker.internal:5432/mmorp?sslmode=disable' \
  -e REDIS_ADDR='host.docker.internal:6379' \
  -e NATS_URL='nats://host.docker.internal:4222' \
  -e MIGRATION_DIR='/app/migrations' \
  mmorp-server:latest
```

## Kubernetes Deployment

The repo includes app manifests under `k8s/`.

### 1. Create namespace

```bash
kubectl apply -f k8s/namespace.yaml
```

### 2. Configure secret values

Create `k8s/secret.yaml` from example and set real values:

```bash
cp k8s/secret.example.yaml k8s/secret.yaml
```

Required secret values:

- `JWT_SECRET`
- `POSTGRES_URL`
- `CORS_ORIGIN`

Apply config + secret:

```bash
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/secret.yaml
```

### 3. Deploy app

```bash
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
```

### 4. Verify

```bash
kubectl -n mmorp get pods
kubectl -n mmorp get svc
kubectl -n mmorp logs deploy/mmorp-server
```

## Production Notes

- Current k8s manifests deploy only the `mmorp-server` app.
- Postgres, Redis, and NATS must be provisioned separately (managed services or in-cluster operators/charts).
- `k8s/deployment.yaml` defaults to `image: mmorp-server:latest`; set to your registry image/tag before deploying.
- App runs migrations on startup, so ensure DB user has DDL permissions (or split migration step if needed).
