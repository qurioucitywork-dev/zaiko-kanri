export class APIError extends Error {
  constructor(message, status, code) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export async function request(path, options = {}) {
	const isFormData = typeof FormData !== "undefined" && options.body instanceof FormData;
  const response = await fetch(`/api/v1${path}`, {
    credentials: "same-origin",
    ...options,
    headers: {
      Accept: "application/json",
			...(options.body && !isFormData ? { "Content-Type": "application/json" } : {}),
      ...options.headers,
    },
  });
  if (response.status === 204) return null;
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new APIError(payload.error?.message || "通信に失敗しました。", response.status, payload.error?.code);
  }
  return payload;
}

export const api = {
  me: () => request("/auth/me"),
  login: (username, password) => request("/auth/login", { method: "POST", body: JSON.stringify({ username, password }) }),
  logout: (csrfToken) => request("/auth/logout", { method: "POST", headers: { "X-CSRF-Token": csrfToken } }),
  dashboard: () => request("/dashboard"),
  products: (params) => request(`/products?${new URLSearchParams(params)}`),
	get: (path) => request(path),
	post: (path, body, csrfToken) => request(path, { method: "POST", body: body instanceof FormData ? body : JSON.stringify(body ?? {}), headers: { "X-CSRF-Token": csrfToken } }),
	put: (path, body, csrfToken) => request(path, { method: "PUT", body: JSON.stringify(body ?? {}), headers: { "X-CSRF-Token": csrfToken } }),
	patch: (path, body, csrfToken) => request(path, { method: "PATCH", body: JSON.stringify(body ?? {}), headers: { "X-CSRF-Token": csrfToken } }),
};
