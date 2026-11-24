// Генератор HTML-отчёта для k6 (handleSummary).
// Отчёт сохраняется в файл results/report.html и публикуется как артефакт
// сборки (аналог HTML-отчёта JMeter).

function esc(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

function fmt(v, digits) {
  if (v === undefined || v === null || Number.isNaN(v)) return '—';
  return Number(v).toFixed(digits === undefined ? 2 : digits);
}

function ms(v) {
  return v === undefined || v === null ? '—' : `${fmt(v)} ms`;
}

// Собирает строки таблицы по сценариям из суб-метрик с тегом scenario.
function scenarioRows(summary) {
  const scenarios = new Set();
  const durationByScenario = {};
  const failedByScenario = {};
  const reqsByScenario = {};

  for (const [name, metric] of Object.entries(summary.metrics)) {
    const m = name.match(/^http_req_duration\{scenario:([^}]+)\}$/);
    if (m) {
      scenarios.add(m[1]);
      durationByScenario[m[1]] = metric;
    }
    const f = name.match(/^http_req_failed\{scenario:([^}]+)\}$/);
    if (f) {
      scenarios.add(f[1]);
      failedByScenario[f[1]] = metric;
    }
    const r = name.match(/^http_reqs\{scenario:([^}]+)\}$/);
    if (r) {
      scenarios.add(r[1]);
      reqsByScenario[r[1]] = metric;
    }
  }

  return [...scenarios].sort().map((name) => {
    const dur = durationByScenario[name] || {};
    const failed = failedByScenario[name];
    const reqs = reqsByScenario[name];
    return `
      <tr>
        <td>${esc(name)}</td>
        <td>${esc(reqs ? fmt(reqs.count, 0) : '—')}</td>
        <td>${esc(reqs ? fmt(reqs.rate) : '—')}</td>
        <td>${ms(dur['p(90)'])}</td>
        <td>${ms(dur['p(95)'])}</td>
        <td>${ms(dur['p(99)'])}</td>
        <td>${esc(failed ? fmt(failed.rate * 100) : '—')} %</td>
      </tr>`;
  });
}

function thresholdRows(summary) {
  const rows = [];
  for (const [name, metric] of Object.entries(summary.metrics)) {
    if (!metric.thresholds) continue;
    for (const [expr, ok] of Object.entries(metric.thresholds)) {
      rows.push(`
        <tr class="${ok.ok ? 'pass' : 'fail'}">
          <td>${esc(name)}</td>
          <td>${esc(expr)}</td>
          <td>${ok.ok ? '✔ пройден' : '✘ НЕ ПРОЙДЕН'}</td>
        </tr>`);
    }
  }
  return rows.join('');
}

export function htmlReport(summary) {
  const total = summary.metrics.http_reqs || {};
  const dur = summary.metrics.http_req_duration || {};
  const failed = summary.metrics.http_req_failed || {};
  const allOk = Object.values(summary.metrics).every(
    (m) => !m.thresholds || Object.values(m.thresholds).every((t) => t.ok),
  );

  return `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<title>k6 — Currency Exchange API</title>
<style>
  body { font-family: system-ui, sans-serif; margin: 2rem; color: #1a1a2e; }
  h1 { font-size: 1.4rem; } h2 { font-size: 1.1rem; margin-top: 2rem; }
  table { border-collapse: collapse; width: 100%; font-size: 0.92rem; }
  th, td { border: 1px solid #d0d0e0; padding: 6px 10px; text-align: left; }
  th { background: #f0f0fa; }
  .pass td:last-child { color: #0a7d33; }
  .fail td:last-child { color: #c0392b; font-weight: 600; }
  .badge { display: inline-block; padding: 4px 12px; border-radius: 4px; font-weight: 600; }
  .badge.ok { background: #e6f7ec; color: #0a7d33; }
  .badge.fail { background: #fdeaea; color: #c0392b; }
  .meta { color: #666; font-size: 0.85rem; }
</style>
</head>
<body>
  <h1>Нагрузочный тест: Currency Exchange API</h1>
  <p class="meta">
    BASE_URL: ${esc(__ENV.BASE_URL || '(не задан)')} &nbsp;·&nbsp;
    Запущено: ${esc(new Date().toLocaleString('ru-RU'))}
  </p>
  <p>Итог порогов: <span class="badge ${allOk ? 'ok' : 'fail'}">${allOk ? 'ВСЕ ПОРОГИ ПРОЙДЕНЫ' : 'ЕСТЬ НАРУШЕНИЯ'}</span></p>

  <h2>Общие метрики</h2>
  <table>
    <tr><th>Метрика</th><th>Значение</th></tr>
    <tr><td>Всего запросов</td><td>${fmt(total.count, 0)}</td></tr>
    <tr><td>Пропускная способность</td><td>${fmt(total.rate)} req/s</td></tr>
    <tr><td>p90 / p95 / p99 длительности</td><td>${ms(dur['p(90)'])} / ${ms(dur['p(95)'])} / ${ms(dur['p(99)'])}</td></tr>
    <tr><td>Доля ошибочных ответов</td><td>${failed.rate === undefined ? '—' : fmt(failed.rate * 100)} %</td></tr>
  </table>

  <h2>По сценариям</h2>
  <table>
    <tr><th>Сценарий</th><th>Запросов</th><th>req/s</th><th>p90</th><th>p95</th><th>p99</th><th>Ошибки</th></tr>
    ${scenarioRows(summary).join('')}
  </table>

  <h2>Пороги (thresholds)</h2>
  <table>
    <tr><th>Метрика</th><th>Условие</th><th>Статус</th></tr>
    ${thresholdRows(summary)}
  </table>
</body>
</html>`;
}
