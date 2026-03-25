import { useEffect, useState } from "react";
import type { FormEvent } from "react";

import { apiGet, apiPost } from "./api";
import type { ProbeForm, ProbeListResponse, ProbeResponse, RankingResponse } from "./types";
import "./App.css";

const initialForm: ProbeForm = {
  stationName: "",
  groupName: "",
  baseUrl: "",
  apiKey: "",
  claimedChannel: "cc",
  expectedModelFamily: "claude",
};

function App() {
  const [form, setForm] = useState<ProbeForm>(initialForm);
  const [result, setResult] = useState<ProbeResponse | null>(null);
  const [recent, setRecent] = useState<ProbeListResponse["items"]>([]);
  const [stationRanking, setStationRanking] = useState<RankingResponse>({ red: [], black: [] });
  const [groupRanking, setGroupRanking] = useState<RankingResponse>({ red: [], black: [] });
  const [submitting, setSubmitting] = useState(false);
  const [loadingDashboard, setLoadingDashboard] = useState(true);
  const [error, setError] = useState<string | null>(null);

  async function loadDashboard() {
    setLoadingDashboard(true);

    try {
      const [recentResponse, stationResponse, groupResponse] = await Promise.all([
        apiGet<ProbeListResponse>("/api/probes?limit=6"),
        apiGet<RankingResponse>("/api/rankings/stations?limit=5"),
        apiGet<RankingResponse>("/api/rankings/groups?limit=5"),
      ]);

      setRecent(recentResponse.items);
      setStationRanking(stationResponse);
      setGroupRanking(groupResponse);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "加载失败");
    } finally {
      setLoadingDashboard(false);
    }
  }

  useEffect(() => {
    void loadDashboard();
  }, []);

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

  return (
    <main className="shell">
      <section className="hero">
        <div className="hero-copy">
          <p className="eyebrow">Relay Truth Desk</p>
          <h1>检测中转站到底在转什么，不再靠截图和口碑猜。</h1>
          <p className="hero-text">
            输入别人发出来的 `baseUrl + apiKey`，系统会请求模型列表接口，分析兼容性、模型家族和宣称冲突，再把结果沉淀到站点与分组红黑榜。
          </p>
        </div>
        <div className="hero-stats">
          <article>
            <strong>{recent.length}</strong>
            <span>最近样本</span>
          </article>
          <article>
            <strong>{stationRanking.red.length}</strong>
            <span>站点上榜数</span>
          </article>
          <article>
            <strong>{groupRanking.red.length}</strong>
            <span>分组上榜数</span>
          </article>
        </div>
      </section>

      <section className="layout">
        <form className="panel form-panel" onSubmit={handleSubmit}>
          <div className="panel-heading">
            <h2>发起探测</h2>
            <p>后端默认命中 `/v1/models`，失败后回退到 `/models`。</p>
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
              <input value={form.claimedChannel} onChange={(event) => updateField("claimedChannel", event.target.value)} placeholder="cc / gpt / glm" />
            </label>

            <label>
              期望家族
              <input value={form.expectedModelFamily} onChange={(event) => updateField("expectedModelFamily", event.target.value)} placeholder="claude / gpt / glm" />
            </label>
          </div>

          <button className="primary-button" disabled={submitting} type="submit">
            {submitting ? "检测中..." : "开始检测"}
          </button>

          {error ? <p className="error-box">{error}</p> : null}
        </form>

        <section className="panel result-panel">
          <div className="panel-heading">
            <h2>检测结果</h2>
            <p>结论不是官方证明，而是基于协议和返回结果的技术判断。</p>
          </div>

          {result ? (
            <div className="result-stack">
              <div className={`verdict-chip ${result.probe.verdict}`}>
                <span>{result.probe.verdict}</span>
                <strong>{result.probe.trustScore} / 100</strong>
              </div>

              <div className="result-grid">
                <article>
                  <span>主模型家族</span>
                  <strong>{result.probe.primaryFamily ?? "未识别"}</strong>
                </article>
                <article>
                  <span>HTTP 状态</span>
                  <strong>{result.probe.httpStatus ?? "无响应"}</strong>
                </article>
                <article>
                  <span>兼容 OpenAI</span>
                  <strong>{result.probe.isOpenAiCompatible ? "是" : "否"}</strong>
                </article>
                <article>
                  <span>响应耗时</span>
                  <strong>{result.probe.responseTimeMs ? `${result.probe.responseTimeMs} ms` : "未知"}</strong>
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

              <div className="list-block">
                <h3>正向证据</h3>
                {result.probe.notes.length > 0 ? (
                  <ul>
                    {result.probe.notes.map((item) => (
                      <li key={item}>{item}</li>
                    ))}
                  </ul>
                ) : (
                  <p>没有额外正向证据。</p>
                )}
              </div>

              <div className="meta-card">
                <p>命中端点：{result.probe.detectedEndpoint ?? "未命中"}</p>
                <p>API Key：{result.probe.apiKeyMasked}</p>
                <p>模型 ID：{result.probe.modelIds.join(", ") || "无"}</p>
              </div>

              {result.probe.rawExcerpt ? (
                <div className="excerpt-card">
                  <h3>原始响应节选</h3>
                  <pre>{result.probe.rawExcerpt}</pre>
                </div>
              ) : null}
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
            <p>{loadingDashboard ? "正在加载..." : "按平均分、风险次数和样本量排序。"}</p>
          </div>
          <div className="board-grid">
            <RankingColumn title="红榜" tone="good" items={stationRanking.red} />
            <RankingColumn title="黑榜" tone="bad" items={stationRanking.black} />
          </div>
        </article>

        <article className="panel">
          <div className="panel-heading">
            <h2>分组红黑榜</h2>
            <p>更适合看某个群组长期分享的渠道质量。</p>
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
          <p>最近一次探测会自动影响榜单结果。</p>
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
    </main>
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
          {items.map((item) => (
            <li key={`${title}-${item.name}`}>
              <div>
                <strong>{item.name}</strong>
                <p>{item.totalProbes} 次样本</p>
              </div>
              <div>
                <strong>{item.avgScore}</strong>
                <p>{item.successRate}% 成功率</p>
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

export default App;
