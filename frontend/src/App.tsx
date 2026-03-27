import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";

import { apiDelete, apiGet, apiPatch, apiPost } from "./api";
import type {
  ChannelModelListResponse,
  ChannelModelMapResponse,
  ChannelModelUpsertRequest,
  ProbeForm,
  ProbeListResponse,
  ProbeManualUpdateRequest,
  ProbeRecord,
  ProbeResponse,
  RankingResponse,
} from "./types";
import "./App.css";

const defaultChannels: Record<string, string[]> = {
  cc: ["claude-sonnet-4.6", "claude-opus-4.6"],
  codex: ["gpt-5.4", "gpt-5.3-codex"],
};

const initialProbeForm: ProbeForm = {
  stationName: "",
  groupName: "",
  baseUrl: "",
  apiKey: "",
  claimedChannel: "cc",
  expectedModelFamily: "claude-sonnet-4.6",
};

type RouteType = "home" | "admin";

function App() {
  const [route, setRoute] = useState<RouteType>(parseRoute(window.location.hash));

  useEffect(() => {
    function onHashChange() {
      setRoute(parseRoute(window.location.hash));
    }

    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  return (
    <main className="shell">
      <header className="top-nav">
        <a className={route === "home" ? "active" : ""} href="#/">
          探测台
        </a>
        <a className={route === "admin" ? "active" : ""} href="#/admin">
          后台管理
        </a>
      </header>

      {route === "admin" ? <AdminPage /> : <ProbePage />}
    </main>
  );
}

function ProbePage() {
  const [form, setForm] = useState<ProbeForm>(initialProbeForm);
  const [channelModels, setChannelModels] = useState<Record<string, string[]>>(defaultChannels);
  const [result, setResult] = useState<ProbeResponse | null>(null);
  const [recent, setRecent] = useState<ProbeListResponse["items"]>([]);
  const [stationRanking, setStationRanking] = useState<RankingResponse>({ red: [], black: [] });
  const [groupRanking, setGroupRanking] = useState<RankingResponse>({ red: [], black: [] });
  const [loadingDashboard, setLoadingDashboard] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const channelOptions = useMemo(() => Object.keys(channelModels), [channelModels]);
  const expectedModels = channelModels[form.claimedChannel] ?? [];

  useEffect(() => {
    void loadDashboard();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function loadDashboard() {
    setLoadingDashboard(true);
    setError(null);

    try {
      const [recentResponse, stationResponse, groupResponse, channelResponse] = await Promise.all([
        apiGet<ProbeListResponse>("/api/probes?limit=8"),
        apiGet<RankingResponse>("/api/rankings/stations?limit=8"),
        apiGet<RankingResponse>("/api/rankings/groups?limit=8"),
        apiGet<ChannelModelMapResponse>("/api/channel-models"),
      ]);

      const normalizedMap = normalizeChannelModelMap(channelResponse.channels);
      setChannelModels(Object.keys(normalizedMap).length > 0 ? normalizedMap : defaultChannels);
      setRecent(recentResponse.items);
      setStationRanking(stationResponse);
      setGroupRanking(groupResponse);

      setForm((current) => {
        const nextChannel = pickChannel(normalizedMap, current.claimedChannel);
        const nextExpected = pickModel(normalizedMap, nextChannel, current.expectedModelFamily);
        return { ...current, claimedChannel: nextChannel, expectedModelFamily: nextExpected };
      });
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "加载失败");
    } finally {
      setLoadingDashboard(false);
    }
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError(null);

    try {
      const payload = await apiPost<ProbeResponse>("/api/probes", form);
      setResult(payload);
      await loadDashboard();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "提交失败");
    } finally {
      setSubmitting(false);
    }
  }

  function updateField<Key extends keyof ProbeForm>(key: Key, value: ProbeForm[Key]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  function updateChannel(channel: string) {
    const nextChannel = pickChannel(channelModels, channel);
    const nextExpected = pickModel(channelModels, nextChannel, "");
    setForm((current) => ({ ...current, claimedChannel: nextChannel, expectedModelFamily: nextExpected }));
  }

  return (
    <>
      <section className="hero">
        <div className="hero-copy">
          <p className="eyebrow">Model Probe</p>
          <h1>中转站红黑榜</h1>
          <p className="hero-text">重点检查模型是否被 kiro、反重力、glm 等渠道冒充，榜单按探测样本自动更新。</p>
        </div>
        <div className="hero-stats">
          <article>
            <strong>{recent.length}</strong>
            <span>最近样本</span>
          </article>
          <article>
            <strong>{stationRanking.red.length}</strong>
            <span>站点上榜</span>
          </article>
          <article>
            <strong>{groupRanking.red.length}</strong>
            <span>分组上榜</span>
          </article>
        </div>
      </section>

      <section className="layout">
        <form className="panel form-panel" onSubmit={handleSubmit}>
          <div className="panel-heading">
            <h2>发起探测</h2>
            <p>白名单来自后台渠道配置，结果只看模型真伪与冒充风险。</p>
          </div>

          <label>
            站点名
            <input value={form.stationName} onChange={(event) => updateField("stationName", event.target.value)} placeholder="例如：某某中转站" />
          </label>
          <label>
            分组名
            <input value={form.groupName} onChange={(event) => updateField("groupName", event.target.value)} placeholder="例如：TG-群组-1" />
          </label>
          <label>
            Base URL
            <input value={form.baseUrl} onChange={(event) => updateField("baseUrl", event.target.value)} placeholder="https://example-proxy.com" />
          </label>
          <label>
            API Key
            <input value={form.apiKey} onChange={(event) => updateField("apiKey", event.target.value)} placeholder="sk-..." />
          </label>

          <div className="split">
            <label>
              宣称渠道
              <select value={form.claimedChannel} onChange={(event) => updateChannel(event.target.value)}>
                {channelOptions.map((channel) => (
                  <option key={channel} value={channel}>
                    {channel}
                  </option>
                ))}
              </select>
            </label>

            <label>
              期望模型
              <select value={form.expectedModelFamily} onChange={(event) => updateField("expectedModelFamily", event.target.value)}>
                {expectedModels.map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
            </label>
          </div>

          <button className="primary-button" disabled={submitting || expectedModels.length === 0} type="submit">
            {submitting ? "检测中..." : "开始检测"}
          </button>

          {error ? <p className="error-box">{error}</p> : null}
        </form>

        <section className="panel result-panel">
          <div className="panel-heading">
            <h2>检测结果</h2>
            <p>如果命中异常家族，系统会直接进入黑榜风险逻辑。</p>
          </div>

          {result ? (
            <div className="result-stack">
              <div className={`verdict-chip ${result.probe.verdict}`}>
                <span>{result.probe.verdict}</span>
                <strong>{result.probe.trustScore} / 100</strong>
              </div>
              <div className="result-grid">
                <article>
                  <span>宣称渠道</span>
                  <strong>{result.probe.claimedChannel ?? "未设置"}</strong>
                </article>
                <article>
                  <span>期望模型</span>
                  <strong>{result.probe.expectedModelFamily ?? "未设置"}</strong>
                </article>
                <article>
                  <span>主模型家族</span>
                  <strong>{result.probe.primaryFamily ?? "未识别"}</strong>
                </article>
                <article>
                  <span>接口状态</span>
                  <strong>{result.probe.httpStatus ?? "无响应"}</strong>
                </article>
              </div>

              <div className="list-block">
                <h3>可疑原因</h3>
                {result.probe.suspicionReasons.length > 0 ? (
                  <ul>
                    {result.probe.suspicionReasons.map((item) => (
                      <li key={item}>{item}</li>
                    ))}
                  </ul>
                ) : (
                  <p>当前没有命中明显风险规则。</p>
                )}
              </div>

              <div className="meta-card">
                <p>模型 ID：{result.probe.modelIds.join(", ") || "无"}</p>
                <p>命中端点：{result.probe.detectedEndpoint ?? "未命中"}</p>
                <p>API Key：{result.probe.apiKeyMasked}</p>
              </div>
            </div>
          ) : (
            <div className="empty-state">
              <p>先提交一次探测，结果会显示在这里。</p>
            </div>
          )}
        </section>
      </section>

      <section className="boards">
        <article className="panel">
          <div className="panel-heading">
            <h2>站点红黑榜</h2>
            <p>{loadingDashboard ? "正在加载..." : "分数高且稳定的站点优先进入红榜。"}</p>
          </div>
          <div className="board-grid">
            <RankingColumn title="红榜" tone="good" items={stationRanking.red} />
            <RankingColumn title="黑榜" tone="bad" items={stationRanking.black} />
          </div>
        </article>

        <article className="panel">
          <div className="panel-heading">
            <h2>分组红黑榜</h2>
            <p>更适合追踪某群组长期分享渠道是否稳定。</p>
          </div>
          <div className="board-grid">
            <RankingColumn title="红榜" tone="good" items={groupRanking.red} />
            <RankingColumn title="黑榜" tone="bad" items={groupRanking.black} />
          </div>
        </article>
      </section>

      <section className="panel">
        <div className="panel-heading">
          <h2>最近探测</h2>
          <p>后台修正会同步更新榜单。</p>
        </div>
        <div className="recent-list">
          {recent.map((item) => (
            <article className="recent-card" key={item.id}>
              <div>
                <strong>{item.stationName}</strong>
                <p>{item.baseUrl}</p>
              </div>
              <div className={`mini-verdict ${item.verdict}`}>{item.verdict}</div>
              <div>
                <span>{item.primaryFamily ?? "未识别"}</span>
                <p>{item.groupName ?? "未分组"}</p>
              </div>
            </article>
          ))}
          {recent.length === 0 && !loadingDashboard ? <p>还没有探测数据。</p> : null}
        </div>
      </section>
    </>
  );
}

function AdminPage() {
  const [channelRows, setChannelRows] = useState<ChannelModelListResponse["items"]>([]);
  const [recent, setRecent] = useState<ProbeRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [savingChannel, setSavingChannel] = useState(false);
  const [savingProbe, setSavingProbe] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [selectedProbe, setSelectedProbe] = useState<ProbeRecord | null>(null);
  const [channelForm, setChannelForm] = useState<ChannelModelUpsertRequest>({ channelName: "cc", modelId: "claude-sonnet-4.6", isEnabled: true });
  const [probeForm, setProbeForm] = useState<ProbeManualUpdateRequest>({
    claimedChannel: "cc",
    expectedModelFamily: "claude-sonnet-4.6",
    status: "invalid_response",
    verdict: "needs_review",
    trustScore: 60,
    primaryFamily: "",
    modelIds: [],
    suspicionReasons: [],
    notes: [],
  });
  const [modelIdsText, setModelIDsText] = useState("");
  const [suspicionText, setSuspicionText] = useState("");
  const [notesText, setNotesText] = useState("");

  const enabledChannelMap = useMemo(() => {
    const map: Record<string, string[]> = {};
    for (const row of channelRows) {
      if (!row.isEnabled) continue;
      if (!map[row.channelName]) {
        map[row.channelName] = [];
      }
      map[row.channelName].push(row.modelId);
    }
    return normalizeChannelModelMap(map);
  }, [channelRows]);

  const adminChannels = useMemo(() => Object.keys(enabledChannelMap), [enabledChannelMap]);

  useEffect(() => {
    void loadAdminData();
  }, []);

  async function loadAdminData() {
    setLoading(true);
    setError(null);

    try {
      const [channelResponse, probeResponse] = await Promise.all([
        apiGet<ChannelModelListResponse>("/api/admin/channel-models"),
        apiGet<ProbeListResponse>("/api/probes?limit=30"),
      ]);

      setChannelRows(channelResponse.items);
      setRecent(probeResponse.items);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }

  async function submitChannelForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSavingChannel(true);
    setError(null);
    setNotice(null);

    try {
      await apiPost<{ item: unknown }>("/api/admin/channel-models", {
        channelName: channelForm.channelName.trim().toLowerCase(),
        modelId: channelForm.modelId.trim().toLowerCase(),
        isEnabled: channelForm.isEnabled,
      });
      setNotice("渠道模型配置已保存");
      await loadAdminData();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "保存失败");
    } finally {
      setSavingChannel(false);
    }
  }

  async function toggleChannelModel(channelName: string, modelId: string, isEnabled: boolean) {
    setSavingChannel(true);
    setError(null);
    setNotice(null);
    try {
      await apiPost<{ item: unknown }>("/api/admin/channel-models", {
        channelName,
        modelId,
        isEnabled: !isEnabled,
      });
      await loadAdminData();
      setNotice("状态已更新");
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "更新失败");
    } finally {
      setSavingChannel(false);
    }
  }

  async function removeChannelModel(id: number) {
    setSavingChannel(true);
    setError(null);
    setNotice(null);
    try {
      await apiDelete<{ ok: boolean }>(`/api/admin/channel-models/${id}`);
      await loadAdminData();
      setNotice("渠道模型配置已删除");
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "删除失败");
    } finally {
      setSavingChannel(false);
    }
  }

  function startEditProbe(item: ProbeRecord) {
    const preferredChannel = pickChannel(enabledChannelMap, item.claimedChannel ?? "");
    const preferredModel = pickModel(enabledChannelMap, preferredChannel, item.expectedModelFamily ?? "");
    const status = normalizeStatus(item.status);
    const verdict = normalizeVerdict(item.verdict);

    setSelectedProbe(item);
    setProbeForm({
      claimedChannel: preferredChannel,
      expectedModelFamily: preferredModel,
      status,
      verdict,
      trustScore: item.trustScore,
      primaryFamily: item.primaryFamily ?? "",
      modelIds: item.modelIds,
      suspicionReasons: item.suspicionReasons,
      notes: item.notes,
    });
    setModelIDsText(item.modelIds.join(", "));
    setSuspicionText(item.suspicionReasons.join("\n"));
    setNotesText(item.notes.join("\n"));
  }

  async function saveProbePatch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedProbe) {
      setError("请先在左侧选择一条探测记录。");
      return;
    }

    const payload: ProbeManualUpdateRequest = {
      ...probeForm,
      modelIds: splitCSV(modelIdsText),
      suspicionReasons: splitLines(suspicionText),
      notes: splitLines(notesText),
    };

    setSavingProbe(true);
    setError(null);
    setNotice(null);
    try {
      const response = await apiPatch<{ probe: ProbeRecord }>(`/api/admin/probes/${selectedProbe.id}`, payload);
      setNotice("探测记录已手工更新");
      setSelectedProbe(response.probe);
      await loadAdminData();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "保存失败");
    } finally {
      setSavingProbe(false);
    }
  }

  function updateProbeChannel(channel: string) {
    const nextChannel = pickChannel(enabledChannelMap, channel);
    const nextModel = pickModel(enabledChannelMap, nextChannel, "");
    setProbeForm((current) => ({ ...current, claimedChannel: nextChannel, expectedModelFamily: nextModel }));
  }

  const adminModels = enabledChannelMap[probeForm.claimedChannel] ?? [];

  return (
    <>
      <section className="panel">
        <div className="panel-heading">
          <h2>后台管理</h2>
          <p>支持渠道/模型白名单管理，以及探测结果手工纠偏。改完立即影响红黑榜。</p>
        </div>
        {error ? <p className="error-box">{error}</p> : null}
        {notice ? <p className="notice-box">{notice}</p> : null}
      </section>

      <section className="admin-layout">
        <article className="panel">
          <div className="panel-heading">
            <h2>渠道与模型管理</h2>
            <p>用于控制前台可选渠道、期望模型以及后端校验规则。</p>
          </div>

          <form className="stack-form" onSubmit={submitChannelForm}>
            <label>
              渠道
              <input value={channelForm.channelName} onChange={(event) => setChannelForm((current) => ({ ...current, channelName: event.target.value }))} />
            </label>
            <label>
              模型 ID
              <input value={channelForm.modelId} onChange={(event) => setChannelForm((current) => ({ ...current, modelId: event.target.value }))} />
            </label>
            <label className="inline-check">
              <input
                checked={channelForm.isEnabled}
                onChange={(event) => setChannelForm((current) => ({ ...current, isEnabled: event.target.checked }))}
                type="checkbox"
              />
              启用
            </label>
            <button className="primary-button" disabled={savingChannel} type="submit">
              {savingChannel ? "保存中..." : "保存渠道模型"}
            </button>
          </form>

          <div className="admin-table-wrap">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>渠道</th>
                  <th>模型</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {channelRows.map((row) => (
                  <tr key={row.id}>
                    <td>{row.channelName}</td>
                    <td>{row.modelId}</td>
                    <td>{row.isEnabled ? "启用" : "停用"}</td>
                    <td className="table-actions">
                      <button onClick={() => toggleChannelModel(row.channelName, row.modelId, row.isEnabled)} type="button">
                        {row.isEnabled ? "停用" : "启用"}
                      </button>
                      <button className="danger" onClick={() => removeChannelModel(row.id)} type="button">
                        删除
                      </button>
                    </td>
                  </tr>
                ))}
                {channelRows.length === 0 && !loading ? (
                  <tr>
                    <td colSpan={4}>暂无配置</td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </article>

        <article className="panel">
          <div className="panel-heading">
            <h2>探测记录手动修正</h2>
            <p>先在左侧选择记录，再在右侧修正渠道、模型、判定和原因。</p>
          </div>

          <div className="admin-probe-grid">
            <div className="probe-picker">
              {recent.map((item) => (
                <button key={item.id} className={selectedProbe?.id === item.id ? "active" : ""} onClick={() => startEditProbe(item)} type="button">
                  <strong>{item.stationName}</strong>
                  <span>{item.claimedChannel ?? "未设渠道"} / {item.expectedModelFamily ?? "未设模型"}</span>
                  <small>{item.id}</small>
                </button>
              ))}
              {recent.length === 0 && !loading ? <p>暂无可编辑记录</p> : null}
            </div>

            <form className="stack-form" onSubmit={saveProbePatch}>
              <label>
                宣称渠道
                <select value={probeForm.claimedChannel} onChange={(event) => updateProbeChannel(event.target.value)}>
                  {adminChannels.map((channel) => (
                    <option key={channel} value={channel}>
                      {channel}
                    </option>
                  ))}
                </select>
              </label>

              <label>
                期望模型
                <select value={probeForm.expectedModelFamily} onChange={(event) => setProbeForm((current) => ({ ...current, expectedModelFamily: event.target.value }))}>
                  {adminModels.map((model) => (
                    <option key={model} value={model}>
                      {model}
                    </option>
                  ))}
                </select>
              </label>

              <div className="split">
                <label>
                  Verdict
                  <select value={probeForm.verdict} onChange={(event) => setProbeForm((current) => ({ ...current, verdict: normalizeVerdict(event.target.value) }))}>
                    <option value="trusted">trusted</option>
                    <option value="needs_review">needs_review</option>
                    <option value="high_risk">high_risk</option>
                  </select>
                </label>
                <label>
                  Status
                  <select value={probeForm.status} onChange={(event) => setProbeForm((current) => ({ ...current, status: normalizeStatus(event.target.value) }))}>
                    <option value="success">success</option>
                    <option value="auth_failed">auth_failed</option>
                    <option value="invalid_response">invalid_response</option>
                    <option value="request_failed">request_failed</option>
                  </select>
                </label>
              </div>

              <label>
                Trust Score
                <input
                  max={100}
                  min={0}
                  onChange={(event) => setProbeForm((current) => ({ ...current, trustScore: Number(event.target.value) }))}
                  type="number"
                  value={probeForm.trustScore}
                />
              </label>

              <label>
                主模型家族
                <input value={probeForm.primaryFamily} onChange={(event) => setProbeForm((current) => ({ ...current, primaryFamily: event.target.value }))} />
              </label>

              <label>
                模型 ID（逗号分隔）
                <textarea onChange={(event) => setModelIDsText(event.target.value)} rows={3} value={modelIdsText} />
              </label>

              <label>
                可疑原因（每行一条）
                <textarea onChange={(event) => setSuspicionText(event.target.value)} rows={4} value={suspicionText} />
              </label>

              <label>
                正向说明（每行一条）
                <textarea onChange={(event) => setNotesText(event.target.value)} rows={4} value={notesText} />
              </label>

              <button className="primary-button" disabled={savingProbe || !selectedProbe} type="submit">
                {savingProbe ? "保存中..." : "保存手工修正"}
              </button>
            </form>
          </div>
        </article>
      </section>
    </>
  );
}

type RankingColumnProps = {
  title: string;
  tone: "good" | "bad";
  items: RankingResponse["red"];
};

function RankingColumn({ title, tone, items }: RankingColumnProps) {
  return (
    <div className={`ranking-column ${tone}`}>
      <header>
        <span>{title}</span>
      </header>
      {items.length > 0 ? (
        <ul>
          {items.map((item, index) => (
            <li key={`${title}-${item.name}`}>
              <div>
                <strong>#{index + 1} {item.name}</strong>
                <p>{item.totalProbes} 次样本 / {item.successRate}% 成功率</p>
              </div>
              <div>
                <strong>{item.avgScore}</strong>
                <p>高风险 {item.highRiskCount}</p>
              </div>
            </li>
          ))}
        </ul>
      ) : (
        <p className="ranking-empty">暂无数据</p>
      )}
    </div>
  );
}

function normalizeChannelModelMap(input: Record<string, string[]>): Record<string, string[]> {
  const result: Record<string, string[]> = {};
  for (const [channel, models] of Object.entries(input)) {
    const normalizedChannel = channel.trim().toLowerCase();
    if (!normalizedChannel) continue;
    const normalizedModels = [...new Set(models.map((item) => item.trim().toLowerCase()).filter(Boolean))].sort();
    if (normalizedModels.length === 0) continue;
    result[normalizedChannel] = normalizedModels;
  }
  return result;
}

function pickChannel(channelMap: Record<string, string[]>, wanted: string): string {
  const options = Object.keys(channelMap);
  if (options.length === 0) return "";
  const normalizedWanted = wanted.trim().toLowerCase();
  if (normalizedWanted && channelMap[normalizedWanted]) return normalizedWanted;
  return options[0];
}

function pickModel(channelMap: Record<string, string[]>, channel: string, wanted: string): string {
  const models = channelMap[channel] ?? [];
  if (models.length === 0) return "";
  const normalizedWanted = wanted.trim().toLowerCase();
  if (normalizedWanted && models.includes(normalizedWanted)) return normalizedWanted;
  return models[0];
}

function splitLines(text: string): string[] {
  return text
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function splitCSV(text: string): string[] {
  return text
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function normalizeStatus(value: string): ProbeManualUpdateRequest["status"] {
  const normalized = value.trim().toLowerCase();
  if (normalized === "success" || normalized === "auth_failed" || normalized === "request_failed") {
    return normalized;
  }
  return "invalid_response";
}

function normalizeVerdict(value: string): ProbeManualUpdateRequest["verdict"] {
  const normalized = value.trim().toLowerCase();
  if (normalized === "trusted" || normalized === "high_risk") {
    return normalized;
  }
  return "needs_review";
}

function parseRoute(hash: string): RouteType {
  return hash.startsWith("#/admin") ? "admin" : "home";
}

export default App;
