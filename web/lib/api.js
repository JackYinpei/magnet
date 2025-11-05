"use client";

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export const API_ROUTES = {
  tasks: `${API_BASE_URL}/api/tasks`,
  storageObjects: `${API_BASE_URL}/api/storage/objects`,
  storageObjectUrl: `${API_BASE_URL}/api/storage/object-url`,
};

export async function fetchTasks() {
  const response = await fetch(API_ROUTES.tasks, {
    method: "GET",
    headers: { "Content-Type": "application/json" },
    cache: "no-store",
  });
  if (!response.ok) {
    throw new Error(`Failed to load tasks: ${response.statusText}`);
  }
  return response.json();
}

export async function createTask(magnet) {
  const response = await fetch(API_ROUTES.tasks, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ magnet }),
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    throw new Error(payload.error ?? "Failed to create task");
  }
  return response.json();
}

export async function deleteTask(id, options = {}) {
  if (!id) {
    throw new Error("Task id is required");
  }
  const { deleteRemote = false } = options;
  const url = new URL(`${API_ROUTES.tasks}/${id}`);
  if (deleteRemote) {
    url.searchParams.set("delete_remote", "true");
  }
  const response = await fetch(url, {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    throw new Error(payload.error ?? "Failed to delete task");
  }
  return response.json();
}

export async function fetchObjects(prefix = "") {
  const url = new URL(API_ROUTES.storageObjects);
  if (prefix.trim().length > 0) {
    url.searchParams.set("prefix", prefix.trim());
  }
  const response = await fetch(url, {
    method: "GET",
    headers: { "Content-Type": "application/json" },
    cache: "no-store",
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    throw new Error(payload.error ?? "Failed to load objects");
  }
  return response.json();
}

export async function fetchObjectUrl(key, options = {}) {
  if (!key) {
    throw new Error("Object key is required");
  }
  const url = new URL(API_ROUTES.storageObjectUrl);
  url.searchParams.set("key", key);
  if (options.expires) {
    url.searchParams.set("expires", String(options.expires));
  }
  const response = await fetch(url, {
    method: "GET",
    headers: { "Content-Type": "application/json" },
    cache: "no-store",
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    throw new Error(payload.error ?? "Failed to resolve object URL");
  }
  return response.json();
}

export function resolveApiBaseUrl() {
  return API_BASE_URL;
}
