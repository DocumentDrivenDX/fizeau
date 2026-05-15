import * as duckdb from "@duckdb/duckdb-wasm";
import "@perspective-dev/viewer/dist/esm/perspective-viewer.inline.js";
import "@perspective-dev/viewer-datagrid/dist/esm/perspective-viewer-datagrid.js";
import { DuckDBHandler } from "@perspective-dev/client/dist/esm/virtual_servers/duckdb.js";

const DEFAULT_COLUMNS = [
  "suite",
  "task",
  "task_subsets",
  "result_state",
  "passed",
  "grader_passed",
  "final_status",
  "invalid_class",
  "harness",
  "harness_label",
  "provider_type",
  "provider_surface",
  "model_display_name",
  "model",
  "model_quant",
  "quant_display",
  "weight_bits",
  "kv_cache_quant",
  "k_quant",
  "v_quant",
  "runtime_mtp_enabled",
  "engine",
  "engine_version",
  "gpu_model",
  "gpu_ram_gb",
  "hardware_vram_gb",
  "machine",
  "rep",
  "turns",
  "input_tokens",
  "output_tokens",
  "reasoning_tokens",
  "total_tokens",
  "cost_usd",
  "wall_seconds",
  "started_at",
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
  { key: "lane_label", label: "Lane", allLabel: "All lanes" },
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

const NUMBER_FORMAT = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 });
const INTEGER_FORMAT = new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 });
const USD_FORMAT = new Intl.NumberFormat(undefined, {
  style: "currency",
  currency: "USD",
  maximumFractionDigits: 2,
});
const TERMINAL_BENCH_TASK_BASE = "https://www.tbench.ai/registry/terminal-bench-core/head/";
const RESULT_STATE_PASSED = "passed";

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

function setOptions(select, rows, valueKey, labelKey, allLabel) {
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
    this.comparison = root.querySelector("[data-bw-comparison]");
    this.combinations = root.querySelector("[data-bw-combinations]");
    this.openConfig = root.querySelector("[data-bw-open-config]");
    this.enumFilters = new Map(
      [...root.querySelectorAll("[data-bw-filter-field]")].map((select) => [select.dataset.bwFilterField, select]),
    );
    this.metrics = Object.fromEntries(
      [...root.querySelectorAll("[data-bw-metric]")].map((el) => [el.dataset.bwMetric, el]),
    );
    this.cellsUrl = absolutize(root.dataset.cellsUrl || "/data/cells.parquet");
    this.manifestUrl = absolutize(root.dataset.manifestUrl || "/data/benchmark-data-manifest.json");
    this.duckdbBase = absolutize(root.dataset.duckdbBase || "/vendor/duckdb/");
    this.activePreset = "all";
    this.reloadTimer = null;
    this.taskLinkUnsubscribe = null;
    this.taskLinkTable = null;
  }

  async init() {
    if (!window.Worker || !window.WebAssembly) {
      throw new Error("This browser does not support the Worker and WebAssembly APIs required by the benchmark workbench.");
    }

    await customElements.whenDefined("perspective-viewer");
    await customElements.whenDefined("perspective-viewer-datagrid");

    this.setStatus("Loading artifact manifest...");
    await this.loadManifest();

    this.setStatus("Starting DuckDB-WASM...");
    await this.initDuckDB();

    this.setStatus("Registering Parquet data...");
    await this.initViews();

    this.setStatus("Preparing controls...");
    await this.populateControls();
    this.bindEvents();

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

    await this.conn.query(`
      CREATE OR REPLACE VIEW cells_enriched AS
      SELECT
        *,
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
          COALESCE(CAST(lane_label AS VARCHAR), ''),
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
      FROM cells_raw
    `);

    await this.conn.query("CREATE OR REPLACE TABLE workbench_cells AS SELECT * FROM cells_enriched");

    const mod = customElements.get("perspective-viewer").__wasm_module__;
    this.perspectiveClient = await makePerspectiveClient(new DuckDBHandler(this.conn, mod), mod);
  }

  async populateControls() {
    const enumQueries = FILTER_FIELDS.map((field) => queryRows(this.conn, `
      SELECT CAST(${field.key} AS VARCHAR) AS value, count(*) AS n
      FROM cells_enriched
      WHERE ${field.key} IS NOT NULL AND CAST(${field.key} AS VARCHAR) <> ''
      GROUP BY ${field.key}
      ORDER BY n DESC, value
    `));

    const [resultStates, tasks, models, modelFamilies, engines, gpus, ...enumRows] = await Promise.all([
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
    ]);

    setOptions(this.resultState, resultStates, "result_state", "result_state", "All outcomes");
    setOptions(this.task, tasks, "task", "task", "All tests");
    setOptions(this.model, models, "model", "model", "All models");
    setOptions(this.compareA, modelFamilies, "value", "label", "Baseline family");
    setOptions(this.compareB, modelFamilies, "value", "label", "Compare family");
    this.setDefaultComparison(modelFamilies.map((row) => String(row.value)));
    setOptions(this.engine, engines, "engine", "engine", "All engines");
    setOptions(this.gpu, gpus, "gpu", "gpu", "All GPUs");

    FILTER_FIELDS.forEach((field, index) => {
      const select = this.enumFilters.get(field.key);
      if (!select) return;
      setOptions(select, enumRows[index], "value", "value", field.allLabel);
      const control = select.closest(".bench-workbench__control");
      if (control) control.hidden = select.options.length <= 1;
    });
  }

  bindEvents() {
    const schedule = () => {
      clearTimeout(this.reloadTimer);
      this.reloadTimer = window.setTimeout(() => this.reload().catch((error) => this.fail(error)), 180);
    };

    this.search.addEventListener("input", schedule);
    this.resultState.addEventListener("change", schedule);
    this.task.addEventListener("change", schedule);
    this.model.addEventListener("change", schedule);
    this.engine.addEventListener("change", schedule);
    this.gpu.addEventListener("change", schedule);
    this.maxRam.addEventListener("input", schedule);
    this.passedOnly.addEventListener("change", schedule);
    this.enumFilters.forEach((select) => {
      select.addEventListener("change", schedule);
    });
    this.compareA.addEventListener("change", schedule);
    this.compareB.addEventListener("change", schedule);
    this.compareDimension.addEventListener("change", schedule);

    this.root.querySelectorAll("[data-bw-preset]").forEach((button) => {
      button.addEventListener("click", () => {
        this.applyPreset(button.dataset.bwPreset);
      });
    });

    this.openConfig.addEventListener("click", () => {
      this.viewer.toggleConfig();
    });

    this.clearFilters?.addEventListener("click", () => {
      this.resetFilters();
      this.reload().catch((error) => this.fail(error));
    });
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

    this.reload().catch((error) => this.fail(error));
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

  sortClause() {
    if (this.activePreset === "recent-failures") {
      return "ORDER BY finished_at DESC NULLS LAST";
    }
    return "ORDER BY suite, task, model_display_name, effective_gpu_model, rep";
  }

  async reload() {
    const where = this.whereClause();
    await this.conn.query(`
      CREATE OR REPLACE TABLE workbench_cells AS
      SELECT * FROM cells_enriched
      ${where}
      ${this.sortClause()}
    `);

    await Promise.all([this.loadGrid(), this.loadMetrics(), this.loadComparison(), this.loadCombinationAggregates()]);
    this.renderActiveFilters();
    const metricRows = this.metrics.rows?.textContent || "-";
    this.setStatus(`${metricRows} rows loaded${this.manifestText()}`);
  }

  async loadGrid() {
    const tableNames = await this.perspectiveClient.get_hosted_table_names();
    const tableName = tableNames.find((name) => name.endsWith(".workbench_cells")) || "memory.workbench_cells";
    const table = await this.perspectiveClient.open_table(tableName);
    const schema = await table.schema();
    const columns = DEFAULT_COLUMNS.filter((column) => Object.prototype.hasOwnProperty.call(schema, column));

    await this.viewer.load(table);
    await this.viewer.restore({
      plugin: "Datagrid",
      columns,
      sort: [["finished_at", "desc"]],
      group_by: [],
      split_by: [],
      filter: [],
      settings: false,
    });
    await this.viewer.flush?.();
    await this.installTaskLinks();
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
        ${this.whereClause({ skipModel: true, skipModelFamily: true })}
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
      ORDER BY
        abs(COALESCE(gap_pp, 0)) DESC,
        COALESCE(a_rows, 0) + COALESCE(b_rows, 0) DESC,
        bucket
      LIMIT 60
    `);

    this.renderComparison(rows, dimension);
  }

  renderComparison(rows, dimension) {
    if (rows.length === 0) {
      this.comparison.innerHTML = "<p>No comparison rows match the current filters.</p>";
      return;
    }

    const labelA = this.selectedOptionLabel(this.compareA);
    const labelB = this.selectedOptionLabel(this.compareB);
    const headers = [
      dimension.label,
      `${labelA} pass`,
      `${labelB} pass`,
      "Gap",
      `${labelA} rows`,
      `${labelB} rows`,
      `${labelA} fails`,
      `${labelB} fails`,
      `${labelA} timeouts`,
      `${labelB} timeouts`,
      `${labelA} tokens`,
      `${labelB} tokens`,
      `${labelA} p50`,
      `${labelB} p50`,
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
          <tr>${headers.map((header) => `<th>${escapeHtml(header)}</th>`).join("")}</tr>
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
      FROM workbench_cells
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
      ORDER BY n_pass DESC, pass_rate DESC, n_rows DESC, task
      LIMIT 80
    `);

    if (rows.length === 0) {
      this.combinations.innerHTML = "<p>No combinations match the current filters.</p>";
      return;
    }

    const headers = ["Test", "Model", "Quant", "KV", "MTP", "Engine", "GPU", "RAM", "Rows", "Passes", "Fails", "Timeouts", "Pass rate", "Tokens", "Cost", "Wall p50"];
    const htmlRows = rows.map((row) => `
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
          <tr>${headers.map((header) => `<th>${header}</th>`).join("")}</tr>
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
