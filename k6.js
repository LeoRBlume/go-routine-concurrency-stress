import http from "k6/http";
import { check, sleep } from "k6";
import exec from "k6/execution";

const BASE_URL = __ENV.BASE_URL || "http://host.docker.internal:8080";

// RPS por endpoint (iguais). Total do teste = RPS * 4
const TARGET_RPS = Number(__ENV.TARGET_RPS || 60);

// Bursts: chance de pico + intensidade
const BURST_CHANCE = Number(__ENV.BURST_CHANCE || 0.08); // 8%
const BURST_HITS = Number(__ENV.BURST_HITS || 3);        // hits extras no burst

const TIMEOUT = __ENV.REQ_TIMEOUT || "2s";

export const options = {
    scenarios: {
        sync_group: {
            executor: "ramping-arrival-rate",
            startRate: 0,
            timeUnit: "1s",
            preAllocatedVUs: 300,
            maxVUs: 4000,
            stages: [
                { duration: "1m", target: TARGET_RPS },
                { duration: "8m", target: TARGET_RPS },
                { duration: "1m", target: 0 },
            ],
            exec: "syncFlow",
            tags: { endpoint: "sync" },
        },

        async_group: {
            executor: "ramping-arrival-rate",
            startRate: 0,
            timeUnit: "1s",
            preAllocatedVUs: 300,
            maxVUs: 4000,
            stages: [
                { duration: "1m", target: TARGET_RPS },
                { duration: "8m", target: TARGET_RPS },
                { duration: "1m", target: 0 },
            ],
            exec: "asyncFlow",
            tags: { endpoint: "async" },
        },

        limited_group: {
            executor: "ramping-arrival-rate",
            startRate: 0,
            timeUnit: "1s",
            preAllocatedVUs: 300,
            maxVUs: 4000,
            stages: [
                { duration: "1m", target: TARGET_RPS },
                { duration: "8m", target: TARGET_RPS },
                { duration: "1m", target: 0 },
            ],
            exec: "limitedFlow",
            tags: { endpoint: "async-limited" },
        },

        timeout_group: {
            executor: "ramping-arrival-rate",
            startRate: 0,
            timeUnit: "1s",
            preAllocatedVUs: 300,
            maxVUs: 4000,
            stages: [
                { duration: "1m", target: TARGET_RPS },
                { duration: "8m", target: TARGET_RPS },
                { duration: "1m", target: 0 },
            ],
            exec: "timeoutFlow",
            tags: { endpoint: "async-timeout" },
        },
    },

    thresholds: {
        // Sync tende a ficar pior; limite aqui é "realista" para não travar o aprendizado
        "http_req_duration{endpoint:sync}": ["p(95)<3000"],
        "http_req_failed{endpoint:sync}": ["rate<0.10"],

        // Async deve ser melhor que sync, mas pode degradar com contenção/erro
        "http_req_duration{endpoint:async}": ["p(95)<2200"],
        "http_req_failed{endpoint:async}": ["rate<0.10"],

        // Limited normalmente estabiliza cauda (p95). Pode aumentar média, mas segura p95.
        "http_req_duration{endpoint:async-limited}": ["p(95)<2500"],
        "http_req_failed{endpoint:async-limited}": ["rate<0.10"],

        // Timeout endpoint vai retornar 408 por design em parte das requests.
        // Não trate isso como "failed" aqui; vamos observar taxa de 408 no Grafana.
        "http_req_duration{endpoint:async-timeout}": ["p(95)<2000"],
        "http_req_failed{endpoint:async-timeout}": ["rate<0.20"]
    },
};

function jitter() {
    // evita lockstep perfeito, simula tempos de usuário/rede
    sleep(Math.random() * 0.05);
}

function maybeBurst(path, endpointName) {
    // picos curtos e não sincronizados
    if (Math.random() < BURST_CHANCE) {
        for (let i = 0; i < BURST_HITS; i++) {
            http.get(`${BASE_URL}${path}`, {
                tags: { endpoint: endpointName, burst: "true" },
                timeout: TIMEOUT,
            });
        }
    }
}

function hit(path, endpointName, acceptTimeout408 = false) {
    const res = http.get(`${BASE_URL}${path}`, {
        tags: { endpoint: endpointName },
        timeout: TIMEOUT,
    });

    // checks:
    // - para a maioria: esperamos 200 (mas aceitamos 5xx devido a erro simulado - o Grafana vai mostrar)
    // - para async-timeout: aceitamos 200 OU 408 (porque é o objetivo)
    const ok =
        acceptTimeout408
            ? (res.status === 200 || res.status === 408)
            : (res.status === 200);

    check(res, {
        "status acceptable": () => ok,
    });

    // burst a cada ~30 iterações por cenário (não sincroniza tudo)
    if (exec.scenario.iterationInTest % 30 === 0) {
        maybeBurst(path, endpointName);
    }

    jitter();
}

export function syncFlow() {
    hit("/sync", "sync", false);
}

export function asyncFlow() {
    hit("/async", "async", false);
}

export function limitedFlow() {
    hit("/async-limited", "async-limited", false);
}

export function timeoutFlow() {
    hit("/async-timeout", "async-timeout", true);
}
