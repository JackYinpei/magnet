"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  API_ROUTES,
  createTask,
  deleteTask,
  fetchObjects,
  fetchTasks,
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

  const loadTasks = useCallback(async () => {
    setLoadingTasks(true);
    try {
      const data = await fetchTasks();
      setTasks(Array.isArray(data) ? data : []);
    } catch (err) {
      showMessage("error", err.message);
    } finally {
      setLoadingTasks(false);
    }
  }, [showMessage]);

  const loadObjects = useCallback(
    async (prefix) => {
      setLoadingObjects(true);
      try {
        const data = await fetchObjects(prefix ?? objectPrefix);
        setObjects(Array.isArray(data) ? data : []);
      } catch (err) {
        showMessage("error", err.message);
      } finally {
        setLoadingObjects(false);
      }
    },
    [objectPrefix, showMessage]
  );

  useEffect(() => {
    loadTasks();
    loadObjects(objectPrefix);
    const id = setInterval(() => {
      loadTasks();
      loadObjects(objectPrefix);
    }, 10000);
    return () => clearInterval(id);
  }, [loadObjects, loadTasks, objectPrefix]);

  const handleCreateTask = async (event) => {
    event.preventDefault();
    const value = magnet.trim();
    if (!value) {
      showMessage("error", "请输入有效的 magnet 链接");
      return;
    }
    setCreatingTask(true);
    try {
      const task = await createTask(value);
      setMagnet("");
      await loadTasks();
      showMessage("success", `任务创建成功：${task.id ?? task.ID ?? "新任务"}`);
    } catch (err) {
      showMessage("error", err.message);
    } finally {
      setCreatingTask(false);
    }
  };

  const handleRefreshObjects = async (event) => {
    event.preventDefault();
    await loadObjects(objectPrefix);
  };

  const handleDeleteTask = useCallback(
    async (task) => {
      const id = task?.id ?? task?.ID;
      if (!id) {
        showMessage("error", "任务 ID 无效");
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
        const result = await deleteTask(id, { deleteRemote });
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
        showMessage("error", err.message);
      } finally {
        setDeletingTaskId(null);
      }
    },
    [loadTasks, showMessage]
  );

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
          <div className="flex flex-wrap items-center gap-3 text-xs text-slate-500">
            <span className="font-mono">任务接口：{API_ROUTES.tasks}</span>
            <span className="font-mono">
              对象接口：{API_ROUTES.storageObjects}
            </span>
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
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 bg-white text-slate-700">
                {objects.length === 0 && (
                  <tr>
                    <td
                      colSpan={3}
                      className="px-4 py-6 text-center text-slate-400"
                    >
                      暂无对象
                    </td>
                  </tr>
                )}
                {objects.map((object) => (
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
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      </main>
    </div>
  );
}
