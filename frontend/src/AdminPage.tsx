import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";

import { ApiError, apiDelete, apiGet, apiPatch, apiPost } from "./api";
import type {
  AdminAccountUpdateRequest,
  AdminSessionResponse,
  AdminUserProfile,
  ChannelModelListResponse,
  ChannelModelUpsertRequest,
  ProbeListResponse,
  ProbeManualUpdateRequest,
  ProbeRecord,
} from "./types";

const initialAdminAccountForm: AdminAccountUpdateRequest = {
  username: "",
  currentPassword: "",
  newPassword: "",
};

export default function AdminPageShell() {
  const [session, setSession] = useState<AdminSessionResponse | null>(null);
  const [checkingSession, setCheckingSession] = useState(true);
  const [loginPending, setLoginPending] = useState(false);
  const [loginError, setLoginError] = useState<string | null>(null);
  const [loginForm, setLoginForm] = useState({ username: "", password: "" });

  useEffect(() => {
    void loadSession();
  }, []);

  async function loadSession() {
    setCheckingSession(true);
    setLoginError(null);

    try {
      const response = await apiGet<AdminSessionResponse>("/api/admin/session");
      setSession(response);
    } catch (requestError) {
      setLoginError(
        requestError instanceof Error
          ? requestError.message
          : "Failed to inspect admin session",
      );
    } finally {
      setCheckingSession(false);
    }
  }

  async function handleLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoginPending(true);
    setLoginError(null);

    try {
      const response = await apiPost<AdminSessionResponse>("/api/admin/login", {
        username: loginForm.username.trim(),
        password: loginForm.password,
      });
      setSession(response);
      setLoginForm({ username: "", password: "" });
    } catch (requestError) {
      setLoginError(
        requestError instanceof Error
          ? requestError.message
          : "Admin login failed",
      );
    } finally {
      setLoginPending(false);
    }
  }

  async function handleLogout() {
    setLoginError(null);
    try {
      await apiPost<{ ok: boolean }>("/api/admin/logout", {});
    } catch (requestError) {
      setLoginError(
        requestError instanceof Error ? requestError.message : "Logout failed",
      );
      return;
    }

    setSession((current) =>
      current
        ? {
            ...current,
            authenticated: false,
            user: null,
          }
        : { configured: true, authenticated: false, user: null },
    );
  }

  if (checkingSession) {
    return (
      <section className="panel">
        <div className="panel-heading">
          <h2>Admin</h2>
          <p>Checking session...</p>
        </div>
      </section>
    );
  }

  if (loginError && !session) {
    return (
      <section className="panel">
        <div className="panel-heading">
          <h2>Admin</h2>
          <p>{loginError}</p>
        </div>
      </section>
    );
  }

  if (!session?.configured) {
    return (
      <section className="panel">
        <div className="panel-heading">
          <h2>Admin Not Initialized</h2>
          <p>
            Set an initial admin username and password in the backend
            environment, then restart the backend.
          </p>
        </div>
        <div className="meta-card">
          <p>Required env: ADMIN_INIT_USERNAME</p>
          <p>Required env: ADMIN_INIT_PASSWORD</p>
          <p>
            The backend will hash the password and store the account in
            PostgreSQL on first startup.
          </p>
        </div>
      </section>
    );
  }

  if (!session.authenticated || !session.user) {
    return (
      <section className="panel">
        <div className="panel-heading">
          <h2>Admin Login</h2>
          <p>
            The admin console now requires a database-backed account and
            session.
          </p>
        </div>
        <form className="stack-form" onSubmit={handleLogin}>
          <label>
            Username
            <input
              autoComplete="username"
              onChange={(event) =>
                setLoginForm((current) => ({
                  ...current,
                  username: event.target.value,
                }))
              }
              value={loginForm.username}
            />
          </label>
          <label>
            Password
            <input
              autoComplete="current-password"
              onChange={(event) =>
                setLoginForm((current) => ({
                  ...current,
                  password: event.target.value,
                }))
              }
              type="password"
              value={loginForm.password}
            />
          </label>
          <button
            className="primary-button"
            disabled={loginPending}
            type="submit"
          >
            {loginPending ? "Logging in..." : "Login"}
          </button>
          {loginError ? <p className="error-box">{loginError}</p> : null}
        </form>
      </section>
    );
  }

  return (
    <AdminConsole
      onLoggedOut={handleLogout}
      onSessionUserChange={(user) =>
        setSession((current) =>
          current
            ? { ...current, authenticated: true, user }
            : { configured: true, authenticated: true, user },
        )
      }
      onUnauthorized={() =>
        setSession((current) =>
          current ? { ...current, authenticated: false, user: null } : null,
        )
      }
      sessionUser={session.user}
    />
  );
}

type AdminConsoleProps = {
  sessionUser: AdminUserProfile;
  onLoggedOut: () => Promise<void>;
  onSessionUserChange: (user: AdminUserProfile) => void;
  onUnauthorized: () => void;
};

function AdminConsole({
  sessionUser,
  onLoggedOut,
  onSessionUserChange,
  onUnauthorized,
}: AdminConsoleProps) {
  const [channelRows, setChannelRows] = useState<
    ChannelModelListResponse["items"]
  >([]);
  const [recent, setRecent] = useState<ProbeRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [savingChannel, setSavingChannel] = useState(false);
  const [savingProbe, setSavingProbe] = useState(false);
  const [savingAccount, setSavingAccount] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [selectedProbe, setSelectedProbe] = useState<ProbeRecord | null>(null);
  const [channelForm, setChannelForm] = useState<ChannelModelUpsertRequest>({
    channelName: "cc",
    modelId: "claude-sonnet-4.6",
    isEnabled: true,
  });
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
  const [accountForm, setAccountForm] = useState<AdminAccountUpdateRequest>({
    ...initialAdminAccountForm,
    username: sessionUser.username,
  });
  const [confirmPassword, setConfirmPassword] = useState("");
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

  const adminChannels = useMemo(
    () => Object.keys(enabledChannelMap),
    [enabledChannelMap],
  );

  useEffect(() => {
    setAccountForm((current) => ({
      ...current,
      username: sessionUser.username,
    }));
  }, [sessionUser.username]);

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
      if (requestError instanceof ApiError && requestError.status === 401) {
        onUnauthorized();
        return;
      }
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Failed to load admin data",
      );
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
      setNotice("Channel model saved");
      await loadAdminData();
    } catch (requestError) {
      if (requestError instanceof ApiError && requestError.status === 401) {
        onUnauthorized();
        return;
      }
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Failed to save channel model",
      );
    } finally {
      setSavingChannel(false);
    }
  }

  async function toggleChannelModel(
    channelName: string,
    modelId: string,
    isEnabled: boolean,
  ) {
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
      setNotice("Channel model status updated");
    } catch (requestError) {
      if (requestError instanceof ApiError && requestError.status === 401) {
        onUnauthorized();
        return;
      }
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Failed to update channel model",
      );
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
      setNotice("Channel model deleted");
    } catch (requestError) {
      if (requestError instanceof ApiError && requestError.status === 401) {
        onUnauthorized();
        return;
      }
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Failed to delete channel model",
      );
    } finally {
      setSavingChannel(false);
    }
  }

  function startEditProbe(item: ProbeRecord) {
    const preferredChannel = pickChannel(
      enabledChannelMap,
      item.claimedChannel ?? "",
    );
    const preferredModel = pickModel(
      enabledChannelMap,
      preferredChannel,
      item.expectedModelFamily ?? "",
    );
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
      setError("Select a probe record first.");
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
      const response = await apiPatch<{ probe: ProbeRecord }>(
        `/api/admin/probes/${selectedProbe.id}`,
        payload,
      );
      setNotice("Probe record updated");
      setSelectedProbe(response.probe);
      await loadAdminData();
    } catch (requestError) {
      if (requestError instanceof ApiError && requestError.status === 401) {
        onUnauthorized();
        return;
      }
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Failed to save probe patch",
      );
    } finally {
      setSavingProbe(false);
    }
  }

  async function saveAdminAccount(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSavingAccount(true);
    setError(null);
    setNotice(null);

    if (
      accountForm.newPassword &&
      accountForm.newPassword !== confirmPassword
    ) {
      setSavingAccount(false);
      setError("New password confirmation does not match.");
      return;
    }

    try {
      const response = await apiPatch<{ user: AdminUserProfile }>(
        "/api/admin/account",
        {
          username: accountForm.username.trim(),
          currentPassword: accountForm.currentPassword,
          newPassword: accountForm.newPassword,
        },
      );
      onSessionUserChange(response.user);
      setAccountForm({
        ...initialAdminAccountForm,
        username: response.user.username,
      });
      setConfirmPassword("");
      setNotice(
        accountForm.newPassword
          ? "Admin username/password updated"
          : "Admin username updated",
      );
    } catch (requestError) {
      if (requestError instanceof ApiError && requestError.status === 401) {
        onUnauthorized();
        return;
      }
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Failed to save admin account",
      );
    } finally {
      setSavingAccount(false);
    }
  }

  function updateProbeChannel(channel: string) {
    const nextChannel = pickChannel(enabledChannelMap, channel);
    const nextModel = pickModel(enabledChannelMap, nextChannel, "");
    setProbeForm((current) => ({
      ...current,
      claimedChannel: nextChannel,
      expectedModelFamily: nextModel,
    }));
  }

  const adminModels = enabledChannelMap[probeForm.claimedChannel] ?? [];

  return (
    <>
      <section className="panel">
        <div className="panel-heading">
          <h2>Admin Console</h2>
          <p>Logged in as {sessionUser.username}</p>
        </div>
        <div className="table-actions">
          <button
            className="danger"
            onClick={() => void onLoggedOut()}
            type="button"
          >
            Logout
          </button>
        </div>
        {error ? <p className="error-box">{error}</p> : null}
        {notice ? <p className="notice-box">{notice}</p> : null}
      </section>

      <section className="admin-layout">
        <article className="panel">
          <div className="panel-heading">
            <h2>Admin Account</h2>
            <p>
              Username, password hash, and login sessions are all persisted in
              PostgreSQL.
            </p>
          </div>

          <form className="stack-form" onSubmit={saveAdminAccount}>
            <label>
              Username
              <input
                onChange={(event) =>
                  setAccountForm((current) => ({
                    ...current,
                    username: event.target.value,
                  }))
                }
                value={accountForm.username}
              />
            </label>
            <label>
              Current Password
              <input
                autoComplete="current-password"
                onChange={(event) =>
                  setAccountForm((current) => ({
                    ...current,
                    currentPassword: event.target.value,
                  }))
                }
                type="password"
                value={accountForm.currentPassword}
              />
            </label>
            <label>
              New Password
              <input
                autoComplete="new-password"
                onChange={(event) =>
                  setAccountForm((current) => ({
                    ...current,
                    newPassword: event.target.value,
                  }))
                }
                type="password"
                value={accountForm.newPassword}
              />
            </label>
            <label>
              Confirm New Password
              <input
                autoComplete="new-password"
                onChange={(event) => setConfirmPassword(event.target.value)}
                type="password"
                value={confirmPassword}
              />
            </label>
            <button
              className="primary-button"
              disabled={savingAccount}
              type="submit"
            >
              {savingAccount ? "Saving..." : "Save Admin Account"}
            </button>
          </form>
        </article>

        <article className="panel">
          <div className="panel-heading">
            <h2>Channel Models</h2>
            <p>
              These records define what the public form can select and what the
              backend accepts.
            </p>
          </div>

          <form className="stack-form" onSubmit={submitChannelForm}>
            <label>
              Channel
              <input
                value={channelForm.channelName}
                onChange={(event) =>
                  setChannelForm((current) => ({
                    ...current,
                    channelName: event.target.value,
                  }))
                }
              />
            </label>
            <label>
              Model ID
              <input
                value={channelForm.modelId}
                onChange={(event) =>
                  setChannelForm((current) => ({
                    ...current,
                    modelId: event.target.value,
                  }))
                }
              />
            </label>
            <label className="inline-check">
              <input
                checked={channelForm.isEnabled}
                onChange={(event) =>
                  setChannelForm((current) => ({
                    ...current,
                    isEnabled: event.target.checked,
                  }))
                }
                type="checkbox"
              />
              Enabled
            </label>
            <button
              className="primary-button"
              disabled={savingChannel}
              type="submit"
            >
              {savingChannel ? "Saving..." : "Save Channel Model"}
            </button>
          </form>

          <div className="admin-table-wrap">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>Channel</th>
                  <th>Model</th>
                  <th>Status</th>
                  <th>Action</th>
                </tr>
              </thead>
              <tbody>
                {channelRows.map((row) => (
                  <tr key={row.id}>
                    <td>{row.channelName}</td>
                    <td>{row.modelId}</td>
                    <td>{row.isEnabled ? "enabled" : "disabled"}</td>
                    <td className="table-actions">
                      <button
                        onClick={() =>
                          void toggleChannelModel(
                            row.channelName,
                            row.modelId,
                            row.isEnabled,
                          )
                        }
                        type="button"
                      >
                        {row.isEnabled ? "Disable" : "Enable"}
                      </button>
                      <button
                        className="danger"
                        onClick={() => void removeChannelModel(row.id)}
                        type="button"
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
                {channelRows.length === 0 && !loading ? (
                  <tr>
                    <td colSpan={4}>No channel model configured.</td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </article>
      </section>

      <section className="panel">
        <div className="panel-heading">
          <h2>Manual Probe Correction</h2>
          <p>
            Select a probe on the left and update its channel, verdict, notes,
            and model IDs on the right.
          </p>
        </div>

        <div className="admin-probe-grid">
          <div className="probe-picker">
            {recent.map((item) => (
              <button
                key={item.id}
                className={selectedProbe?.id === item.id ? "active" : ""}
                onClick={() => startEditProbe(item)}
                type="button"
              >
                <strong>{item.stationName}</strong>
                <span>
                  {item.claimedChannel ?? "no channel"} /{" "}
                  {item.expectedModelFamily ?? "no model"}
                </span>
                <small>{item.id}</small>
              </button>
            ))}
            {recent.length === 0 && !loading ? (
              <p>No editable records yet.</p>
            ) : null}
          </div>

          <form className="stack-form" onSubmit={saveProbePatch}>
            <label>
              Claimed Channel
              <select
                value={probeForm.claimedChannel}
                onChange={(event) => updateProbeChannel(event.target.value)}
              >
                {adminChannels.map((channel) => (
                  <option key={channel} value={channel}>
                    {channel}
                  </option>
                ))}
              </select>
            </label>

            <label>
              Expected Model
              <select
                value={probeForm.expectedModelFamily}
                onChange={(event) =>
                  setProbeForm((current) => ({
                    ...current,
                    expectedModelFamily: event.target.value,
                  }))
                }
              >
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
                <select
                  value={probeForm.verdict}
                  onChange={(event) =>
                    setProbeForm((current) => ({
                      ...current,
                      verdict: normalizeVerdict(event.target.value),
                    }))
                  }
                >
                  <option value="trusted">trusted</option>
                  <option value="needs_review">needs_review</option>
                  <option value="high_risk">high_risk</option>
                </select>
              </label>
              <label>
                Status
                <select
                  value={probeForm.status}
                  onChange={(event) =>
                    setProbeForm((current) => ({
                      ...current,
                      status: normalizeStatus(event.target.value),
                    }))
                  }
                >
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
                onChange={(event) =>
                  setProbeForm((current) => ({
                    ...current,
                    trustScore: Number(event.target.value),
                  }))
                }
                type="number"
                value={probeForm.trustScore}
              />
            </label>

            <label>
              Primary Family
              <input
                value={probeForm.primaryFamily}
                onChange={(event) =>
                  setProbeForm((current) => ({
                    ...current,
                    primaryFamily: event.target.value,
                  }))
                }
              />
            </label>

            <label>
              Model IDs, comma separated
              <textarea
                onChange={(event) => setModelIDsText(event.target.value)}
                rows={3}
                value={modelIdsText}
              />
            </label>

            <label>
              Suspicion Reasons, one per line
              <textarea
                onChange={(event) => setSuspicionText(event.target.value)}
                rows={4}
                value={suspicionText}
              />
            </label>

            <label>
              Notes, one per line
              <textarea
                onChange={(event) => setNotesText(event.target.value)}
                rows={4}
                value={notesText}
              />
            </label>

            <button
              className="primary-button"
              disabled={savingProbe || !selectedProbe}
              type="submit"
            >
              {savingProbe ? "Saving..." : "Save Manual Patch"}
            </button>
          </form>
        </div>
      </section>
    </>
  );
}

function normalizeChannelModelMap(
  input: Record<string, string[]>,
): Record<string, string[]> {
  const result: Record<string, string[]> = {};
  for (const [channel, models] of Object.entries(input)) {
    const normalizedChannel = channel.trim().toLowerCase();
    if (!normalizedChannel) continue;
    const normalizedModels = [
      ...new Set(
        models.map((item) => item.trim().toLowerCase()).filter(Boolean),
      ),
    ].sort();
    if (normalizedModels.length === 0) continue;
    result[normalizedChannel] = normalizedModels;
  }
  return result;
}

function pickChannel(
  channelMap: Record<string, string[]>,
  wanted: string,
): string {
  const options = Object.keys(channelMap);
  if (options.length === 0) return "";
  const normalizedWanted = wanted.trim().toLowerCase();
  if (normalizedWanted && channelMap[normalizedWanted]) return normalizedWanted;
  return options[0];
}

function pickModel(
  channelMap: Record<string, string[]>,
  channel: string,
  wanted: string,
): string {
  const models = channelMap[channel] ?? [];
  if (models.length === 0) return "";
  const normalizedWanted = wanted.trim().toLowerCase();
  if (normalizedWanted && models.includes(normalizedWanted))
    return normalizedWanted;
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
  if (
    normalized === "success" ||
    normalized === "auth_failed" ||
    normalized === "request_failed"
  ) {
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
