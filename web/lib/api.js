"use client";

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export const API_ROUTES = {
  tasks: `${API_BASE_URL}/api/tasks`,
  storageObjects: `${API_BASE_URL}/api/storage/objects`,
  authLogin: `${API_BASE_URL}/api/auth/login`,
  authRegister: `${API_BASE_URL}/api/auth/register`,
  authMe: `${API_BASE_URL}/api/auth/me`,
};

function buildHeaders(token, base = {}) {
  const headers = { ...base };
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  return headers;
}

async function parseJson(response) {
  const text = await response.text();
  if (!text) {
    return {};
  }
  try {
    return JSON.parse(text);
  } catch (err) {
    return {};
  }
}

function buildError(response, payload, fallbackMessage) {
  const message =
    payload?.error ??
    fallbackMessage ??
    response.statusText ??
    "Request failed";
  const error = new Error(message);
  error.status = response.status;
  return error;
}

export async function login(username, password) {
  const response = await fetch(API_ROUTES.authLogin, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  const payload = await parseJson(response);
  if (!response.ok) {
    throw buildError(response, payload, "登录失败");
  }
  return payload;
}

export async function registerUser(username, password, registerSecret) {
  const response = await fetch(API_ROUTES.authRegister, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password, register_secret: registerSecret }),
  });
  const payload = await parseJson(response);
  if (!response.ok) {
    throw buildError(response, payload, "注册失败");
  }
  return payload;
}

export async function fetchCurrentUser(token) {
  const response = await fetch(API_ROUTES.authMe, {
    method: "GET",
    headers: buildHeaders(token, { "Content-Type": "application/json" }),
    cache: "no-store",
  });
  const payload = await parseJson(response);
  if (!response.ok) {
    throw buildError(response, payload, "获取用户信息失败");
  }
  return payload;
}

export async function fetchTasks(token) {
  const response = await fetch(API_ROUTES.tasks, {
    method: "GET",
    headers: buildHeaders(token, { "Content-Type": "application/json" }),
    cache: "no-store",
  });
  const payload = await parseJson(response);
  if (!response.ok) {
    throw buildError(response, payload, "Failed to load tasks");
  }
  return payload;
}

export async function createTask(magnet, token) {
  const response = await fetch(API_ROUTES.tasks, {
    method: "POST",
    headers: buildHeaders(token, { "Content-Type": "application/json" }),
    body: JSON.stringify({ magnet }),
  });
  const payload = await parseJson(response);
  if (!response.ok) {
    throw buildError(response, payload, "Failed to create task");
  }
  return payload;
}

export async function deleteTask(id, options = {}) {
  if (!id) {
    throw new Error("Task id is required");
  }
  const { deleteRemote = false, token } = options;
  const url = new URL(`${API_ROUTES.tasks}/${id}`);
  if (deleteRemote) {
    url.searchParams.set("delete_remote", "true");
  }
  const response = await fetch(url, {
    method: "DELETE",
    headers: buildHeaders(token, { "Content-Type": "application/json" }),
  });
  const payload = await parseJson(response);
  if (!response.ok) {
    throw buildError(response, payload, "Failed to delete task");
  }
  return payload;
}

export async function fetchObjects(prefix = "", token) {
  const url = new URL(API_ROUTES.storageObjects);
  if (prefix.trim().length > 0) {
    url.searchParams.set("prefix", prefix.trim());
  }
  const response = await fetch(url, {
    method: "GET",
    headers: buildHeaders(token, { "Content-Type": "application/json" }),
    cache: "no-store",
  });
  const payload = await parseJson(response);
  if (!response.ok) {
    throw buildError(response, payload, "Failed to load objects");
  }
  return payload;
}

export function resolveApiBaseUrl() {
  return API_BASE_URL;
}
