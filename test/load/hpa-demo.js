// Демонстрация автоскейлинга HPA (модуль 10): агрессивная нагрузка на
// /convert (запись в БД) и /stats (SQL-агрегация), чтобы поды сервиса
// вышли за порог CPU 60% и HorizontalPodAutoscaler поднял реплики 1 → 4.
//
// Запуск на отдельной от кластера машине (или локально):
//   k6 run -e BASE_URL=http://192.168.45.100:30090 test/load/hpa-demo.js
//
// Параллельно наблюдать масштабирование:
//   watch -n 2 'kubectl -n currency get hpa currency'
//   kubectl -n currency get pods -l app.kubernetes.io/component=api -w
//
// После окончания нагрузки реплики сокращаются до minReplicas (~5 минут,
// стандартная стабилизация scale-down).

import http from 'k6/http';
import { sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://192.168.45.100:30090';
const DURATION = __ENV.DURATION || '3m';
const VUS = Number(__ENV.VUS || 60);

export const options = {
  vus: VUS,
  duration: DURATION,
  thresholds: {
    // Демо скейлинга — жёсткая нагрузка, допускаем до 5% ошибок,
    // p95 не валируем: цель демо — рост CPU, а не SLO.
    http_req_failed: ['rate<0.05'],
  },
};

const PAIRS = [
  ['USD', 'EUR'], ['USD', 'RUB'], ['EUR', 'USD'],
  ['USD', 'GBP'], ['EUR', 'RUB'], ['USD', 'JPY'],
];

export default function () {
  const [from, to] = PAIRS[Math.floor(Math.random() * PAIRS.length)];
  const payload = JSON.stringify({ from, to, amount: Math.random() * 1000 + 1 });

  http.post(`${BASE_URL}/api/v1/convert`, payload, {
    headers: { 'Content-Type': 'application/json' },
  });
  http.get(`${BASE_URL}/api/v1/stats?limit=10`);

  sleep(0.2);
}
