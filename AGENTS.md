# 🤖 AGENTS.md — Agentes e Papéis no Projeto `goCep-k8s`

Projeto: **goCep-k8s**
Linguagem: **Go (Golang)**
Banco: **PostgreSQL (cache de CEP)**
Registry: **Docker Hub — victordias21/gocep**
Ambientes:
- 🧪 Local (Kind)
- ☁️ Produção (EKS)

## 👷 Agente de Desenvolvimento
Responsável por escrever código limpo e comentado em Go, garantindo que `go test ./...` passa antes do build.

### Comandos úteis
```bash
docker run --name pg-cep -e POSTGRES_PASSWORD=1234 -e POSTGRES_DB=cepdb -p 5432:5432 -d postgres:15
export DB_DSN="postgres://postgres:1234@localhost:5432/cepdb?sslmode=disable"
go run ./cmd/api
go test ./... -v
```

## 🧪 Agente de Build & CI/CD
Executa pipeline GitHub Actions: testes → build → push para Docker Hub.

Secrets necessários:
- `DOCKERHUB_USERNAME`
- `DOCKERHUB_TOKEN`

## ☸️ Agente de Deploy
Aplica manifests em `k8s/`:
```bash
kind create cluster --name gocep
kubectl apply -f k8s/
kubectl get pods -n gocep
```

## 🔁 Fluxo CI/CD
```mermaid
graph TD
A[Commit main] --> B[GitHub Actions: Testes]
B --> C[GitHub Actions: Build Docker]
C --> D[GitHub Actions: Push Docker Hub victordias21/gocep]
D --> E[GitHub Actions: Kubernetes Deploy Kind/EKS]
```
