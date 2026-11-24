// Нагрузочный тест Currency Exchange API (k6).
//
// Аналог JMeter-тест-плана: покрывает 4 метода API, включая запись в БД
// (POST /api/v1/convert). Адрес сервиса передаётся параметром окружения
// BASE_URL (аналог -J / __P() в JMeter):
//
//   k6 run -e BASE_URL=http://192.168.1.50:30080 test/load/currency-load-test.js
//
// Наращивание нагрузки — ступенчатое (stages). Пороги (thresholds) валируют
// прогон: доля ошибок > 1% или превышение p95 завершает тест с ошибкой —
// пайплайн падает (требование В3).
//
// Сторонний API под нагрузку не ставится: метод /rates читает из кэша,
// а на время теста сервис переключается на заглушку mockfx
// (FX_EXCHANGE_API_URL=http://mock:8090).
//
// Отчёт: results/report.html (через handleSummary) + results/summary.json
// (--summary-export в CI).

import http from 'k6/http';
import { check, sleep } from 'k6';
import { htmlReport, vals } from './lib/report.js';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:30080';

// Пулы валютных пар для параметризации запросов.
const PAIRS = [
  ['USD', 'EUR'], ['USD', 'RUB'], ['EUR', 'USD'],
  ['USD', 'GBP'], ['EUR', 'RUB'], ['USD', 'JPY'],
];
const BASES = ['USD', 'EUR', 'GBP'];

function pair() {
  return PAIRS[Math.floor(Math.random() * PAIRS.length)];
}

export const options = {
  scenarios: {
    // Запись в БД — самая тяжёлая операция.
    convert_write: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '30s', target: 5 },   // разгон
        { duration: '1m', target: 10 },   // рабочая нагрузка
        { duration: '1m', target: 20 },   // пик
        { duration: '30s', target: 0 },   // спад
      ],
      exec: 'convertWrite',
    },
    rates_cached: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '30s', target: 5 },
        { duration: '1m', target: 10 },
        { duration: '1m', target: 15 },
        { duration: '30s', target: 0 },
      ],
      exec: 'ratesCached',
      startTime: '5s',
    },
    history_read: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '30s', target: 3 },
        { duration: '1m', target: 6 },
        { duration: '1m', target: 8 },
        { duration: '30s', target: 0 },
      ],
      exec: 'historyRead',
      startTime: '10s',
    },
    stats_read: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '30s', target: 3 },
        { duration: '1m', target: 6 },
        { duration: '1m', target: 8 },
        { duration: '30s', target: 0 },
      ],
      exec: 'statsRead',
      startTime: '15s',
    },
  },
  thresholds: {
    // Общий порог: не более 1% ошибок (требование В3).
    http_req_failed: ['rate<0.01'],
    // p95-пороги по сценариям: SLO сервиса.
    // 300 мс для записи (конвертация + INSERT в БД),
    // 250 мс для чтения из БД, 100 мс для кэша курсов.
    'http_req_duration{scenario:convert_write}': ['p(95)<300'],
    'http_req_duration{scenario:rates_cached}': ['p(95)<100'],
    'http_req_duration{scenario:history_read}': ['p(95)<250'],
    'http_req_duration{scenario:stats_read}': ['p(95)<250'],
  },
};

export function convertWrite() {
  const [from, to] = pair();
  const payload = JSON.stringify({ from, to, amount: Math.round(Math.random() * 900 + 100) });
  const res = http.post(`${BASE_URL}/api/v1/convert`, payload, {
    headers: { 'Content-Type': 'application/json' },
  });

  check(res, {
    'convert: статус 201': (r) => r.status === 201,
    'convert: результат положительный': (r) => {
      try {
        return r.json('result') > 0;
      } catch (_) {
        return false;
      }
    },
  });

  sleep(1);
}

export function ratesCached() {
  const base = BASES[Math.floor(Math.random() * BASES.length)];
  const res = http.get(`${BASE_URL}/api/v1/rates/${base}`);

  check(res, {
    'rates: статус 200': (r) => r.status === 200,
    'rates: база совпадает': (r) => {
      try {
        return r.json('base') === base;
      } catch (_) {
        return false;
      }
    },
  });

  sleep(1);
}

export function historyRead() {
  const res = http.get(`${BASE_URL}/api/v1/conversions?limit=20`);

  check(res, {
    'history: статус 200': (r) => r.status === 200,
    'history: ответ — массив': (r) => {
      try {
        return Array.isArray(r.json());
      } catch (_) {
        return false;
      }
    },
  });

  sleep(2);
}

export function statsRead() {
  const res = http.get(`${BASE_URL}/api/v1/stats?limit=10`);

  check(res, {
    'stats: статус 200': (r) => r.status === 200,
    'stats: ответ — массив': (r) => {
      try {
        return Array.isArray(r.json());
      } catch (_) {
        return false;
      }
    },
  });

  sleep(2);
}

export function handleSummary(summary) {
  // HTML-отчёт — артефакт пайплайна (путь можно переопределить через
  // REPORT_PATH, например при запуске k6 в контейнере); JSON — для машины.
  const reportPath = __ENV.REPORT_PATH || 'results/report.html';
  return {
    [reportPath]: htmlReport(summary),
    stdout: textSummary(summary),
  };
}

function textSummary(summary) {
  const total = vals(summary.metrics.http_reqs);
  const dur = vals(summary.metrics.http_req_duration);
  const failed = vals(summary.metrics.http_req_failed);
  return [
    '────────── k6 summary ──────────',
    `requests: ${total.count || 0}  rate: ${(total.rate || 0).toFixed(2)}/s`,
    `p95: ${(dur['p(95)'] || 0).toFixed(1)}ms  errors: ${((failed.rate || 0) * 100).toFixed(2)}%`,
    '────────────────────────────────',
  ].join('\n');
}
