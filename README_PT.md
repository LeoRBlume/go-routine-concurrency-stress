# Go Goroutine Concurrency Lab

## Glossário de Termos

**Concorrência**  
Capacidade de lidar com múltiplas tarefas ao mesmo tempo, não necessariamente executando em paralelo.

**Backpressure**  
Mecanismo de controle que limita a taxa de entrada quando o sistema está saturado, evitando crescimento infinito de filas e degradação total.

**Inflight Requests**  
Requisições que já entraram no sistema e ainda não terminaram. Representam pressão interna.

**p95 / p99 (Percentis)**  
Medidas de latência de cauda. p95 significa que 95% das requisições foram mais rápidas que aquele valor.

**Timeout**  
Prazo máximo permitido para uma operação. Ao estourar, a execução é cancelada.

**Cancelamento (Context Cancellation)**  
Capacidade de interromper goroutines em andamento para evitar trabalho inútil.

**Semaphore (Semáforo)**  
Estrutura de sincronização usada para limitar concorrência.

**Throughput (RPS)**  
Quantidade de requisições processadas por segundo.

---

## Visão Geral do Projeto

Este projeto é um **laboratório local de concorrência em Go**, criado para estudar **goroutines, estratégias de sincronização, backpressure e cancelamento**, utilizando **métricas reais** exportadas via **OpenTelemetry → Prometheus → Grafana**.

O foco não é construir uma API produtiva, mas **medir e visualizar comportamentos reais** de concorrência sob carga.

Tudo roda **100% local e offline** usando Docker.

---

## Objetivos

Responder perguntas comuns que normalmente ficam vagas:

- Async é sempre melhor que sync?
- O que acontece quando a concorrência é ilimitada?
- Como o backpressure melhora a estabilidade?
- Por que timeouts são essenciais?
- Como esses efeitos aparecem nas métricas?

---

## Arquitetura

Client (k6)  
→ Go App (Gin + Goroutines)  
→ OpenTelemetry SDK  
→ OTel Collector  
→ Prometheus  
→ Grafana  

---

## Endpoints

Todos os endpoints executam a mesma lógica de negócio:

- Chamada do **Service A** (rápido e estável)
- Chamada do **Service B** (lento, variável e instável)

A diferença está **na forma como a concorrência é tratada**.

### `/sync` — Execução Sequencial

- Service A executa primeiro
- Service B executa depois

**Resultado esperado:**
- Menor throughput
- Latência = A + B
- Extremamente estável
- Poucas requisições inflight

---

### `/async` — Concorrência Ilimitada

- Service A e B executam em paralelo
- Nenhum limite de concorrência

**Resultado esperado:**
- Menor latência em baixa carga
- Melhor throughput inicial
- Crescimento rápido de inflight sob carga
- Degradação severa do p95/p99

---

### `/async-limited` — Concorrência Controlada (Backpressure)

- Service A sem limite
- Service B protegido por semáforo

**Resultado esperado:**
- Latência média levemente maior
- p95/p99 muito mais estáveis
- Inflight se estabiliza
- Tempo de espera no semáforo visível

---

### `/async-timeout` — Cancelamento por Deadline

- Execução paralela
- Timeout forçado via context

**Resultado esperado:**
- Parte das requisições falha por timeout (comportamento esperado)
- Inflight cai rapidamente após picos
- Goroutines não ficam presas
- Sistema se recupera mais rápido

---

## Serviços

### Service A
- 50–150ms
- Estável
- Simula cache ou computação local

### Service B
- 300–1200ms
- 5% de erro
- Gargalo proposital
- Pode simular contenção

---

## Métricas Coletadas

### HTTP
- `http_requests_total`
- `http_request_duration_ms`
- `http_inflight`

### Serviços
- `service_duration_ms`
- `service_errors_total`

### Backpressure
- `serviceB_semaphore_wait_ms`

### Runtime
- Goroutines
- Heap
- GC

Essas métricas permitem **análise de causa e efeito**, não apenas sintomas.

---

## Dashboard Grafana

O dashboard permite observar:

- RPS por endpoint
- Inflight por endpoint
- Latência média, p95 e p99
- Taxa de erro
- Custo do semáforo
- Uso de goroutines e memória

---

## Como Executar

### Requisitos
- Docker
- Docker Compose

### Subir o ambiente

```bash
docker compose down -v
docker compose up --build
```

Acessos:
- App: http://localhost:8080
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin / admin)

---

## Teste de Carga com k6

### Linux

```bash
docker run --rm -i   --add-host=host.docker.internal:host-gateway   -e TARGET_RPS=30   -e REQ_TIMEOUT=5s   grafana/k6 run - < k6.js
```

### Windows / macOS

```bash
docker run --rm -i   -e TARGET_RPS=30   -e REQ_TIMEOUT=5s   grafana/k6 run - < k6.js
```

---

## Como Interpretar os Resultados

- Inflight subindo continuamente → saturação
- p95 subindo após inflight → fila interna
- Inflight estável → backpressure funcionando
- Queda rápida de inflight → cancelamento eficaz

**Latência mostra o efeito. Inflight mostra a causa.**

---

## Conclusão

Este projeto transforma conceitos abstratos de concorrência em **evidência mensurável**.

Ele mostra, na prática, que:
- Async sem limites é perigoso
- Backpressure é essencial
- Timeout evita colapso
- Métricas explicam mais do que intuição

---

## Licença

MIT
