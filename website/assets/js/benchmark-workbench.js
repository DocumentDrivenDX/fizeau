import * as duckdb from "@duckdb/duckdb-wasm";
import "@perspective-dev/viewer/dist/esm/perspective-viewer.inline.js";
import "@perspective-dev/viewer-datagrid/dist/esm/perspective-viewer-datagrid.js";
import { DuckDBHandler } from "@perspective-dev/client/dist/esm/virtual_servers/duckdb.js";

const DEFAULT_COLUMNS = [
  "task",
  "task_category",
  "task_difficulty",
  "result_state",
  "model_display_name",
  "provider_type",
  "harness_label",
  "quant_display",
  "engine",
  "effective_gpu_model",
  "turns",
  "total_tokens",
  "cost_usd",
  "wall_seconds",
  "profile_ttft_p50_s",
  "profile_decode_tps_p50",
  "profile_timing_turns",
  "finished_at",
];

const FILTER_FIELDS = [
  { key: "model_family", label: "Model family", allLabel: "All families" },
  { key: "model_quant", label: "Model quant", allLabel: "All model quants" },
  { key: "kv_cache_quant", label: "KV cache", allLabel: "All KV caches" },
  { key: "k_quant", label: "K quant", allLabel: "All K quants" },
  { key: "v_quant", label: "V quant", allLabel: "All V quants" },
  { key: "runtime_mtp_enabled", label: "MTP", allLabel: "All MTP states" },
  { key: "provider_type", label: "Provider", allLabel: "All providers" },
  { key: "provider_surface", label: "Surface", allLabel: "All surfaces" },
  { key: "harness_label", label: "Harness", allLabel: "All harnesses" },
  { key: "profile_id", label: "Profile", allLabel: "All profiles" },
  { key: "task_category", label: "Task category", allLabel: "All categories" },
  { key: "task_difficulty", label: "Difficulty", allLabel: "All difficulties" },
  { key: "deployment_class", label: "Deployment", allLabel: "All deployments" },
  { key: "machine", label: "Machine", allLabel: "All machines" },
  { key: "gpu_vendor", label: "GPU vendor", allLabel: "All vendors" },
  { key: "hardware_chip_family", label: "Chip family", allLabel: "All chip families" },
  { key: "hardware_memory_type", label: "Memory type", allLabel: "All memory types" },
  { key: "backend", label: "Backend", allLabel: "All backends" },
  { key: "runtime_draft_mode", label: "Draft mode", allLabel: "All draft modes" },
  { key: "runtime_draft_model", label: "Draft model", allLabel: "All draft models" },
  { key: "kv_cache_disk", label: "KV disk", allLabel: "All KV disk states" },
  { key: "sampling_reasoning", label: "Reasoning", allLabel: "All reasoning" },
  { key: "sampling_temperature", label: "Temperature", allLabel: "All temperatures" },
  { key: "sampling_top_p", label: "Top-p", allLabel: "All top-p values" },
  { key: "sampling_top_k", label: "Top-k", allLabel: "All top-k values" },
  { key: "context_tokens", label: "Context", allLabel: "All contexts" },
  { key: "max_output_tokens", label: "Output cap", allLabel: "All output caps" },
];

const COMPARISON_DIMENSIONS = {
  task_category: { key: "task_category", label: "Task category" },
  task_difficulty: { key: "task_difficulty", label: "Task difficulty" },
  task: { key: "task", label: "Task" },
  result_state: { key: "result_state", label: "Outcome" },
  engine: { key: "engine", label: "Engine" },
  model_quant: { key: "model_quant", label: "Model quant" },
  deployment_class: { key: "deployment_class", label: "Deployment" },
  gpu_vendor: { key: "gpu_vendor", label: "GPU vendor" },
  effective_gpu_model: { key: "effective_gpu_model", label: "GPU" },
  sampling_reasoning: { key: "sampling_reasoning", label: "Reasoning" },
  provider_type: { key: "provider_type", label: "Provider" },
  harness_label: { key: "harness_label", label: "Harness" },
};

const ROUTES = {
  summary: "Summary",
  data: "Raw database",
  compare: "Comparison",
  combinations: "Combinations",
};

const COMPARE_FILTER_FIELDS = [
  { key: "task_category", label: "Task category", allLabel: "All categories" },
  { key: "task_difficulty", label: "Difficulty", allLabel: "All difficulties" },
  { key: "task", label: "Task", allLabel: "All tasks" },
  { key: "engine", label: "Engine", allLabel: "All engines" },
  { key: "effective_gpu_model", label: "GPU", allLabel: "All GPUs" },
  { key: "provider_type", label: "Provider", allLabel: "All providers" },
  { key: "harness_label", label: "Harness", allLabel: "All harnesses" },
  { key: "model_quant", label: "Model quant", allLabel: "All model quants" },
  { key: "kv_cache_quant", label: "KV cache", allLabel: "All KV caches" },
  { key: "runtime_mtp_enabled", label: "MTP", allLabel: "All MTP states" },
  { key: "gpu_vendor", label: "GPU vendor", allLabel: "All vendors" },
  { key: "sampling_reasoning", label: "Reasoning", allLabel: "All reasoning" },
];

const COMPARISON_SORTS = {
  bucket: { type: "string" },
  a_pass_rate: { type: "number" },
  b_pass_rate: { type: "number" },
  gap_pp: { type: "number" },
  a_rows: { type: "number" },
  b_rows: { type: "number" },
  a_fail: { type: "number" },
  b_fail: { type: "number" },
  a_timeout: { type: "number" },
  b_timeout: { type: "number" },
  a_tokens: { type: "number" },
  b_tokens: { type: "number" },
  a_wall_p50: { type: "number" },
  b_wall_p50: { type: "number" },
};

const COMBINATION_SORTS = {
  task: { type: "string" },
  model_display_name: { type: "string" },
  model_quant: { type: "string" },
  kv_cache_quant: { type: "string" },
  runtime_mtp_enabled: { type: "string" },
  engine: { type: "string" },
  gpu: { type: "string" },
  gpu_ram_gb: { type: "number" },
  n_rows: { type: "number" },
  n_pass: { type: "number" },
  n_fail: { type: "number" },
  n_timeout: { type: "number" },
  pass_rate: { type: "number" },
  token_total: { type: "number" },
  cost_total: { type: "number" },
  wall_p50: { type: "number" },
};

const SUMMARY_COLORS = ["#0a8e8e", "#c46a00", "#5e3aa8", "#1f7a3f", "#b22b2b", "#4f6f9f", "#7a5c1d", "#4a7a64"];

const NUMBER_FORMAT = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 });
const INTEGER_FORMAT = new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 });
const USD_FORMAT = new Intl.NumberFormat(undefined, {
  style: "currency",
  currency: "USD",
  maximumFractionDigits: 2,
});
const TERMINAL_BENCH_TASK_BASE = "https://www.tbench.ai/registry/terminal-bench-core/head/";
const RESULT_STATE_PASSED = "passed";

const RAW_STATE_KEYS = {
  preset: "preset",
  search: "q",
  resultState: "outcome",
  task: "task",
  model: "model",
  engine: "engine",
  gpu: "gpu",
  maxRam: "max_ram",
  passedOnly: "passed",
};

const COMPARE_STATE_KEYS = {
  a: "a",
  b: "b",
  dimension: "dim",
  maxRam: "max_ram",
  sort: "sort",
};

const COMBINATION_STATE_KEYS = {
  task: "task",
  model: "model",
  gpu: "gpu",
  passedOnly: "passed",
  sort: "sort",
};

const RAW_FILTER_PREFIX = "f.";
const COMPARE_FILTER_PREFIX = "cf.";

function ready(fn) {
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", fn, { once: true });
  } else {
    fn();
  }
}

function absolutize(url) {
  return new URL(url, window.location.href).toString();
}

function sqlString(value) {
  return `'${String(value).replace(/'/g, "''")}'`;
}

function valueOf(row, key) {
  return row && Object.prototype.hasOwnProperty.call(row, key) ? row[key] : undefined;
}

function normalizeScalar(value) {
  if (typeof value === "bigint") return Number(value);
  if (typeof value === "string" && /^-?\d+n$/.test(value)) return Number(value.slice(0, -1));
  if (value && typeof value.toJSON === "function") return normalizeScalar(value.toJSON());
  return value;
}

function sqlNumber(value) {
  const num = Number(normalizeScalar(value));
  return Number.isFinite(num) ? String(num) : "NULL";
}

function sqlInteger(value) {
  const num = Number(normalizeScalar(value));
  return Number.isSafeInteger(num) ? String(num) : "NULL";
}

function normalizeRow(row) {
  const raw = row && typeof row.toJSON === "function" ? row.toJSON() : row;
  return Object.fromEntries(Object.entries(raw || {}).map(([key, value]) => [key, normalizeScalar(value)]));
}

async function queryRows(conn, sql) {
  const table = await conn.query(sql);
  return table.toArray().map(normalizeRow);
}

function formatCount(value) {
  const num = Number(normalizeScalar(value) ?? 0);
  return Number.isFinite(num) ? INTEGER_FORMAT.format(num) : "-";
}

function formatNumber(value, suffix = "") {
  const num = Number(normalizeScalar(value));
  if (value === null || value === undefined || !Number.isFinite(num)) return "-";
  return `${NUMBER_FORMAT.format(num)}${suffix}`;
}

function formatPercent(value) {
  const num = Number(normalizeScalar(value));
  if (value === null || value === undefined || !Number.isFinite(num)) return "-";
  return `${NUMBER_FORMAT.format(num * 100)}%`;
}

function formatGap(value) {
  const num = Number(normalizeScalar(value));
  if (value === null || value === undefined || !Number.isFinite(num)) return "-";
  const sign = num > 0 ? "+" : "";
  return `${sign}${NUMBER_FORMAT.format(num)} pp`;
}

function compareSortValue(row, key, sortDefs) {
  const value = sortDefs[key]?.value ? sortDefs[key].value(row) : row[key];
  if (value === null || value === undefined || value === "") return null;
  if (sortDefs[key]?.type === "number") {
    const num = Number(normalizeScalar(value));
    return Number.isFinite(num) ? num : null;
  }
  return String(value).toLowerCase();
}

function sortRows(rows, sort, sortDefs) {
  const def = sortDefs[sort.key];
  if (!def) return rows;
  const direction = sort.direction === "asc" ? 1 : -1;
  return [...rows].sort((left, right) => {
    const a = compareSortValue(left, sort.key, sortDefs);
    const b = compareSortValue(right, sort.key, sortDefs);
    if (a === null && b === null) return 0;
    if (a === null) return 1;
    if (b === null) return -1;
    if (a < b) return -direction;
    if (a > b) return direction;
    return 0;
  });
}

function nextSort(current, key) {
  if (current.key !== key) return { key, direction: "asc" };
  return { key, direction: current.direction === "asc" ? "desc" : "asc" };
}

function parseSort(value, sortDefs, fallback) {
  const [key, direction] = String(value || "").split(":");
  if (!Object.prototype.hasOwnProperty.call(sortDefs, key)) return { ...fallback };
  return { key, direction: direction === "asc" ? "asc" : "desc" };
}

function encodeSort(sort) {
  return `${sort.key}:${sort.direction === "asc" ? "asc" : "desc"}`;
}

function setControlValue(control, value) {
  if (!control || value === null || value === undefined) return;
  const next = String(value);
  if (control.tagName === "SELECT" && ![...control.options].some((option) => option.value === next)) return;
  control.value = next;
}

function setCheckboxValue(control, value) {
  if (!control) return;
  control.checked = value === "1" || value === "true";
}

function sortHeader(label, key, state) {
  const active = state.key === key;
  const direction = active ? state.direction : "none";
  const indicator = active ? (state.direction === "asc" ? "^" : "v") : "";
  return `
    <th aria-sort="${direction === "none" ? "none" : direction === "asc" ? "ascending" : "descending"}">
      <button type="button" data-bw-sort="${escapeHtml(key)}" aria-label="Sort by ${escapeHtml(label)}">
        <span>${escapeHtml(label)}</span>
        <span aria-hidden="true">${indicator}</span>
      </button>
    </th>
  `;
}

function setOptions(select, rows, valueKey, labelKey, allLabel) {
  if (!select) return;
  const current = select.value;
  select.innerHTML = "";
  const all = document.createElement("option");
  all.value = "";
  all.textContent = allLabel;
  select.appendChild(all);

  for (const row of rows) {
    const value = valueOf(row, valueKey);
    if (value === null || value === undefined || value === "") continue;
    const label = labelKey ? String(valueOf(row, labelKey)) : String(value);
    const option = document.createElement("option");
    option.value = String(value);
    option.dataset.label = label;
    option.textContent = `${label} (${formatCount(row.n)})`;
    select.appendChild(option);
  }

  if ([...select.options].some((option) => option.value === current)) {
    select.value = current;
  }
}

function makePerspectiveClient(handler, mod) {
  const channel = new MessageChannel();
  let server;

  channel.port1.onmessage = async (event) => {
    if (event.data?.cmd === "init") {
      server = new mod.VirtualServer(handler);
      channel.port1.postMessage(null);
      return;
    }

    try {
      const request = new Uint8Array(event.data);
      const response = (await server.handleRequest(request)).slice().buffer;
      channel.port1.postMessage(response, [response]);
    } catch (error) {
      console.error("Perspective virtual server request failed", error);
      throw error;
    }
  };

  const port = channel.port2;
  const client = new mod.Client(
    async (request) => {
      const payload = request.slice().buffer;
      port.postMessage(payload, [payload]);
    },
    async () => {
      port.close();
      channel.port1.close();
    },
  );

  port.addEventListener("message", (event) => {
    client.handle_response(event.data);
  });
  port.start();

  return new Promise((resolve) => {
    port.addEventListener("message", () => resolve(client), { once: true });
    port.postMessage({ cmd: "init" });
  });
}

function taskUrl(task) {
  return `${TERMINAL_BENCH_TASK_BASE}${encodeURIComponent(String(task))}`;
}

class BenchmarkWorkbench {
  constructor(root) {
    this.root = root;
    this.status = root.querySelector("[data-bw-status]");
    this.routes = new Map([...root.querySelectorAll("[data-bw-route]")].map((el) => [el.dataset.bwRoute, el]));
    this.panes = new Map([...root.querySelectorAll("[data-bw-pane]")].map((el) => [el.dataset.bwPane, el]));
    this.viewer = root.querySelector("[data-bw-viewer]");
    this.search = root.querySelector("[data-bw-search]");
    this.resultState = root.querySelector("[data-bw-result-state]");
    this.task = root.querySelector("[data-bw-task]");
    this.model = root.querySelector("[data-bw-model]");
    this.engine = root.querySelector("[data-bw-engine]");
    this.gpu = root.querySelector("[data-bw-gpu]");
    this.maxRam = root.querySelector("[data-bw-max-ram]");
    this.passedOnly = root.querySelector("[data-bw-passed-only]");
    this.activeFilters = root.querySelector("[data-bw-active-filters]");
    this.clearFilters = root.querySelector("[data-bw-clear-filters]");
    this.compareA = root.querySelector("[data-bw-compare-a]");
    this.compareB = root.querySelector("[data-bw-compare-b]");
    this.compareDimension = root.querySelector("[data-bw-compare-dimension]");
    this.compareMaxRam = root.querySelector("[data-bw-compare-max-ram]");
    this.comparison = root.querySelector("[data-bw-comparison]");
    this.comboTask = root.querySelector("[data-bw-combo-task]");
    this.comboModel = root.querySelector("[data-bw-combo-model]");
    this.comboGpu = root.querySelector("[data-bw-combo-gpu]");
    this.comboPassedOnly = root.querySelector("[data-bw-combo-passed-only]");
    this.combinations = root.querySelector("[data-bw-combinations]");
    this.openConfig = root.querySelector("[data-bw-open-config]");
    this.enumFilters = new Map(
      [...root.querySelectorAll("[data-bw-filter-field]")].map((select) => [select.dataset.bwFilterField, select]),
    );
    this.compareFilters = new Map(
      [...root.querySelectorAll("[data-bw-compare-filter-field]")].map((select) => [select.dataset.bwCompareFilterField, select]),
    );
    this.metrics = Object.fromEntries(
      [...root.querySelectorAll("[data-bw-metric]")].map((el) => [el.dataset.bwMetric, el]),
    );
    this.summaryMetrics = Object.fromEntries(
      [...root.querySelectorAll("[data-bw-summary-metric]")].map((el) => [el.dataset.bwSummaryMetric, el]),
    );
    this.summaryCharts = Object.fromEntries(
      [...root.querySelectorAll("[data-bw-chart]")].map((el) => [el.dataset.bwChart, el]),
    );
    this.cellsUrl = absolutize(root.dataset.cellsUrl || "/data/cells.parquet");
    this.timingUrl = absolutize(root.dataset.timingUrl || "/data/profile-timing.json");
    this.manifestUrl = absolutize(root.dataset.manifestUrl || "/data/benchmark-data-manifest.json");
    this.duckdbBase = absolutize(root.dataset.duckdbBase || "/vendor/duckdb/");
    this.activePreset = "all";
    this.activePane = "summary";
    this.comparisonSort = { key: "gap_pp", direction: "desc" };
    this.combinationSort = { key: "n_pass", direction: "desc" };
    this.reloadTimer = null;
    this.compareReloadTimer = null;
    this.comboReloadTimer = null;
    this.taskLinkUnsubscribe = null;
    this.taskLinkTable = null;
    this.gridConfig = null;
    this.gridConfigReady = false;
    this.restoringRouteState = false;
  }

  async init() {
    if (!window.Worker || !window.WebAssembly) {
      throw new Error("This browser does not support the Worker and WebAssembly APIs required by the benchmark workbench.");
    }

    await customElements.whenDefined("perspective-viewer");
    await customElements.whenDefined("perspective-viewer-datagrid");

    this.setStatus("Loading artifact manifest...");
    await this.loadManifest();
    await this.loadTiming();

    this.setStatus("Starting DuckDB-WASM...");
    await this.initDuckDB();

    this.setStatus("Registering Parquet data...");
    await this.initViews();

    this.setStatus("Preparing controls...");
    await this.populateControls();
    this.bindEvents();
    this.applyRouteFromLocation({ replace: true });

    this.setStatus("Loading analytical grid...");
    await this.reload();
  }

  async loadManifest() {
    try {
      const response = await fetch(this.manifestUrl);
      if (!response.ok) return;
      this.manifest = await response.json();
    } catch {
      this.manifest = null;
    }
  }

  async loadTiming() {
    this.profileTiming = {};
    try {
      const response = await fetch(this.timingUrl);
      if (!response.ok) return;
      const payload = await response.json();
      const rows = Array.isArray(payload)
        ? payload
        : Array.isArray(payload?.rows)
        ? payload.rows
        : Object.entries(payload || {}).map(([profileID, timing]) => ({
            profile_id: profileID,
            profile_ttft_p50_s: timing?.ttft_p50,
            profile_decode_tps_p50: timing?.decode_tps_p50,
            profile_timing_turns: timing?.n_turns,
          }));
      for (const row of rows) {
        if (!row?.profile_id) continue;
        this.profileTiming[String(row.profile_id)] = {
          profile_ttft_p50_s: normalizeScalar(row.profile_ttft_p50_s),
          profile_decode_tps_p50: normalizeScalar(row.profile_decode_tps_p50),
          profile_timing_turns: normalizeScalar(row.profile_timing_turns),
        };
      }
    } catch {
      this.profileTiming = {};
    }
  }

  async initDuckDB() {
    const assetBase = this.duckdbBase;
    const bundles = {
      mvp: {
        mainModule: `${assetBase}duckdb-mvp.wasm`,
        mainWorker: `${assetBase}duckdb-browser-mvp.worker.js`,
      },
      eh: {
        mainModule: `${assetBase}duckdb-eh.wasm`,
        mainWorker: `${assetBase}duckdb-browser-eh.worker.js`,
      },
    };

    const bundle = await duckdb.selectBundle(bundles);
    this.duckWorker = new Worker(bundle.mainWorker);
    this.db = new duckdb.AsyncDuckDB(new duckdb.VoidLogger(), this.duckWorker);
    await this.db.instantiate(bundle.mainModule, bundle.pthreadWorker);
    this.conn = await this.db.connect();
    await this.db.registerFileURL("cells.parquet", this.cellsUrl, duckdb.DuckDBDataProtocol.HTTP, false);
  }

  async initViews() {
    await this.conn.query(`
      CREATE OR REPLACE VIEW cells_raw AS
      SELECT * FROM read_parquet('cells.parquet')
    `);

    await this.createProfileTimingTable();

    await this.conn.query(`
      CREATE OR REPLACE VIEW cells_enriched AS
      SELECT
        c.*,
        COALESCE(pt_profile.profile_ttft_p50_s, pt_report.profile_ttft_p50_s) AS profile_ttft_p50_s,
        COALESCE(pt_profile.profile_decode_tps_p50, pt_report.profile_decode_tps_p50) AS profile_decode_tps_p50,
        COALESCE(pt_profile.profile_timing_turns, pt_report.profile_timing_turns) AS profile_timing_turns,
        COALESCE(NULLIF(gpu_model, ''), NULLIF(hardware_chip, ''), NULLIF(hardware_profile, ''), NULLIF(machine, ''), 'unknown') AS effective_gpu_model,
        COALESCE(gpu_ram_gb, hardware_vram_gb, hardware_memory_gb) AS effective_gpu_ram_gb,
        CASE
          WHEN task IS NOT NULL AND suite = 'terminal-bench-2-1'
          THEN concat('${TERMINAL_BENCH_TASK_BASE}', task)
          ELSE NULL
        END AS terminalbench_task_url,
        lower(concat_ws(' ',
          COALESCE(CAST(suite AS VARCHAR), ''),
          COALESCE(CAST(task AS VARCHAR), ''),
          COALESCE(CAST(task_subsets AS VARCHAR), ''),
          COALESCE(CAST(result_state AS VARCHAR), ''),
          COALESCE(CAST(harness AS VARCHAR), ''),
          COALESCE(CAST(harness_label AS VARCHAR), ''),
          COALESCE(CAST(provider_type AS VARCHAR), ''),
          COALESCE(CAST(provider_surface AS VARCHAR), ''),
          COALESCE(CAST(model_display_name AS VARCHAR), ''),
          COALESCE(CAST(model_family AS VARCHAR), ''),
          COALESCE(CAST(model AS VARCHAR), ''),
          COALESCE(CAST(model_quant AS VARCHAR), ''),
          COALESCE(CAST(quant_display AS VARCHAR), ''),
          COALESCE(CAST(kv_cache_quant AS VARCHAR), ''),
          COALESCE(CAST(k_quant AS VARCHAR), ''),
          COALESCE(CAST(v_quant AS VARCHAR), ''),
          COALESCE(CAST(runtime_mtp_enabled AS VARCHAR), ''),
          COALESCE(CAST(runtime_draft_mode AS VARCHAR), ''),
          COALESCE(CAST(runtime_draft_model AS VARCHAR), ''),
          COALESCE(CAST(engine AS VARCHAR), ''),
          COALESCE(CAST(backend AS VARCHAR), ''),
          COALESCE(CAST(gpu_model AS VARCHAR), ''),
          COALESCE(CAST(gpu_vendor AS VARCHAR), ''),
          COALESCE(CAST(hardware_chip_family AS VARCHAR), ''),
          COALESCE(CAST(hardware_memory_type AS VARCHAR), ''),
          COALESCE(CAST(task_category AS VARCHAR), ''),
          COALESCE(CAST(task_difficulty AS VARCHAR), ''),
          COALESCE(CAST(c.profile_id AS VARCHAR), ''),
          COALESCE(CAST(deployment_class AS VARCHAR), ''),
          COALESCE(CAST(sampling_reasoning AS VARCHAR), ''),
          COALESCE(CAST(sampling_temperature AS VARCHAR), ''),
          COALESCE(CAST(sampling_top_p AS VARCHAR), ''),
          COALESCE(CAST(sampling_top_k AS VARCHAR), ''),
          COALESCE(CAST(context_tokens AS VARCHAR), ''),
          COALESCE(CAST(max_output_tokens AS VARCHAR), ''),
          COALESCE(CAST(machine AS VARCHAR), ''),
          COALESCE(CAST(machine_label AS VARCHAR), ''),
          COALESCE(CAST(final_status AS VARCHAR), ''),
          COALESCE(CAST(invalid_class AS VARCHAR), ''),
          COALESCE(CAST(descriptor AS VARCHAR), '')
        )) AS search_text
      FROM cells_raw AS c
      LEFT JOIN profile_timing AS pt_profile ON c.profile_id = pt_profile.profile_id
      LEFT JOIN profile_timing AS pt_report ON c.report_profile_id = pt_report.profile_id
    `);

    await this.conn.query("CREATE OR REPLACE TABLE workbench_cells AS SELECT * FROM cells_enriched");

    const mod = customElements.get("perspective-viewer").__wasm_module__;
    this.perspectiveClient = await makePerspectiveClient(new DuckDBHandler(this.conn, mod), mod);
  }

  async createProfileTimingTable() {
    await this.conn.query(`
      CREATE OR REPLACE TABLE profile_timing (
        profile_id VARCHAR,
        profile_ttft_p50_s DOUBLE,
        profile_decode_tps_p50 DOUBLE,
        profile_timing_turns BIGINT
      )
    `);

    const values = Object.entries(this.profileTiming || {}).map(([profileID, timing]) => `(
      ${sqlString(profileID)},
      ${sqlNumber(timing.profile_ttft_p50_s)},
      ${sqlNumber(timing.profile_decode_tps_p50)},
      ${sqlInteger(timing.profile_timing_turns)}
    )`);
    if (values.length) {
      await this.conn.query(`INSERT INTO profile_timing VALUES ${values.join(",")}`);
    }
  }

  async populateControls() {
    const enumQueries = FILTER_FIELDS.map((field) => queryRows(this.conn, `
      SELECT CAST(${field.key} AS VARCHAR) AS value, count(*) AS n
      FROM cells_enriched
      WHERE ${field.key} IS NOT NULL AND CAST(${field.key} AS VARCHAR) <> ''
      GROUP BY ${field.key}
      ORDER BY n DESC, value
    `));
    const compareFilterQueries = COMPARE_FILTER_FIELDS.map((field) => queryRows(this.conn, `
      SELECT CAST(${field.key} AS VARCHAR) AS value, count(*) AS n
      FROM cells_enriched
      WHERE ${field.key} IS NOT NULL AND CAST(${field.key} AS VARCHAR) <> ''
      GROUP BY ${field.key}
      ORDER BY n DESC, value
    `));

    const [resultStates, tasks, models, modelFamilies, engines, gpus, ...controlRows] = await Promise.all([
      queryRows(this.conn, `
        SELECT result_state, count(*) AS n
        FROM cells_enriched
        WHERE result_state IS NOT NULL AND result_state <> ''
        GROUP BY result_state
        ORDER BY result_state
      `),
      queryRows(this.conn, `
        SELECT task, count(*) AS n
        FROM cells_enriched
        WHERE task IS NOT NULL
        GROUP BY task
        ORDER BY task
      `),
      queryRows(this.conn, `
        SELECT model_display_name AS model, count(*) AS n
        FROM cells_enriched
        WHERE model_display_name IS NOT NULL AND model_display_name <> ''
        GROUP BY model_display_name
        ORDER BY n DESC, model_display_name
      `),
      queryRows(this.conn, `
        SELECT
          model_family AS value,
          concat(any_value(model_display_name), ' · ', model_family) AS label,
          count(*) AS n
        FROM cells_enriched
        WHERE model_family IS NOT NULL AND model_family <> ''
        GROUP BY model_family
        ORDER BY n DESC, model_family
      `),
      queryRows(this.conn, `
        SELECT engine, count(*) AS n
        FROM cells_enriched
        WHERE engine IS NOT NULL AND engine <> ''
        GROUP BY engine
        ORDER BY n DESC, engine
      `),
      queryRows(this.conn, `
        SELECT effective_gpu_model AS gpu, count(*) AS n
        FROM cells_enriched
        WHERE effective_gpu_model IS NOT NULL AND effective_gpu_model <> 'unknown'
        GROUP BY effective_gpu_model
        ORDER BY n DESC, effective_gpu_model
      `),
      ...enumQueries,
      ...compareFilterQueries,
    ]);
    const enumRows = controlRows.slice(0, FILTER_FIELDS.length);
    const compareFilterRows = controlRows.slice(FILTER_FIELDS.length);

    setOptions(this.resultState, resultStates, "result_state", "result_state", "All outcomes");
    setOptions(this.task, tasks, "task", "task", "All tests");
    setOptions(this.model, models, "model", "model", "All models");
    setOptions(this.comboTask, tasks, "task", "task", "All tests");
    setOptions(this.comboModel, models, "model", "model", "All models");
    setOptions(this.compareA, modelFamilies, "value", "label", "Baseline family");
    setOptions(this.compareB, modelFamilies, "value", "label", "Compare family");
    this.setDefaultComparison(modelFamilies.map((row) => String(row.value)));
    setOptions(this.engine, engines, "engine", "engine", "All engines");
    setOptions(this.gpu, gpus, "gpu", "gpu", "All GPUs");
    setOptions(this.comboGpu, gpus, "gpu", "gpu", "All GPUs");

    FILTER_FIELDS.forEach((field, index) => {
      const select = this.enumFilters.get(field.key);
      if (!select) return;
      setOptions(select, enumRows[index], "value", "value", field.allLabel);
      const control = select.closest(".bench-workbench__control");
      if (control) control.hidden = select.options.length <= 1;
    });
    COMPARE_FILTER_FIELDS.forEach((field, index) => {
      const select = this.compareFilters.get(field.key);
      if (!select) return;
      setOptions(select, compareFilterRows[index], "value", "value", field.allLabel);
      const control = select.closest(".bench-workbench__control");
      if (control) control.hidden = select.options.length <= 1;
    });
  }

  bindEvents() {
    const scheduleRaw = (options = {}) => {
      if (!this.restoringRouteState) this.syncRouteState({ replace: true });
      clearTimeout(this.reloadTimer);
      this.reloadTimer = window.setTimeout(() => this.reloadRaw(options).catch((error) => this.fail(error)), 180);
    };
    const scheduleComparison = () => {
      if (!this.restoringRouteState) this.syncRouteState({ replace: true });
      clearTimeout(this.compareReloadTimer);
      this.compareReloadTimer = window.setTimeout(() => this.loadComparison().catch((error) => this.fail(error)), 180);
    };
    const scheduleCombinations = () => {
      if (!this.restoringRouteState) this.syncRouteState({ replace: true });
      clearTimeout(this.comboReloadTimer);
      this.comboReloadTimer = window.setTimeout(() => this.loadCombinationAggregates().catch((error) => this.fail(error)), 180);
    };

    window.addEventListener("hashchange", () => this.applyRouteFromLocation());
    window.addEventListener("popstate", () => this.applyRouteFromLocation());
    this.routes.forEach((link, route) => {
      link.addEventListener("click", (event) => {
        event.preventDefault();
        this.activateRoute(route, { push: true });
      });
    });

    this.search.addEventListener("input", scheduleRaw);
    this.resultState.addEventListener("change", scheduleRaw);
    this.task.addEventListener("change", scheduleRaw);
    this.model.addEventListener("change", scheduleRaw);
    this.engine.addEventListener("change", scheduleRaw);
    this.gpu.addEventListener("change", scheduleRaw);
    this.maxRam.addEventListener("input", scheduleRaw);
    this.passedOnly.addEventListener("change", scheduleRaw);
    this.enumFilters.forEach((select) => {
      select.addEventListener("change", scheduleRaw);
    });
    this.compareA.addEventListener("change", scheduleComparison);
    this.compareB.addEventListener("change", scheduleComparison);
    this.compareDimension.addEventListener("change", scheduleComparison);
    this.compareMaxRam?.addEventListener("input", scheduleComparison);
    this.compareFilters.forEach((select) => {
      select.addEventListener("change", scheduleComparison);
    });
    this.comboTask?.addEventListener("change", scheduleCombinations);
    this.comboModel?.addEventListener("change", scheduleCombinations);
    this.comboGpu?.addEventListener("change", scheduleCombinations);
    this.comboPassedOnly?.addEventListener("change", scheduleCombinations);

    this.root.querySelectorAll("[data-bw-preset]").forEach((button) => {
      button.addEventListener("click", () => {
        this.applyPreset(button.dataset.bwPreset);
      });
    });

    this.openConfig?.addEventListener("click", () => {
      this.viewer.toggleConfig();
    });
    this.viewer?.addEventListener("perspective-config-update", () => {
      if (!this.restoringRouteState) this.syncRouteState({ replace: true });
      this.saveGridConfig().catch((error) => this.fail(error));
    });

    this.clearFilters?.addEventListener("click", () => {
      this.resetFilters();
      this.syncRouteState({ replace: true });
      this.reloadRaw().catch((error) => this.fail(error));
    });

    this.root.querySelector("[data-bw-reset-view]")?.addEventListener("click", () => {
      this.gridConfig = null;
      this.gridConfigReady = false;
      this.loadGrid().catch((error) => this.fail(error));
    });

    this.comparison.addEventListener("click", (event) => {
      const button = event.target instanceof Element ? event.target.closest("button[data-bw-sort]") : null;
      if (!button) return;
      this.comparisonSort = nextSort(this.comparisonSort, button.dataset.bwSort);
      this.syncRouteState({ replace: true });
      this.loadComparison().catch((error) => this.fail(error));
    });

    this.combinations.addEventListener("click", (event) => {
      const button = event.target instanceof Element ? event.target.closest("button[data-bw-sort]") : null;
      if (!button) return;
      this.combinationSort = nextSort(this.combinationSort, button.dataset.bwSort);
      this.syncRouteState({ replace: true });
      this.loadCombinationAggregates().catch((error) => this.fail(error));
    });
  }

  routeStateFromHash() {
    const raw = window.location.hash.replace(/^#/, "");
    const [routePart, query = ""] = raw.split("?");
    const route = decodeURIComponent(routePart || "");
    return {
      route: Object.prototype.hasOwnProperty.call(ROUTES, route) ? route : "summary",
      params: new URLSearchParams(query),
    };
  }

  routeFromHash() {
    const route = this.routeStateFromHash().route;
    return Object.prototype.hasOwnProperty.call(ROUTES, route) ? route : "summary";
  }

  activateRoute(route, options = {}) {
    const nextRoute = Object.prototype.hasOwnProperty.call(ROUTES, route) ? route : "summary";
    this.activePane = nextRoute;
    this.panes.forEach((pane, key) => {
      pane.hidden = key !== nextRoute;
    });
    this.routes.forEach((link, key) => {
      if (key === nextRoute) {
        link.setAttribute("aria-current", "page");
      } else {
        link.removeAttribute("aria-current");
      }
    });
    const targetHash = options.hash || this.hashForRoute(nextRoute);
    if (options.push && window.location.hash !== targetHash) {
      window.history.pushState(null, "", targetHash);
    } else if (options.replace && window.location.hash !== targetHash) {
      window.history.replaceState(null, "", targetHash);
    }

    if (options.push) {
      requestAnimationFrame(() => this.root.scrollIntoView({ block: "start" }));
    }

    if (nextRoute === "data") {
      requestAnimationFrame(() => {
        this.viewer?.flush?.();
        if (this.taskLinkTable) this.linkVisibleTaskCells(this.taskLinkTable);
      });
    }
  }

  applyRouteFromLocation(options = {}) {
    const { route, params } = this.routeStateFromHash();
    this.restoringRouteState = true;
    try {
      this.applyStateParams(route, params);
      this.activateRoute(route, { ...options, hash: this.hashForRoute(route) });
    } finally {
      this.restoringRouteState = false;
    }

    if (!this.conn) return;
    if (route === "data") {
      this.reloadRaw().catch((error) => this.fail(error));
    } else if (route === "compare") {
      this.loadComparison().catch((error) => this.fail(error));
    } else if (route === "combinations") {
      this.loadCombinationAggregates().catch((error) => this.fail(error));
    }
  }

  applyStateParams(route, params) {
    if (route === "data") {
      this.applyRawState(params);
    } else if (route === "compare") {
      this.applyComparisonState(params);
    } else if (route === "combinations") {
      this.applyCombinationState(params);
    }
  }

  syncRouteState(options = {}) {
    const targetHash = this.hashForRoute(this.activePane);
    if (options.push && window.location.hash !== targetHash) {
      window.history.pushState(null, "", targetHash);
    } else if (window.location.hash !== targetHash) {
      window.history.replaceState(null, "", targetHash);
    }
  }

  hashForRoute(route) {
    const nextRoute = Object.prototype.hasOwnProperty.call(ROUTES, route) ? route : "summary";
    const params = this.paramsForRoute(nextRoute);
    const query = params.toString();
    return `#${encodeURIComponent(nextRoute)}${query ? `?${query}` : ""}`;
  }

  paramsForRoute(route) {
    if (route === "data") return this.rawStateParams();
    if (route === "compare") return this.comparisonStateParams();
    if (route === "combinations") return this.combinationStateParams();
    return new URLSearchParams();
  }

  applyPreset(preset) {
    this.activePreset = preset;
    this.root.querySelectorAll("[data-bw-preset]").forEach((button) => {
      button.setAttribute("aria-pressed", button.dataset.bwPreset === preset ? "true" : "false");
    });

    if (preset === "all") {
      this.passedOnly.checked = false;
      this.resultState.value = "";
    } else if (preset.startsWith("passing")) {
      this.passedOnly.checked = true;
      this.resultState.value = RESULT_STATE_PASSED;
    }

    this.syncRouteState({ replace: true });
    this.reloadRaw().catch((error) => this.fail(error));
  }

  setDefaultComparison(families) {
    const has = (value) => families.includes(value);
    if (!this.compareA.value) {
      this.compareA.value = has("claude-sonnet-4") ? "claude-sonnet-4" : families[1] || families[0] || "";
    }
    if (!this.compareB.value) {
      this.compareB.value = has("qwen3-6-27b")
        ? "qwen3-6-27b"
        : families.find((family) => family !== this.compareA.value) || "";
    }
    if (this.compareA.value === this.compareB.value) {
      this.compareB.value = families.find((family) => family !== this.compareA.value) || "";
    }
  }

  resetFilters() {
    this.activePreset = "all";
    this.root.querySelectorAll("[data-bw-preset]").forEach((button) => {
      button.setAttribute("aria-pressed", button.dataset.bwPreset === "all" ? "true" : "false");
    });
    this.search.value = "";
    this.resultState.value = "";
    this.task.value = "";
    this.model.value = "";
    this.engine.value = "";
    this.gpu.value = "";
    this.maxRam.value = "";
    this.passedOnly.checked = false;
    this.enumFilters.forEach((select) => {
      select.value = "";
    });
    if (this.gridConfig) {
      this.gridConfig = this.sanitizeGridConfig(this.gridConfig, { clearFilters: true });
    }
  }

  applyRawState(params) {
    const preset = params.get(RAW_STATE_KEYS.preset) || "all";
    const presetExists = [...this.root.querySelectorAll("[data-bw-preset]")].some((button) => button.dataset.bwPreset === preset);
    this.activePreset = presetExists ? preset : "all";
    this.root.querySelectorAll("[data-bw-preset]").forEach((button) => {
      button.setAttribute("aria-pressed", button.dataset.bwPreset === this.activePreset ? "true" : "false");
    });
    setControlValue(this.search, params.get(RAW_STATE_KEYS.search) || "");
    setControlValue(this.resultState, params.get(RAW_STATE_KEYS.resultState) || "");
    setControlValue(this.task, params.get(RAW_STATE_KEYS.task) || "");
    setControlValue(this.model, params.get(RAW_STATE_KEYS.model) || "");
    setControlValue(this.engine, params.get(RAW_STATE_KEYS.engine) || "");
    setControlValue(this.gpu, params.get(RAW_STATE_KEYS.gpu) || "");
    setControlValue(this.maxRam, params.get(RAW_STATE_KEYS.maxRam) || "");
    setCheckboxValue(this.passedOnly, params.get(RAW_STATE_KEYS.passedOnly) || "");
    this.enumFilters.forEach((select, key) => {
      setControlValue(select, params.get(`${RAW_FILTER_PREFIX}${key}`) || "");
    });
    if (this.gridConfig) {
      this.gridConfig = this.sanitizeGridConfig(this.gridConfig, { clearFilters: true });
    }
  }

  rawStateParams() {
    const params = new URLSearchParams();
    if (this.activePreset !== "all") params.set(RAW_STATE_KEYS.preset, this.activePreset);
    if (this.search.value.trim()) params.set(RAW_STATE_KEYS.search, this.search.value.trim());
    if (this.resultState.value) params.set(RAW_STATE_KEYS.resultState, this.resultState.value);
    if (this.task.value) params.set(RAW_STATE_KEYS.task, this.task.value);
    if (this.model.value) params.set(RAW_STATE_KEYS.model, this.model.value);
    if (this.engine.value) params.set(RAW_STATE_KEYS.engine, this.engine.value);
    if (this.gpu.value) params.set(RAW_STATE_KEYS.gpu, this.gpu.value);
    if (this.maxRam.value !== "") params.set(RAW_STATE_KEYS.maxRam, this.maxRam.value);
    if (this.passedOnly.checked) params.set(RAW_STATE_KEYS.passedOnly, "1");
    this.enumFilters.forEach((select, key) => {
      if (select.value) params.set(`${RAW_FILTER_PREFIX}${key}`, select.value);
    });
    return params;
  }

  applyComparisonState(params) {
    setControlValue(this.compareA, params.get(COMPARE_STATE_KEYS.a));
    setControlValue(this.compareB, params.get(COMPARE_STATE_KEYS.b));
    setControlValue(this.compareDimension, params.get(COMPARE_STATE_KEYS.dimension) || "task_category");
    setControlValue(this.compareMaxRam, params.get(COMPARE_STATE_KEYS.maxRam) || "");
    this.comparisonSort = parseSort(params.get(COMPARE_STATE_KEYS.sort), COMPARISON_SORTS, {
      key: "gap_pp",
      direction: "desc",
    });
    this.compareFilters.forEach((select, key) => {
      setControlValue(select, params.get(`${COMPARE_FILTER_PREFIX}${key}`) || "");
    });
  }

  comparisonStateParams() {
    const params = new URLSearchParams();
    if (this.compareA.value) params.set(COMPARE_STATE_KEYS.a, this.compareA.value);
    if (this.compareB.value) params.set(COMPARE_STATE_KEYS.b, this.compareB.value);
    if (this.compareDimension.value && this.compareDimension.value !== "task_category") {
      params.set(COMPARE_STATE_KEYS.dimension, this.compareDimension.value);
    }
    if (this.compareMaxRam && this.compareMaxRam.value !== "") params.set(COMPARE_STATE_KEYS.maxRam, this.compareMaxRam.value);
    if (this.comparisonSort.key !== "gap_pp" || this.comparisonSort.direction !== "desc") {
      params.set(COMPARE_STATE_KEYS.sort, encodeSort(this.comparisonSort));
    }
    this.compareFilters.forEach((select, key) => {
      if (select.value) params.set(`${COMPARE_FILTER_PREFIX}${key}`, select.value);
    });
    return params;
  }

  applyCombinationState(params) {
    setControlValue(this.comboTask, params.get(COMBINATION_STATE_KEYS.task) || "");
    setControlValue(this.comboModel, params.get(COMBINATION_STATE_KEYS.model) || "");
    setControlValue(this.comboGpu, params.get(COMBINATION_STATE_KEYS.gpu) || "");
    setCheckboxValue(this.comboPassedOnly, params.get(COMBINATION_STATE_KEYS.passedOnly) || "");
    this.combinationSort = parseSort(params.get(COMBINATION_STATE_KEYS.sort), COMBINATION_SORTS, {
      key: "n_pass",
      direction: "desc",
    });
  }

  combinationStateParams() {
    const params = new URLSearchParams();
    if (this.comboTask?.value) params.set(COMBINATION_STATE_KEYS.task, this.comboTask.value);
    if (this.comboModel?.value) params.set(COMBINATION_STATE_KEYS.model, this.comboModel.value);
    if (this.comboGpu?.value) params.set(COMBINATION_STATE_KEYS.gpu, this.comboGpu.value);
    if (this.comboPassedOnly?.checked) params.set(COMBINATION_STATE_KEYS.passedOnly, "1");
    if (this.combinationSort.key !== "n_pass" || this.combinationSort.direction !== "desc") {
      params.set(COMBINATION_STATE_KEYS.sort, encodeSort(this.combinationSort));
    }
    return params;
  }

  filterClauses(options = {}) {
    const clauses = [];
    const search = this.search.value.trim();
    const resultState = this.resultState.value;
    const task = this.task.value;
    const model = this.model.value;
    const engine = this.engine.value;
    const gpu = this.gpu.value;
    const maxRam = Number(this.maxRam.value);
    const passRequired = this.passedOnly.checked || this.activePreset.startsWith("passing");

    if (search) {
      clauses.push(`strpos(search_text, lower(${sqlString(search)})) > 0`);
    }
    if (resultState) {
      clauses.push(`result_state = ${sqlString(resultState)}`);
    }
    if (task) {
      clauses.push(`task = ${sqlString(task)}`);
    }
    if (model && !options.skipModel) {
      clauses.push(`model_display_name = ${sqlString(model)}`);
    }
    if (engine) {
      clauses.push(`engine = ${sqlString(engine)}`);
    }
    if (gpu) {
      clauses.push(`effective_gpu_model = ${sqlString(gpu)}`);
    }
    if (Number.isFinite(maxRam) && this.maxRam.value !== "") {
      clauses.push(`effective_gpu_ram_gb IS NOT NULL AND effective_gpu_ram_gb < ${maxRam}`);
    }
    if (passRequired) {
      clauses.push("result_state = 'passed'");
    }
    if (this.activePreset === "recent-failures") {
      clauses.push("result_state <> 'passed'");
    }
    for (const field of FILTER_FIELDS) {
      if (options.skipModelFamily && field.key === "model_family") continue;
      const value = this.enumFilters.get(field.key)?.value;
      if (value) {
        clauses.push(`CAST(${field.key} AS VARCHAR) = ${sqlString(value)}`);
      }
    }

    return clauses;
  }

  whereClause(options = {}) {
    const clauses = this.filterClauses(options);
    return clauses.length ? `WHERE ${clauses.join(" AND ")}` : "";
  }

  compareFilterClauses() {
    const clauses = [];
    const maxRam = Number(this.compareMaxRam?.value);
    if (Number.isFinite(maxRam) && this.compareMaxRam?.value !== "") {
      clauses.push(`effective_gpu_ram_gb IS NOT NULL AND effective_gpu_ram_gb < ${maxRam}`);
    }
    for (const field of COMPARE_FILTER_FIELDS) {
      const value = this.compareFilters.get(field.key)?.value;
      if (value) {
        clauses.push(`CAST(${field.key} AS VARCHAR) = ${sqlString(value)}`);
      }
    }
    return clauses;
  }

  compareWhereClause() {
    const clauses = this.compareFilterClauses();
    return clauses.length ? `WHERE ${clauses.join(" AND ")}` : "";
  }

  combinationFilterClauses() {
    const clauses = [];
    if (this.comboTask?.value) {
      clauses.push(`task = ${sqlString(this.comboTask.value)}`);
    }
    if (this.comboModel?.value) {
      clauses.push(`model_display_name = ${sqlString(this.comboModel.value)}`);
    }
    if (this.comboGpu?.value) {
      clauses.push(`effective_gpu_model = ${sqlString(this.comboGpu.value)}`);
    }
    if (this.comboPassedOnly?.checked) {
      clauses.push("result_state = 'passed'");
    }
    return clauses;
  }

  combinationWhereClause() {
    const clauses = this.combinationFilterClauses();
    return clauses.length ? `WHERE ${clauses.join(" AND ")}` : "";
  }

  sortClause() {
    if (this.activePreset === "recent-failures") {
      return "ORDER BY finished_at DESC NULLS LAST";
    }
    return "ORDER BY suite, task, model_display_name, effective_gpu_model, rep";
  }

  async reload() {
    await Promise.all([this.loadSummary(), this.reloadRaw(), this.loadComparison(), this.loadCombinationAggregates()]);
  }

  async reloadRaw() {
    const where = this.whereClause();
    await this.conn.query(`
      CREATE OR REPLACE TABLE workbench_cells AS
      SELECT * FROM cells_enriched
      ${where}
      ${this.sortClause()}
    `);

    await Promise.all([this.loadGrid(), this.loadMetrics()]);
    this.renderActiveFilters();
    const metricRows = this.metrics.rows?.textContent || "-";
    this.setStatus(`${metricRows} rows loaded${this.manifestText()}`);
  }

  async loadSummary() {
    const [metrics] = await queryRows(this.conn, `
      SELECT
        count(*) AS n_rows,
        CAST(count(*) FILTER (WHERE result_state = 'passed') AS DOUBLE) AS n_pass,
        CAST(count(*) FILTER (WHERE result_state = 'failed') AS DOUBLE) AS n_fail,
        CAST(count(*) FILTER (WHERE result_state = 'timeout') AS DOUBLE) AS n_timeout,
        count(DISTINCT task) FILTER (WHERE task IS NOT NULL) AS n_tasks,
        count(DISTINCT model_display_name) FILTER (WHERE model_display_name IS NOT NULL) AS n_models,
        count(DISTINCT effective_gpu_model) FILTER (WHERE effective_gpu_model IS NOT NULL AND effective_gpu_model <> 'unknown') AS n_gpus,
        CAST(sum(total_tokens) AS DOUBLE) AS token_total,
        median(wall_seconds) FILTER (WHERE wall_seconds IS NOT NULL) AS wall_p50
      FROM cells_enriched
    `);

    const passes = Number(metrics.n_pass || 0);
    const failures = Number(metrics.n_fail || 0);
    const completed = passes + failures;
    this.setSummaryMetric("rows", formatCount(metrics.n_rows));
    this.setSummaryMetric("pass_rate", completed ? `${formatNumber((passes / completed) * 100, "%")} (${formatCount(passes)})` : "-");
    this.setSummaryMetric("timeouts", formatCount(metrics.n_timeout));
    this.setSummaryMetric("tasks", formatCount(metrics.n_tasks));
    this.setSummaryMetric("models", formatCount(metrics.n_models));
    this.setSummaryMetric("gpus", formatCount(metrics.n_gpus));
    this.setSummaryMetric("tokens", formatCount(metrics.token_total));
    this.setSummaryMetric("wall_p50", formatNumber(metrics.wall_p50, "s"));

    const [taskCategories, gpus, wallByGpu, wallByModel] = await Promise.all([
      queryRows(this.conn, `
        SELECT COALESCE(NULLIF(CAST(task_category AS VARCHAR), ''), '(missing)') AS label, count(*) AS n
        FROM cells_enriched
        GROUP BY label
        ORDER BY n DESC, label
        LIMIT 8
      `),
      queryRows(this.conn, `
        SELECT COALESCE(NULLIF(CAST(effective_gpu_model AS VARCHAR), ''), '(missing)') AS label, count(*) AS n
        FROM cells_enriched
        WHERE effective_gpu_model IS NOT NULL AND effective_gpu_model <> 'unknown'
        GROUP BY label
        ORDER BY n DESC, label
        LIMIT 8
      `),
      queryRows(this.conn, `
        SELECT
          COALESCE(NULLIF(CAST(effective_gpu_model AS VARCHAR), ''), '(missing)') AS label,
          median(wall_seconds) FILTER (WHERE wall_seconds IS NOT NULL) AS value,
          count(*) AS n
        FROM cells_enriched
        WHERE effective_gpu_model IS NOT NULL AND effective_gpu_model <> 'unknown' AND wall_seconds IS NOT NULL
        GROUP BY label
        ORDER BY value DESC, n DESC
        LIMIT 12
      `),
      queryRows(this.conn, `
        SELECT
          COALESCE(NULLIF(CAST(model_family AS VARCHAR), ''), NULLIF(CAST(model_display_name AS VARCHAR), ''), '(missing)') AS label,
          median(wall_seconds) FILTER (WHERE wall_seconds IS NOT NULL) AS value,
          count(*) AS n
        FROM cells_enriched
        WHERE wall_seconds IS NOT NULL
        GROUP BY label
        ORDER BY value DESC, n DESC
        LIMIT 12
      `),
    ]);

    this.renderPieChart("task_category", taskCategories);
    this.renderPieChart("gpu", gpus);
    this.renderBarChart("wall_by_gpu", wallByGpu, "s");
    this.renderBarChart("wall_by_model", wallByModel, "s");
  }

  setSummaryMetric(key, value) {
    if (this.summaryMetrics[key]) this.summaryMetrics[key].textContent = value;
  }

  renderPieChart(key, rows) {
    const target = this.summaryCharts[key];
    if (!target) return;
    const total = rows.reduce((sum, row) => sum + Number(row.n || 0), 0);
    if (!total) {
      target.innerHTML = '<p class="bench-workbench__empty">No data.</p>';
      return;
    }

    let start = 0;
    const segments = rows.map((row, index) => {
      const count = Number(row.n || 0);
      const end = start + (count / total) * 100;
      const color = SUMMARY_COLORS[index % SUMMARY_COLORS.length];
      const segment = `${color} ${start}% ${end}%`;
      start = end;
      return segment;
    });

    const legend = rows.map((row, index) => {
      const count = Number(row.n || 0);
      const pct = total ? (count / total) * 100 : 0;
      return `
        <li>
          <span style="background:${SUMMARY_COLORS[index % SUMMARY_COLORS.length]}"></span>
          <strong>${escapeHtml(row.label)}</strong>
          <em>${formatCount(count)} / ${formatNumber(pct, "%")}</em>
        </li>
      `;
    }).join("");

    target.innerHTML = `
      <div class="bench-workbench__pie-wrap">
        <div class="bench-workbench__pie" role="img" aria-label="Distribution chart" style="background: conic-gradient(${segments.join(", ")});"></div>
        <ol>${legend}</ol>
      </div>
    `;
  }

  renderBarChart(key, rows, suffix = "") {
    const target = this.summaryCharts[key];
    if (!target) return;
    const max = Math.max(...rows.map((row) => Number(row.value || 0)), 0);
    if (!max) {
      target.innerHTML = '<p class="bench-workbench__empty">No timed rows.</p>';
      return;
    }

    const bars = rows.map((row) => {
      const value = Number(row.value || 0);
      const width = Math.max(2, (value / max) * 100);
      return `
        <div class="bench-workbench__bar-row">
          <span>${escapeHtml(row.label)}</span>
          <div><i style="width:${width}%"></i></div>
          <strong>${formatNumber(value, suffix)}</strong>
        </div>
      `;
    }).join("");
    target.innerHTML = `<div class="bench-workbench__bars">${bars}</div>`;
  }

  async loadGrid() {
    await this.saveGridConfig();
    const tableNames = await this.perspectiveClient.get_hosted_table_names();
    const tableName = tableNames.find((name) => name.endsWith(".workbench_cells")) || "memory.workbench_cells";
    const table = await this.perspectiveClient.open_table(tableName);
    const schema = await table.schema();
    const config = this.gridConfigReady
      ? this.sanitizeGridConfig(this.gridConfig, { schema })
      : this.defaultGridConfig(schema);

    await this.viewer.load(table);
    await this.viewer.restore(config);
    this.gridConfig = config;
    this.gridConfigReady = true;
    await this.viewer.flush?.();
    await this.installTaskLinks();
  }

  defaultGridConfig(schema) {
    return {
      plugin: "Datagrid",
      columns: DEFAULT_COLUMNS.filter((column) => Object.prototype.hasOwnProperty.call(schema, column)),
      sort: Object.prototype.hasOwnProperty.call(schema, "finished_at") ? [["finished_at", "desc"]] : [],
      group_by: [],
      split_by: [],
      filter: [],
      settings: false,
    };
  }

  async saveGridConfig() {
    if (!this.gridConfigReady || !this.viewer?.save) return;
    try {
      this.gridConfig = this.sanitizeGridConfig(await this.viewer.save(), { clearFilters: true });
    } catch {
      // Keep the last known-good config; Perspective can reject save() while reloading.
    }
  }

  sanitizeGridConfig(config, options = {}) {
    const next = { ...(config || {}) };
    next.plugin = next.plugin || "Datagrid";
    next.settings = Boolean(next.settings);
    if (options.clearFilters) next.filter = [];
    if (!Array.isArray(next.filter)) next.filter = [];

    if (options.schema) {
      const hasColumn = (column) => Object.prototype.hasOwnProperty.call(options.schema, column);
      next.columns = Array.isArray(next.columns) ? next.columns.filter(hasColumn) : [];
      next.sort = Array.isArray(next.sort) ? next.sort.filter(([column]) => hasColumn(column)) : [];
      next.group_by = Array.isArray(next.group_by) ? next.group_by.filter(hasColumn) : [];
      next.split_by = Array.isArray(next.split_by) ? next.split_by.filter(hasColumn) : [];
      if (!next.columns.length) return this.defaultGridConfig(options.schema);
    }

    return next;
  }

  async installTaskLinks() {
    await new Promise((resolve) => requestAnimationFrame(resolve));
    const datagrid = await this.findDatagridPlugin();
    const regularTable = datagrid?.regular_table;
    if (!regularTable) return;

    if (this.taskLinkTable !== regularTable) {
      this.taskLinkUnsubscribe?.();
      this.taskLinkTable = regularTable;
      this.taskLinkUnsubscribe = regularTable.addStyleListener(() => this.linkVisibleTaskCells(regularTable));
      regularTable.addEventListener(
        "click",
        (event) => {
          const anchor = event.target instanceof Element ? event.target.closest("a[data-terminalbench-task-link]") : null;
          if (anchor) event.stopPropagation();
        },
        true,
      );
    }

    this.linkVisibleTaskCells(regularTable);
  }

  async findDatagridPlugin() {
    try {
      const plugin = await this.viewer.getPlugin?.("Datagrid");
      if (plugin) return plugin;
    } catch {
      // Fall through to DOM lookup for older Perspective builds.
    }
    return (
      this.viewer.querySelector("perspective-viewer-datagrid") ||
      this.viewer.shadowRoot?.querySelector("perspective-viewer-datagrid") ||
      null
    );
  }

  linkVisibleTaskCells(regularTable) {
    regularTable.querySelectorAll("tbody td").forEach((cell) => {
      const metadata = regularTable.getMeta(cell);
      const columnName = metadata?.column_header?.at(-1);
      if (columnName !== "task") return;

      const task = String(metadata?.value ?? cell.textContent ?? "").trim();
      if (!task || task === "-") return;

      const existing = cell.querySelector("a[data-terminalbench-task-link]");
      if (existing && existing.textContent === task) return;

      const anchor = document.createElement("a");
      anchor.dataset.terminalbenchTaskLink = "true";
      anchor.href = taskUrl(task);
      anchor.target = "_blank";
      anchor.rel = "noopener noreferrer";
      anchor.textContent = task;
      anchor.style.color = "var(--accent-cyan)";
      anchor.style.textDecorationThickness = "1px";
      anchor.style.textUnderlineOffset = "2px";
      cell.textContent = "";
      cell.appendChild(anchor);
      cell.classList.add("bench-workbench__task-cell");
    });
  }

  async loadMetrics() {
    const [row] = await queryRows(this.conn, `
      SELECT
        count(*) AS n_rows,
        CAST(count(*) FILTER (WHERE result_state = 'passed') AS DOUBLE) AS n_pass,
        CAST(count(*) FILTER (WHERE result_state = 'failed') AS DOUBLE) AS n_fail,
        CAST(count(*) FILTER (WHERE result_state = 'timeout') AS DOUBLE) AS n_timeout,
        count(DISTINCT model_display_name) FILTER (WHERE model_display_name IS NOT NULL) AS n_models,
        count(DISTINCT effective_gpu_model) FILTER (WHERE effective_gpu_model IS NOT NULL AND effective_gpu_model <> 'unknown') AS n_gpus,
        CAST(sum(total_tokens) AS DOUBLE) AS token_total,
        sum(cost_usd) FILTER (WHERE cost_usd IS NOT NULL) AS cost_total,
        median(wall_seconds) FILTER (WHERE wall_seconds IS NOT NULL) AS wall_p50
      FROM workbench_cells
    `);

    const rows = Number(row.n_rows || 0);
    const passes = Number(row.n_pass || 0);
    const failures = Number(row.n_fail || 0);
    const completed = passes + failures;
    this.metrics.rows.textContent = formatCount(rows);
    this.metrics.pass_rate.textContent = completed ? `${formatNumber((passes / completed) * 100, "%")} (${formatCount(passes)})` : "-";
    if (this.metrics.timeouts) this.metrics.timeouts.textContent = formatCount(row.n_timeout);
    this.metrics.models.textContent = formatCount(row.n_models);
    this.metrics.gpus.textContent = formatCount(row.n_gpus);
    this.metrics.tokens.textContent = formatCount(row.token_total);
    this.metrics.cost.textContent = row.cost_total === null || row.cost_total === undefined ? "-" : USD_FORMAT.format(Number(row.cost_total));
    this.metrics.wall_p50.textContent = formatNumber(row.wall_p50, "s");
  }

  async loadComparison() {
    const familyA = this.compareA.value;
    const familyB = this.compareB.value;
    const dimension = COMPARISON_DIMENSIONS[this.compareDimension.value] || COMPARISON_DIMENSIONS.task_category;

    if (!familyA || !familyB || familyA === familyB) {
      this.comparison.innerHTML = "<p>Select two different model families.</p>";
      return;
    }

    const rows = await queryRows(this.conn, `
      WITH scoped AS (
        SELECT * FROM cells_enriched
        ${this.compareWhereClause()}
      ),
      grouped AS (
        SELECT
          COALESCE(NULLIF(CAST(${dimension.key} AS VARCHAR), ''), '(missing)') AS bucket,
          model_family,
          count(*) AS n_rows,
          count(*) FILTER (WHERE result_state IN ('passed', 'failed')) AS n_graded,
          count(*) FILTER (WHERE result_state = 'passed') AS n_pass,
          count(*) FILTER (WHERE result_state = 'failed') AS n_fail,
          count(*) FILTER (WHERE result_state = 'timeout') AS n_timeout,
          CAST(sum(total_tokens) AS DOUBLE) AS token_total,
          median(wall_seconds) FILTER (WHERE wall_seconds IS NOT NULL) AS wall_p50
        FROM scoped
        WHERE model_family IN (${sqlString(familyA)}, ${sqlString(familyB)})
        GROUP BY bucket, model_family
      ),
      pivoted AS (
        SELECT
          bucket,
          max(n_rows) FILTER (WHERE model_family = ${sqlString(familyA)}) AS a_rows,
          max(n_graded) FILTER (WHERE model_family = ${sqlString(familyA)}) AS a_graded,
          max(n_pass) FILTER (WHERE model_family = ${sqlString(familyA)}) AS a_pass,
          max(n_fail) FILTER (WHERE model_family = ${sqlString(familyA)}) AS a_fail,
          max(n_timeout) FILTER (WHERE model_family = ${sqlString(familyA)}) AS a_timeout,
          max(token_total) FILTER (WHERE model_family = ${sqlString(familyA)}) AS a_tokens,
          max(wall_p50) FILTER (WHERE model_family = ${sqlString(familyA)}) AS a_wall_p50,
          max(n_rows) FILTER (WHERE model_family = ${sqlString(familyB)}) AS b_rows,
          max(n_graded) FILTER (WHERE model_family = ${sqlString(familyB)}) AS b_graded,
          max(n_pass) FILTER (WHERE model_family = ${sqlString(familyB)}) AS b_pass,
          max(n_fail) FILTER (WHERE model_family = ${sqlString(familyB)}) AS b_fail,
          max(n_timeout) FILTER (WHERE model_family = ${sqlString(familyB)}) AS b_timeout,
          max(token_total) FILTER (WHERE model_family = ${sqlString(familyB)}) AS b_tokens,
          max(wall_p50) FILTER (WHERE model_family = ${sqlString(familyB)}) AS b_wall_p50
        FROM grouped
        GROUP BY bucket
      )
      SELECT
        *,
        CASE WHEN a_graded > 0 THEN CAST(a_pass AS DOUBLE) / CAST(a_graded AS DOUBLE) ELSE NULL END AS a_pass_rate,
        CASE WHEN b_graded > 0 THEN CAST(b_pass AS DOUBLE) / CAST(b_graded AS DOUBLE) ELSE NULL END AS b_pass_rate,
        CASE
          WHEN a_graded > 0 AND b_graded > 0 THEN
            100 * (
              CAST(b_pass AS DOUBLE) / CAST(b_graded AS DOUBLE)
              - CAST(a_pass AS DOUBLE) / CAST(a_graded AS DOUBLE)
            )
          ELSE NULL
      END AS gap_pp
      FROM pivoted
      WHERE COALESCE(a_rows, 0) > 0 OR COALESCE(b_rows, 0) > 0
    `);

    this.renderComparison(sortRows(rows, this.comparisonSort, COMPARISON_SORTS), dimension);
  }

  renderComparison(rows, dimension) {
    if (rows.length === 0) {
      this.comparison.innerHTML = "<p>No comparison rows match the current filters.</p>";
      return;
    }

    const labelA = this.selectedOptionLabel(this.compareA);
    const labelB = this.selectedOptionLabel(this.compareB);
    const headers = [
      [dimension.label, "bucket"],
      [`${labelA} pass`, "a_pass_rate"],
      [`${labelB} pass`, "b_pass_rate"],
      ["Gap", "gap_pp"],
      [`${labelA} rows`, "a_rows"],
      [`${labelB} rows`, "b_rows"],
      [`${labelA} fails`, "a_fail"],
      [`${labelB} fails`, "b_fail"],
      [`${labelA} timeouts`, "a_timeout"],
      [`${labelB} timeouts`, "b_timeout"],
      [`${labelA} tokens`, "a_tokens"],
      [`${labelB} tokens`, "b_tokens"],
      [`${labelA} p50`, "a_wall_p50"],
      [`${labelB} p50`, "b_wall_p50"],
    ];

    const htmlRows = rows.map((row) => {
      const gap = Number(row.gap_pp);
      const gapClass = Number.isFinite(gap)
        ? gap > 0
          ? "bench-workbench__gap-positive"
          : gap < 0
            ? "bench-workbench__gap-negative"
            : ""
        : "";
      return `
        <tr>
          <td>${this.comparisonBucket(row.bucket, dimension.key)}</td>
          <td>${formatPercent(row.a_pass_rate)}</td>
          <td>${formatPercent(row.b_pass_rate)}</td>
          <td class="${gapClass}">${formatGap(row.gap_pp)}</td>
          <td>${formatCount(row.a_rows)}</td>
          <td>${formatCount(row.b_rows)}</td>
          <td>${formatCount(row.a_fail)}</td>
          <td>${formatCount(row.b_fail)}</td>
          <td>${formatCount(row.a_timeout)}</td>
          <td>${formatCount(row.b_timeout)}</td>
          <td>${formatCount(row.a_tokens)}</td>
          <td>${formatCount(row.b_tokens)}</td>
          <td>${formatNumber(row.a_wall_p50, "s")}</td>
          <td>${formatNumber(row.b_wall_p50, "s")}</td>
        </tr>
      `;
    }).join("");

    this.comparison.innerHTML = `
      <table>
        <thead>
          <tr>${headers.map(([label, key]) => sortHeader(label, key, this.comparisonSort)).join("")}</tr>
        </thead>
        <tbody>${htmlRows}</tbody>
      </table>
    `;
  }

  comparisonBucket(bucket, dimensionKey) {
    if (dimensionKey === "task" && bucket && bucket !== "(missing)") {
      const task = escapeHtml(bucket);
      return `<a href="${escapeHtml(taskUrl(bucket))}" target="_blank" rel="noopener noreferrer">${task}</a>`;
    }
    return escapeHtml(bucket);
  }

  selectedOptionLabel(select) {
    const selected = select.selectedOptions?.[0];
    return selected?.dataset.label || selected?.textContent || select.value;
  }

  async loadCombinationAggregates() {
    const rows = await queryRows(this.conn, `
      SELECT
        task,
        model_display_name,
        model_quant,
        quant_display,
        kv_cache_quant,
        k_quant,
        v_quant,
        runtime_mtp_enabled,
        engine,
        effective_gpu_model AS gpu,
        effective_gpu_ram_gb AS gpu_ram_gb,
        min(terminalbench_task_url) AS terminalbench_task_url,
        count(*) AS n_rows,
        CAST(count(*) FILTER (WHERE result_state = 'passed') AS DOUBLE) AS n_pass,
        CAST(count(*) FILTER (WHERE result_state = 'failed') AS DOUBLE) AS n_fail,
        CAST(count(*) FILTER (WHERE result_state = 'timeout') AS DOUBLE) AS n_timeout,
        CASE
          WHEN count(*) FILTER (WHERE result_state IN ('passed', 'failed')) = 0 THEN NULL
          ELSE CAST(count(*) FILTER (WHERE result_state = 'passed') AS DOUBLE)
            / CAST(count(*) FILTER (WHERE result_state IN ('passed', 'failed')) AS DOUBLE)
        END AS pass_rate,
        CAST(sum(total_tokens) AS DOUBLE) AS token_total,
        sum(cost_usd) FILTER (WHERE cost_usd IS NOT NULL) AS cost_total,
        median(wall_seconds) FILTER (WHERE wall_seconds IS NOT NULL) AS wall_p50
      FROM cells_enriched
      ${this.combinationWhereClause()}
      GROUP BY
        task,
        model_display_name,
        model_quant,
        quant_display,
        kv_cache_quant,
        k_quant,
        v_quant,
        runtime_mtp_enabled,
        engine,
        effective_gpu_model,
        effective_gpu_ram_gb
    `);
    const sortedRows = sortRows(rows, this.combinationSort, COMBINATION_SORTS);

    if (sortedRows.length === 0) {
      this.combinations.innerHTML = "<p>No combinations match the current filters.</p>";
      return;
    }

    const headers = [
      ["Test", "task"],
      ["Model", "model_display_name"],
      ["Quant", "model_quant"],
      ["KV", "kv_cache_quant"],
      ["MTP", "runtime_mtp_enabled"],
      ["Engine", "engine"],
      ["GPU", "gpu"],
      ["RAM", "gpu_ram_gb"],
      ["Rows", "n_rows"],
      ["Passes", "n_pass"],
      ["Fails", "n_fail"],
      ["Timeouts", "n_timeout"],
      ["Pass rate", "pass_rate"],
      ["Tokens", "token_total"],
      ["Cost", "cost_total"],
      ["Wall p50", "wall_p50"],
    ];
    const htmlRows = sortedRows.map((row) => `
      <tr>
        <td>${taskAnchor(row)}</td>
        <td>${escapeHtml(row.model_display_name)}</td>
        <td>${escapeHtml(row.model_quant || row.quant_display)}</td>
        <td>${escapeHtml([row.kv_cache_quant, row.k_quant, row.v_quant].filter(Boolean).join(" / ") || "-")}</td>
        <td>${row.runtime_mtp_enabled === null || row.runtime_mtp_enabled === undefined ? "-" : escapeHtml(row.runtime_mtp_enabled)}</td>
        <td>${escapeHtml(row.engine)}</td>
        <td>${escapeHtml(row.gpu)}</td>
        <td>${formatNumber(row.gpu_ram_gb, " GB")}</td>
        <td>${formatCount(row.n_rows)}</td>
        <td>${formatCount(row.n_pass)}</td>
        <td>${formatCount(row.n_fail)}</td>
        <td>${formatCount(row.n_timeout)}</td>
        <td>${formatNumber(Number(row.pass_rate || 0) * 100, "%")}</td>
        <td>${formatCount(row.token_total)}</td>
        <td>${row.cost_total === null || row.cost_total === undefined ? "-" : USD_FORMAT.format(Number(row.cost_total))}</td>
        <td>${formatNumber(row.wall_p50, "s")}</td>
      </tr>
    `).join("");

    this.combinations.innerHTML = `
      <table>
        <thead>
          <tr>${headers.map(([label, key]) => sortHeader(label, key, this.combinationSort)).join("")}</tr>
        </thead>
        <tbody>${htmlRows}</tbody>
      </table>
    `;
  }

  renderActiveFilters() {
    if (!this.activeFilters) return;
    const labels = this.activeFilterLabels();
    this.activeFilters.hidden = labels.length === 0;
    this.activeFilters.innerHTML = labels.map((label) => `<span>${escapeHtml(label)}</span>`).join("");
  }

  activeFilterLabels() {
    const labels = [];
    const preset = this.root.querySelector(`[data-bw-preset="${this.activePreset}"]`)?.textContent?.trim();
    if (this.activePreset !== "all" && preset) labels.push(`View: ${preset}`);
    if (this.search.value.trim()) labels.push(`Search: ${this.search.value.trim()}`);
    this.pushSelectFilter(labels, "Outcome", this.resultState);
    this.pushSelectFilter(labels, "Task", this.task);
    this.pushSelectFilter(labels, "Model", this.model);
    this.pushSelectFilter(labels, "Engine", this.engine);
    this.pushSelectFilter(labels, "GPU", this.gpu);
    if (this.maxRam.value !== "") labels.push(`Max GPU RAM: ${this.maxRam.value} GB`);
    if (this.passedOnly.checked) labels.push("Passed only");
    for (const field of FILTER_FIELDS) {
      this.pushSelectFilter(labels, field.label, this.enumFilters.get(field.key));
    }
    return labels;
  }

  pushSelectFilter(labels, label, select) {
    if (!select?.value) return;
    const selected = select.selectedOptions?.[0];
    labels.push(`${label}: ${selected?.dataset.label || selected?.textContent || select.value}`);
  }

  manifestText() {
    const generated = this.manifest?.generated_at;
    const artifact = this.manifest?.artifacts?.find((item) => item.kind === "cell_rows" && item.format === "parquet");
    if (!generated || !artifact?.rows) return "";
    const excluded = this.manifest?.source?.n_excluded;
    const suffix = excluded ? `; ${formatCount(excluded)} excluded` : "";
    return ` from ${formatCount(artifact.rows)} valid cells at ${generated}${suffix}`;
  }

  setStatus(message) {
    this.status.textContent = message;
  }

  fail(error) {
    console.error(error);
    this.root.classList.add("bench-workbench--error");
    this.setStatus(`Workbench failed: ${error.message || error}`);
  }
}

function taskAnchor(row) {
  const task = escapeHtml(row.task);
  if (!row.terminalbench_task_url) return task;
  return `<a href="${escapeHtml(row.terminalbench_task_url)}" target="_blank" rel="noopener noreferrer">${task}</a>`;
}

function escapeHtml(value) {
  if (value === null || value === undefined || value === "") return "-";
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

ready(() => {
  document.querySelectorAll("[data-benchmark-workbench]").forEach((root) => {
    const workbench = new BenchmarkWorkbench(root);
    workbench.init().catch((error) => workbench.fail(error));
  });
});
