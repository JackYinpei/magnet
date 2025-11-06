"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  API_ROUTES,
  createTask,
  deleteTask,
  fetchObjects,
  fetchTasks,
  fetchCurrentUser,
  login,
  registerUser,
  resolveApiBaseUrl,
} from "../lib/api";

const STATUS_COLORS = {
  pending: "bg-amber-100 text-amber-800",
  downloading: "bg-sky-100 text-sky-800",
  paused: "bg-slate-200 text-slate-700",
  downloaded: "bg-emerald-100 text-emerald-800",
  uploading: "bg-purple-100 text-purple-800",
  completed: "bg-emerald-200 text-emerald-900",
  failed: "bg-rose-100 text-rose-800",
};

const OBJECT_BASE_URL = (process.env.NEXT_PUBLIC_OBJECT_BASE_URL ?? "").replace(
  /\/+$/,
  ""
);
const OBJECT_SIGNING_QUERY =
  process.env.NEXT_PUBLIC_OBJECT_SIGNING_QUERY ?? "";
const VIDEO_EXTENSIONS = ["mp4", "m4v", "mov", "webm", "ogg", "mkv"];
const AUTO_REFRESH_INTERVAL = 10000;
const TOKEN_STORAGE_KEY = "magnet-player.auth.token";
const USER_STORAGE_KEY = "magnet-player.auth.user";

function isVideoObject(key) {
  if (!key || typeof key !== "string") return false;
  const lastSegment = key.split("/").pop() ?? "";
  const ext = lastSegment.split(".").pop()?.toLowerCase();
  if (!ext) return false;
  return VIDEO_EXTENSIONS.includes(ext);
}

function buildObjectUrl(key) {
  if (!OBJECT_BASE_URL || !key) return "";
  const encodedKey = key
    .split("/")
    .map((segment) => encodeURIComponent(segment))
    .join("/");
  let url = `${OBJECT_BASE_URL}/${encodedKey}`;
  const signing = OBJECT_SIGNING_QUERY.trim();
  if (signing) {
    if (signing.startsWith("?") || signing.startsWith("&")) {
      url += signing;
    } else if (url.includes("?")) {
      url += `&${signing}`;
    } else {
      url += `?${signing}`;
    }
  }
  return url;
}

function formatBytes(bytes) {
  const value = Number(bytes ?? 0);
  if (!value || value <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(Math.floor(Math.log10(value) / Math.log10(1024)), units.length - 1);
  const size = value / Math.pow(1024, index);
  return `${size >= 10 ? size.toFixed(0) : size.toFixed(1)} ${units[index]}`;
}

function formatSpeed(speed) {
  const value = Number(speed ?? 0);
  if (!value || value <= 0) return "0 B/s";
  return `${formatBytes(value)}/s`;
}

function formatDate(value) {
  if (!value) return "--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "--";
  return date.toLocaleString();
}

export default function Home() {
  const [authReady, setAuthReady] = useState(false);
  const [authToken, setAuthToken] = useState("");
  const [currentUser, setCurrentUser] = useState(null);
  const [authMode, setAuthMode] = useState("login");
  const [authLoading, setAuthLoading] = useState(false);
  const [authMessage, setAuthMessage] = useState("");
  const [authForm, setAuthForm] = useState({
    username: "",
    password: "",
    registerSecret: "",
  });
  const isAuthenticated = Boolean(authToken);
  const [magnet, setMagnet] = useState("");
  const [tasks, setTasks] = useState([]);
  const [objects, setObjects] = useState([]);
  const [objectPrefix, setObjectPrefix] = useState("");
  const [loadingTasks, setLoadingTasks] = useState(false);
  const [loadingObjects, setLoadingObjects] = useState(false);
  const [creatingTask, setCreatingTask] = useState(false);
  const [deletingTaskId, setDeletingTaskId] = useState(null);
  const [message, setMessage] = useState("");
  const [messageTone, setMessageTone] = useState("info");
  const [previewObject, setPreviewObject] = useState(null);
  const tasksRequestIdRef = useRef(0);
  const objectsRequestIdRef = useRef(0);

  const apiBaseUrl = useMemo(() => resolveApiBaseUrl(), []);

  const showMessage = useCallback((tone, text) => {
    setMessageTone(tone);
    setMessage(text);
    if (text) {
      const timer = setTimeout(() => setMessage(""), 5000);
      return () => clearTimeout(timer);
    }
    return undefined;
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    try {
      const storedToken = window.localStorage.getItem(TOKEN_STORAGE_KEY);
      const storedUser = window.localStorage.getItem(USER_STORAGE_KEY);
      if (storedToken) {
        setAuthToken(storedToken);
      }
      if (storedUser) {
        try {
          setCurrentUser(JSON.parse(storedUser));
        } catch (err) {
          window.localStorage.removeItem(USER_STORAGE_KEY);
        }
      }
    } finally {
      setAuthReady(true);
    }
  }, []);

  const handleLogout = useCallback(() => {
    setAuthToken("");
    setCurrentUser(null);
    setAuthMode("login");
    setAuthForm({ username: "", password: "", registerSecret: "" });
    setTasks([]);
    setObjects([]);
    setObjectPrefix("");
    setMagnet("");
    setMessage("");
    setAuthMessage("");
    setPreviewObject(null);
    tasksRequestIdRef.current = 0;
    objectsRequestIdRef.current = 0;
    if (typeof window !== "undefined") {
      window.localStorage.removeItem(TOKEN_STORAGE_KEY);
      window.localStorage.removeItem(USER_STORAGE_KEY);
    }
  }, [setAuthForm, setAuthMode]);

  useEffect(() => {
    if (!authToken) {
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const user = await fetchCurrentUser(authToken);
        if (cancelled) {
          return;
        }
        setCurrentUser(user);
        if (typeof window !== "undefined") {
          window.localStorage.setItem(TOKEN_STORAGE_KEY, authToken);
          window.localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(user));
        }
      } catch (err) {
        if (!cancelled) {
          handleLogout();
          setAuthMessage("登录已过期，请重新登录");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [authToken, handleLogout, setAuthMessage]);

  const loadTasks = useCallback(async () => {
    if (!authToken) {
      setTasks([]);
      return;
    }
    const requestId = ++tasksRequestIdRef.current;
    setLoadingTasks(true);
    try {
      const data = await fetchTasks(authToken);
      if (tasksRequestIdRef.current === requestId) {
        setTasks(Array.isArray(data) ? data : []);
      }
    } catch (err) {
      if (tasksRequestIdRef.current === requestId) {
        if (err?.status === 401) {
          handleLogout();
          setAuthMessage("登录已过期，请重新登录");
        } else {
          showMessage("error", err.message);
        }
      }
    } finally {
      if (tasksRequestIdRef.current === requestId) {
        setLoadingTasks(false);
      }
    }
  }, [authToken, handleLogout, setAuthMessage, showMessage]);

  const loadObjects = useCallback(
    async (prefix) => {
      if (!authToken) {
        setObjects([]);
        return;
      }
      const requestId = ++objectsRequestIdRef.current;
      const targetPrefix = prefix ?? objectPrefix;
      setLoadingObjects(true);
      try {
        const data = await fetchObjects(targetPrefix, authToken);
        if (objectsRequestIdRef.current === requestId) {
          setObjects(Array.isArray(data) ? data : []);
        }
      } catch (err) {
        if (objectsRequestIdRef.current === requestId) {
          if (err?.status === 401) {
            handleLogout();
            setAuthMessage("登录已过期，请重新登录");
          } else {
            showMessage("error", err.message);
          }
        }
      } finally {
        if (objectsRequestIdRef.current === requestId) {
          setLoadingObjects(false);
        }
      }
    },
    [authToken, handleLogout, objectPrefix, setAuthMessage, showMessage]
  );

  useEffect(() => {
    if (!isAuthenticated || previewObject) {
      return undefined;
    }

    let isActive = true;
    let timeoutId;

    const refresh = async () => {
      if (!isActive) {
        return;
      }
      await Promise.all([loadTasks(), loadObjects()]);
    };

    const tick = async () => {
      await refresh();
      if (!isActive) {
        return;
      }
      timeoutId = setTimeout(tick, AUTO_REFRESH_INTERVAL);
    };

    tick();

    return () => {
      isActive = false;
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
    };
  }, [isAuthenticated, loadObjects, loadTasks, previewObject]);

  useEffect(() => {
    if (!previewObject) {
      return undefined;
    }
    const handleKeyDown = (event) => {
      if (event.key === "Escape") {
        setPreviewObject(null);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [previewObject]);

  const handleCreateTask = async (event) => {
    event.preventDefault();
    const value = magnet.trim();
    if (!value) {
      showMessage("error", "请输入有效的 magnet 链接");
      return;
    }
    if (!isAuthenticated) {
      showMessage("error", "请先登录后再创建任务");
      return;
    }
    setCreatingTask(true);
    try {
      const task = await createTask(value, authToken);
      setMagnet("");
      await loadTasks();
      showMessage("success", `任务创建成功：${task.id ?? task.ID ?? "新任务"}`);
    } catch (err) {
      if (err?.status === 401) {
        handleLogout();
        setAuthMessage("登录已过期，请重新登录");
      } else {
        showMessage("error", err.message);
      }
    } finally {
      setCreatingTask(false);
    }
  };

  const handleRefreshObjects = async (event) => {
    event.preventDefault();
    if (!isAuthenticated) {
      showMessage("error", "请先登录后再刷新对象列表");
      return;
    }
    await loadObjects(objectPrefix);
  };

  const handleDeleteTask = useCallback(
    async (task) => {
      const id = task?.id ?? task?.ID;
      if (!id) {
        showMessage("error", "任务 ID 无效");
        return;
      }
      if (!authToken) {
        showMessage("error", "请先登录后再删除任务");
        return;
      }
      const confirmed = window.confirm(`确认删除任务 ${id} 吗？该操作不可撤销。`);
      if (!confirmed) {
        return;
      }
      const remoteLocation =
        task?.s3_location ?? task?.s3Location ?? task?.S3Location ?? "";
      let deleteRemote = false;
      if (remoteLocation) {
        deleteRemote = window.confirm(
          "是否同时删除对象存储中的数据？若选择“取消”，仅删除任务记录。"
        );
      }

      setDeletingTaskId(id);
      try {
        const result = await deleteTask(id, { deleteRemote, token: authToken });
        if (Array.isArray(result?.warnings) && result.warnings.length > 0) {
          showMessage(
            "info",
            `任务 ${id} 已删除，但需要额外关注：${result.warnings.join("；")}`
          );
        } else {
          showMessage("success", `任务 ${id} 已删除`);
        }
        await loadTasks();
      } catch (err) {
        if (err?.status === 401) {
          handleLogout();
          setAuthMessage("登录已过期，请重新登录");
        } else {
          showMessage("error", err.message);
        }
      } finally {
        setDeletingTaskId(null);
      }
    },
    [authToken, handleLogout, loadTasks, setAuthMessage, showMessage]
  );

  const handlePreviewObject = useCallback(
    (object) => {
      const key = object?.key ?? "";
      if (!key) {
        showMessage("error", "对象 Key 无效");
        return;
      }
      if (!isVideoObject(key)) {
        showMessage("info", "当前仅支持 mp4、m4v、mov、webm、ogg 视频文件预览");
        return;
      }
      const url = buildObjectUrl(key);
      if (!url) {
        showMessage(
          "error",
          "请先在 NEXT_PUBLIC_OBJECT_BASE_URL 中配置可访问的对象存储地址"
        );
        return;
      }
      setPreviewObject({ key, url });
    },
    [showMessage]
  );

  const handleClosePreview = useCallback(() => {
    setPreviewObject(null);
  }, []);

  const handleAuthFieldChange = useCallback(
    (field) => (event) => {
      const value = event.target.value;
      setAuthForm((prev) => ({
        ...prev,
        [field]: value,
      }));
    },
    [setAuthForm]
  );

  const handleToggleAuthMode = useCallback(() => {
    setAuthMode((prev) => (prev === "login" ? "register" : "login"));
    setAuthMessage("");
  }, [setAuthMessage, setAuthMode]);

  const handleAuthSubmit = useCallback(
    async (event) => {
      event.preventDefault();
      const username = authForm.username.trim();
      const password = authForm.password.trim();
      const registerSecret = authForm.registerSecret.trim();

      if (!username || !password) {
        setAuthMessage("请输入用户名和密码");
        return;
      }
      if (authMode === "register" && !registerSecret) {
        setAuthMessage("请输入注册密码");
        return;
      }

      setAuthLoading(true);
      setAuthMessage("");

      try {
        const result =
          authMode === "login"
            ? await login(username, password)
            : await registerUser(username, password, registerSecret);

        if (!result?.token) {
          throw new Error("认证失败，未收到凭证");
        }

        setAuthToken(result.token);
        setCurrentUser(result.user ?? null);
        if (typeof window !== "undefined") {
          window.localStorage.setItem(TOKEN_STORAGE_KEY, result.token);
          if (result.user) {
            window.localStorage.setItem(
              USER_STORAGE_KEY,
              JSON.stringify(result.user)
            );
          } else {
            window.localStorage.removeItem(USER_STORAGE_KEY);
          }
        }
        setAuthForm({ username: "", password: "", registerSecret: "" });
      } catch (err) {
        setAuthMessage(
          err?.message ?? (authMode === "login" ? "登录失败" : "注册失败")
        );
      } finally {
        setAuthLoading(false);
      }
    },
    [authForm, authMode]
  );

  if (!authReady) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-slate-50 text-slate-500">
        正在加载配置...
      </div>
    );
  }

  if (!isAuthenticated) {
    const isRegister = authMode === "register";
    const submitLabel = authLoading
      ? isRegister
        ? "注册中..."
        : "登录中..."
      : isRegister
      ? "注册并登录"
      : "登录";
    const switchLabel = isRegister ? "已有账号？去登录" : "没有账号？立即注册";
    const title = isRegister ? "注册 Magnet Player" : "登录 Magnet Player";

    return (
      <div className="flex min-h-screen items-center justify-center bg-slate-50 px-4">
        <div className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-8 shadow-xl">
          <h1 className="text-2xl font-semibold text-slate-900 text-center">
            {title}
          </h1>
          <p className="mt-2 text-center text-sm text-slate-500">
            认证成功后方可访问下载控制台
          </p>
          <form className="mt-6 space-y-4" onSubmit={handleAuthSubmit}>
            <div>
              <label className="block text-sm font-medium text-slate-600">
                用户名
              </label>
              <input
                type="text"
                className="mt-1 w-full rounded-lg border border-slate-300 px-4 py-3 text-sm text-slate-700 focus:border-sky-500 focus:outline-none focus:ring"
                placeholder="输入用户名"
                value={authForm.username}
                onChange={handleAuthFieldChange("username")}
                disabled={authLoading}
                autoComplete="username"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-600">
                密码
              </label>
              <input
                type="password"
                className="mt-1 w-full rounded-lg border border-slate-300 px-4 py-3 text-sm text-slate-700 focus:border-sky-500 focus:outline-none focus:ring"
                placeholder="输入密码"
                value={authForm.password}
                onChange={handleAuthFieldChange("password")}
                disabled={authLoading}
                autoComplete={isRegister ? "new-password" : "current-password"}
              />
            </div>
            {isRegister && (
              <div>
                <label className="block text-sm font-medium text-slate-600">
                  注册密码
                </label>
                <input
                  type="password"
                  className="mt-1 w-full rounded-lg border border-slate-300 px-4 py-3 text-sm text-slate-700 focus:border-sky-500 focus:outline-none focus:ring"
                  placeholder="请输入后端配置中的注册密码"
                  value={authForm.registerSecret}
                  onChange={handleAuthFieldChange("registerSecret")}
                  disabled={authLoading}
                  autoComplete="one-time-code"
                />
              </div>
            )}
            {authMessage && (
              <p className="text-sm text-rose-600">{authMessage}</p>
            )}
            <button
              type="submit"
              className="w-full rounded-lg bg-sky-600 py-3 text-sm font-medium text-white transition hover:bg-sky-700 disabled:cursor-not-allowed disabled:bg-slate-400"
              disabled={authLoading}
            >
              {submitLabel}
            </button>
          </form>
          <button
            type="button"
            onClick={handleToggleAuthMode}
            className="mt-4 w-full text-center text-sm font-medium text-sky-600 hover:text-sky-700"
            disabled={authLoading}
          >
            {switchLabel}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <header className="border-b border-slate-200 bg-white px-6 py-4 shadow-sm">
        <div className="mx-auto flex max-w-6xl flex-col gap-2 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-2xl font-semibold text-slate-900">
              Magnet Player 控制台
            </h1>
            <p className="text-sm text-slate-500">
              后端 API 基址：<span className="font-mono">{apiBaseUrl}</span>
            </p>
          </div>
          <div className="flex flex-col items-start gap-3 text-xs text-slate-500 md:items-end">
            <div className="flex flex-wrap items-center gap-3">
              <span className="font-mono">任务接口：{API_ROUTES.tasks}</span>
              <span className="font-mono">
                对象接口：{API_ROUTES.storageObjects}
              </span>
            </div>
            <div className="flex items-center gap-3 text-sm text-slate-600">
              <span>
                当前用户：
                <span className="font-medium text-slate-900">
                  {currentUser?.username ?? "未知用户"}
                </span>
              </span>
              <button
                type="button"
                onClick={handleLogout}
                className="rounded-lg border border-slate-300 px-3 py-1 text-sm font-medium text-slate-600 transition hover:bg-slate-100"
              >
                退出登录
              </button>
            </div>
          </div>
        </div>
      </header>

      <main className="mx-auto flex max-w-6xl flex-col gap-8 px-6 py-8">
        <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
          <h2 className="text-lg font-semibold text-slate-900">新增磁力任务</h2>
          <p className="mt-1 text-sm text-slate-500">
            输入磁力链接后提交，新任务将立即排队下载并自动上传到对象存储。
          </p>
          <form
            onSubmit={handleCreateTask}
            className="mt-4 flex flex-col gap-4 md:flex-row"
          >
            <input
              type="text"
              placeholder="magnet:?xt=urn:btih:..."
              className="w-full rounded-lg border border-slate-300 px-4 py-3 text-sm text-slate-700 focus:border-sky-500 focus:outline-none focus:ring"
              value={magnet}
              onChange={(event) => setMagnet(event.target.value)}
              disabled={creatingTask}
            />
            <button
              type="submit"
              className="inline-flex items-center justify-center rounded-lg bg-sky-600 px-5 py-3 text-sm font-medium text-white transition hover:bg-sky-700 disabled:cursor-not-allowed disabled:bg-slate-400"
              disabled={creatingTask}
            >
              {creatingTask ? "创建中..." : "创建任务"}
            </button>
          </form>
          {message && (
            <p
              className={`mt-3 text-sm ${
                messageTone === "error"
                  ? "text-rose-600"
                  : messageTone === "success"
                  ? "text-emerald-600"
                  : "text-slate-600"
              }`}
            >
              {message}
            </p>
          )}
        </section>

        <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
          <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
            <div>
              <h2 className="text-lg font-semibold text-slate-900">
                下载任务列表
              </h2>
              <p className="text-sm text-slate-500">
                任务状态每 10 秒自动刷新，可随时手动点击刷新按钮。
              </p>
            </div>
            <button
              onClick={loadTasks}
              className="inline-flex items-center justify-center rounded-lg border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:cursor-not-allowed"
              disabled={loadingTasks}
            >
              {loadingTasks ? "刷新中..." : "刷新任务"}
            </button>
          </div>
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full table-fixed divide-y divide-slate-200 text-sm">
              <thead className="bg-slate-100 text-left text-xs uppercase tracking-wide text-slate-500">
                <tr>
                  <th className="w-20 px-4 py-3">任务 ID</th>
                  <th className="w-36 px-4 py-3">状态</th>
                  <th className="w-28 px-4 py-3">进度</th>
                  <th className="w-28 px-4 py-3">速度</th>
                  <th className="w-40 px-4 py-3">节点</th>
                  <th className="w-32 px-4 py-3">已下载</th>
                  <th className="w-32 px-4 py-3">总大小</th>
                  <th className="px-4 py-3">名称</th>
                  <th className="w-44 px-4 py-3">更新时间</th>
                  <th className="w-64 px-4 py-3">上传地址</th>
                  <th className="w-28 px-4 py-3">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 bg-white text-slate-700">
                {tasks.length === 0 && (
                  <tr>
                    <td colSpan={11} className="px-4 py-6 text-center text-slate-400">
                      暂无任务
                    </td>
                  </tr>
                )}
                {tasks.map((task) => {
                  const id = task.id ?? task.ID;
                  const status = task.status ?? task.Status;
                  const progress = task.progress ?? task.Progress ?? 0;
                  const totalPeers =
                    task.total_peers ??
                    task.totalPeers ??
                    task.TotalPeers ??
                    0;
                  const activePeers =
                    task.active_peers ??
                    task.activePeers ??
                    task.ActivePeers ??
                    0;
                  const pendingPeers =
                    task.pending_peers ??
                    task.pendingPeers ??
                    task.PendingPeers ??
                    0;
                  const connectedSeeders =
                    task.connected_seeders ??
                    task.connectedSeeders ??
                    task.ConnectedSeeders ??
                    0;
                  const halfOpenPeers =
                    task.half_open_peers ??
                    task.halfOpenPeers ??
                    task.HalfOpenPeers ??
                    0;
                  const downloaded =
                    task.downloaded_bytes ??
                    task.downloadedBytes ??
                    task.DownloadedBytes;
                  const totalSize =
                    task.total_size ?? task.totalSize ?? task.TotalSize;
                  const name =
                    task.torrent_name ?? task.torrentName ?? task.TorrentName;
                  const updatedAt =
                    task.updated_at ?? task.updatedAt ?? task.UpdatedAt;
                  const s3Location =
                    task.s3_location ?? task.s3Location ?? task.S3Location;

                  return (
                    <tr key={id}>
                      <td className="truncate px-4 py-3 font-mono text-xs">
                        {id}
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className={`inline-flex rounded-full px-3 py-1 text-xs font-medium ${
                            STATUS_COLORS[String(status).toLowerCase()] ??
                            "bg-slate-200 text-slate-700"
                          }`}
                        >
                          {status}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-3">
                          <span>{progress}%</span>
                          <div className="h-2 w-20 overflow-hidden rounded-full bg-slate-200">
                            <div
                              className="h-2 rounded-full bg-sky-500 transition-all"
                              style={{
                                width: `${Math.min(
                                  typeof progress === "number" ? progress : Number(progress) || 0,
                                  100
                                )}%`,
                              }}
                            />
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        {formatSpeed(task.speed ?? task.Speed)}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex flex-col gap-1">
                          <span className="font-mono text-xs">
                            {activePeers}/{totalPeers} 活跃/总
                          </span>
                          <span className="font-mono text-[11px] text-slate-500">
                            S:{connectedSeeders} P:{pendingPeers} H:{halfOpenPeers}
                          </span>
                        </div>
                      </td>
                      <td className="px-4 py-3">{formatBytes(downloaded)}</td>
                      <td className="px-4 py-3">{formatBytes(totalSize)}</td>
                      <td className="px-4 py-3">
                        <div className="max-w-xs truncate">{name ?? "--"}</div>
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-500">
                        {formatDate(updatedAt)}
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-500">
                        {s3Location ?? "--"}
                      </td>
                      <td className="px-4 py-3">
                        <button
                          onClick={() => handleDeleteTask(task)}
                          className="inline-flex items-center justify-center rounded-lg border border-rose-200 px-3 py-2 text-xs font-medium text-rose-600 transition hover:bg-rose-50 disabled:cursor-not-allowed disabled:opacity-50"
                          disabled={deletingTaskId === id}
                        >
                          {deletingTaskId === id ? "删除中..." : "删除"}
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>

        <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
          <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
            <div>
              <h2 className="text-lg font-semibold text-slate-900">
                对象存储内容
              </h2>
              <p className="text-sm text-slate-500">
                查询当前 Bucket 下的文件，可通过前缀快速过滤。
              </p>
            </div>
            <form
              onSubmit={handleRefreshObjects}
              className="flex flex-col gap-2 md:flex-row md:items-center"
            >
              <input
                type="text"
                placeholder="按前缀过滤，例如 task-"
                value={objectPrefix}
                onChange={(event) => setObjectPrefix(event.target.value)}
                className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-700 focus:border-sky-500 focus:outline-none focus:ring md:w-56"
              />
              <button
                type="submit"
                className="inline-flex items-center justify-center rounded-lg border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:cursor-not-allowed"
                disabled={loadingObjects}
              >
                {loadingObjects ? "查询中..." : "查询对象"}
              </button>
            </form>
          </div>
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full table-fixed divide-y divide-slate-200 text-sm">
              <thead className="bg-slate-100 text-left text-xs uppercase tracking-wide text-slate-500">
                <tr>
                  <th className="px-4 py-3">Key</th>
                  <th className="w-32 px-4 py-3">大小</th>
                  <th className="w-44 px-4 py-3">最后修改时间</th>
                  <th className="w-28 px-4 py-3">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 bg-white text-slate-700">
                {objects.length === 0 && (
                  <tr>
                    <td
                      colSpan={4}
                      className="px-4 py-6 text-center text-slate-400"
                    >
                      暂无对象
                    </td>
                  </tr>
                )}
                {objects.map((object) => {
                  const objectUrl = buildObjectUrl(object.key);
                  const canPreview = isVideoObject(object.key) && Boolean(objectUrl);
                  const previewTitle = canPreview
                    ? "播放该对象"
                    : OBJECT_BASE_URL
                    ? "当前仅支持常见视频格式预览"
                    : "请配置 NEXT_PUBLIC_OBJECT_BASE_URL 后再试";
                  return (
                    <tr key={object.key}>
                      <td className="px-4 py-3">
                        <div className="max-w-lg truncate font-mono text-xs">
                          {object.key}
                        </div>
                      </td>
                      <td className="px-4 py-3">{formatBytes(object.size)}</td>
                      <td className="px-4 py-3 text-xs text-slate-500">
                        {formatDate(
                          object.last_modified ??
                            object.lastModified ??
                            object.LastModified
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <button
                          type="button"
                          className="text-sm font-medium text-sky-600 transition hover:text-sky-700 disabled:cursor-not-allowed disabled:text-slate-400"
                          onClick={() => handlePreviewObject(object)}
                          disabled={!canPreview}
                          title={previewTitle}
                        >
                          播放
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>
      </main>
      {previewObject && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/70 px-4 py-6 backdrop-blur-sm"
          onClick={handleClosePreview}
          role="presentation"
        >
          <div
            className="w-full max-w-4xl overflow-hidden rounded-xl bg-slate-950 shadow-2xl"
            onClick={(event) => event.stopPropagation()}
            role="presentation"
          >
            <div className="flex items-center justify-between border-b border-slate-800 px-4 py-3">
              <div className="truncate font-mono text-xs text-slate-300">
                {previewObject.key}
              </div>
              <button
                type="button"
                onClick={handleClosePreview}
                className="text-xs font-medium text-slate-300 transition hover:text-white"
              >
                关闭
              </button>
            </div>
            <div className="bg-black">
              <video
                key={previewObject.url}
                src={previewObject.url}
                controls
                controlsList="nodownload"
                autoPlay
                className="h-full w-full bg-black"
              >
                您的浏览器不支持 HTML5 视频播放。
              </video>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
