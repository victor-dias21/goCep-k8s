# goCep-k8s

API em Go que consulta o ViaCEP, cacheia resultados em PostgreSQL e é distribuída com Docker e Kubernetes. O pipeline completo (GitHub Actions) cobre testes, build, scan, push e deploy.

## Arquitetura

- **Linguagem:** Go 1.23+ (toolchain 1.24 para dependências)
- **Banco:** PostgreSQL 15 (cache de CEP)
- **Cache:** Tabela `ceps` com JSONB
- **Containers:** Docker multi-stage
- **Orquestração:** Kubernetes/Kind com ingress opcional

## Pré-requisitos

- Go 1.23+ com suporte a `go1.24.9` (gobinaries recentes)
- Docker 24+
- kubectl + kind (para ambiente local)
- make (opcional)

## Configuração local

1. **Clonar o repositório**
   ```bash
   git clone https://github.com/victor-dias21/goCep-k8s.git
   cd goCep-k8s
   ```

2. **Variáveis de ambiente**  
   Copie o `.env` e ajuste conforme necessário:
   ```bash
   cp .env .env.local
   source .env.local
   ```
   Principais variáveis:
   - `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE`
   - `DB_DSN` (opcional; se vazio, será montado a partir das variáveis acima)
   - `HTTP_ADDR`, `CACHE_TTL`, `HTTP_CLIENT_TIMEOUT`

3. **Banco local (Docker)**
   ```bash
   docker run --rm --name pg-cep \
     -e POSTGRES_PASSWORD=1234 \
     -e POSTGRES_DB=cepdb \
     -p 5432:5432 \
     -d postgres:15
   ```

4. **Executar testes**
   ```bash
   go test ./...
   ```

5. **Rodar a API**
   ```bash
   go run ./cmd/api
   ```
   Endpoints:
   - `GET http://127.0.0.1:8080/healthz`
   - `GET http://127.0.0.1:8080/cep/01001000`

6. **Build do binário**
   ```bash
   go build -o goCep ./cmd/api
   ./goCep
   ```

7. **Imagem Docker**
   ```bash
   docker build -t victordias21/gocep:latest .
   docker run --rm -p 8080:8080 \
     -e DB_DSN="postgres://postgres:1234@host.docker.internal:5432/cepdb?sslmode=disable" \
     victordias21/gocep:latest
   ```

## Kubernetes (Kind)

1. **Criar cluster**
   ```bash
   kind create cluster --name gocep
   ```

2. **Namespace e secrets**
   ```bash
   kubectl create namespace gocep
   kubectl create secret generic gocep-secrets \
     --namespace gocep \
     --from-literal=POSTGRES_USER=postgres \
     --from-literal=POSTGRES_PASSWORD=1234 \
     --from-literal=POSTGRES_DB=cepdb \
     --from-literal=DB_DSN="postgres://postgres:1234@gocep-postgres.gocep.svc.cluster.local:5432/cepdb?sslmode=disable"
   ```
   > Em `k8s/secret.yaml`, substitua os placeholders apenas se quiser aplicar o arquivo diretamente.

3. **Aplicar manifests**
   ```bash
   kubectl apply -f k8s/postgres-storage.yaml
   kubectl apply -f k8s/
   ```

4. **Ingress (opcional)**
   ```bash
   kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
   kubectl port-forward -n ingress-nginx svc/ingress-nginx-controller 8080:80
   ```
   Acesse `http://127.0.0.1:8080/cep/01001000`.

## GitHub Actions

Workflows definidos em `.github/workflows/`:

1. **1. Test & Lint** — gofmt, go vet, testes e upload do binário `goCep`
2. **2. Build Docker** — build multi-stage e exportação como artefato
3. **3. Scan Security** — Trivy (CRITICAL/HIGH) sobre a imagem construída
4. **4. Push to Registry** — login + push para Docker Hub
5. **5. Deploy to EKS** — aplica manifests quando o pipeline chegar ao fim

### Secrets necessários

- `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN`
- `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE`, `DB_DSN`
- `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`, `EKS_CLUSTER_NAME` (deploy EKS)

## Limpeza

```bash
kind delete cluster --name gocep
docker stop pg-cep
```

---

Com isso você tem todo o ciclo local → container → Kubernetes → CI/CD operacional e documentado. Bons testes! 💡
