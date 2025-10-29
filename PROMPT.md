# 🚀 PROMPT.md — Contexto para Codex / Copilot Chat (VS Code)

> Fornece o contexto necessário para que a IA compreenda e auxilie o projeto `goCep-k8s`.

## 📘 Contexto
- Projeto: **goCep-k8s**
- Linguagem: **Go**
- Banco: **PostgreSQL**
- Registry: **Docker Hub — victordias21/gocep**
- Repositório: [github.com/victor-dias21/goCep-k8s](https://github.com/victor-dias21/goCep-k8s)
- Infra: Docker, Kubernetes (Kind local e EKS futuro)

## ⚙️ Variáveis de ambiente
| Variável | Descrição | Exemplo |
|-----------|------------|----------|
| DB_DSN | String de conexão PostgreSQL | postgres://user:pass@host:5432/db?sslmode=disable |
| APP_ENV | Ambiente (`dev`/`prod`) | dev |
| HTTP_ADDR | Endereço HTTP | :8080 |

## 🧩 Comandos Locais
```bash
docker run --name pg-cep -e POSTGRES_PASSWORD=1234 -e POSTGRES_DB=cepdb -p 5432:5432 -d postgres:15
export DB_DSN="postgres://postgres:1234@localhost:5432/cepdb?sslmode=disable"
export APP_ENV=dev
go run ./cmd/api
```

## 🧪 Testes
```bash
go test ./... -v
curl http://localhost:8080/healthz
```

## 🐳 Docker
```bash
docker build -t victordias21/gocep:latest .
docker run -p 8080:8080 -e DB_DSN="postgres://postgres:1234@host.docker.internal:5432/cepdb?sslmode=disable" victordias21/gocep:latest
```

## ☸️ Kubernetes (Kind)
```bash
kind create cluster --name gocep
kubectl apply -f k8s/
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
```
Acesse: http://gocep.localtest.me

## 🧠 Prompts úteis
- "Explique a função main.go e adicione logs estruturados."
- "Adicione endpoint /cep/:cep que cacheia no PostgreSQL."
- "Crie Helm Chart a partir dos manifests k8s/."

## 💬 Prompt de Inicialização
```
Você é um assistente DevOps.  
Projeto: goCep-k8s (Go + PostgreSQL + Docker + Kubernetes).  
Objetivos:
1. Executar e testar localmente (Kind).
2. Garantir pipeline CI/CD funcional.
3. Ajustar manifests para produção (Ingress + EKS).
4. Produzir código comentado e limpo.
Registry: Docker Hub victordias21/gocep.
```
