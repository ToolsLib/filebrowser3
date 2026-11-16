import { useAuthStore } from "@/stores/auth";
import router from "@/router";
import type { JwtPayload } from "jwt-decode";
import { jwtDecode } from "jwt-decode";
import { authMethod, baseURL, noAuth, logoutPage } from "./constants";
import { StatusError } from "@/api/utils";
import { setSafeTimeout } from "@/utils/timeout";

export function parseToken(token: string) {
  // falsy or malformed jwt will throw InvalidTokenError
  const data = jwtDecode<JwtPayload & { user: IUser }>(token);

  localStorage.removeItem("jwt");

  const authStore = useAuthStore();
  authStore.jwt = token;
  authStore.setUser(data.user);
  authStore.logoutTimer?.();

  // proxy auth with custom logout subject to unknown external timeout
  if (logoutPage !== "/login" && authMethod === "proxy") {
    console.warn("idle timeout disabled with proxy auth and custom logout");
    return;
  }

  const expiresAt = new Date(data.exp! * 1000);
  const timeout = expiresAt.getTime() - Date.now();
  authStore.setLogoutTimer(
    setSafeTimeout(() => {
      void logout("inactivity").catch(console.error);
    }, timeout)
  );
}

export async function validateLogin() {
  localStorage.removeItem("jwt");
  try {
    await renew();
  } catch (error) {
    if (error instanceof StatusError && error.status === 401) return;
    throw error;
  }
}

export async function login(
  username: string,
  password: string,
  recaptcha: string
) {
  const data = { username, password, recaptcha };

  const res = await fetch(`${baseURL}/api/login`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Requested-With": "FileBrowser",
    },
    body: JSON.stringify(data),
  });

  const body = await res.text();

  if (res.status === 200) {
    parseToken(body);
  } else {
    throw new StatusError(
      body || `${res.status} ${res.statusText}`,
      res.status
    );
  }
}

let renewal: Promise<void> | null = null;
export function renew() {
  if (renewal) return renewal;
  const perform = () => renewSession();
  renewal = (
    navigator.locks
      ? navigator.locks.request(`filebrowser-renew:${baseURL}`, perform)
      : perform()
  ).finally(() => {
    renewal = null;
  });
  return renewal;
}

async function renewSession() {
  const res = await fetch(`${baseURL}/api/renew`, {
    method: "POST",
    headers: {
      "X-Requested-With": "FileBrowser",
    },
  });

  const body = await res.text();

  if (res.status === 200) {
    parseToken(body);
  } else {
    throw new StatusError(
      body || `${res.status} ${res.statusText}`,
      res.status
    );
  }
}

export async function signup(username: string, password: string) {
  const data = { username, password };

  const res = await fetch(`${baseURL}/api/signup`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(data),
  });

  if (res.status !== 200) {
    const body = await res.text();
    throw new StatusError(
      body || `${res.status} ${res.statusText}`,
      res.status
    );
  }
}

export async function logout(reason?: string, all = false) {
  const authStore = useAuthStore();
  const res = await fetch(`${baseURL}/api/${all ? "logout-all" : "logout"}`, {
    method: "POST",
    headers: { "X-Auth": authStore.jwt, "X-Requested-With": "FileBrowser" },
  });
  if (!res.ok && res.status !== 401)
    throw new StatusError(
      "Could not revoke the session. Please retry signing out.",
      res.status
    );
  authStore.clearUser();

  localStorage.removeItem("jwt");
  if (noAuth) {
    window.location.reload();
  } else if (logoutPage !== "/login") {
    document.location.href = `${logoutPage}`;
  } else {
    if (typeof reason === "string" && reason.trim() !== "") {
      router.push({
        path: "/login",
        query: { "logout-reason": reason },
      });
    } else {
      router.push({
        path: "/login",
      });
    }
  }
}
