const TOKEN_KEY = "artdream.token";

let unauthorizedHandler: (() => void) | null = null;

export function getToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY);
  } catch {
    return null;
  }
}

export function setToken(token: string | null): void {
  try {
    if (token) {
      localStorage.setItem(TOKEN_KEY, token);
    } else {
      localStorage.removeItem(TOKEN_KEY);
    }
  } catch {
    // Storage can be unavailable in private modes; keep the in-memory session
    // working for the current tab.
  }
}

export function onUnauthorized(handler: (() => void) | null): void {
  unauthorizedHandler = handler;
}

export function notifyUnauthorized(): void {
  unauthorizedHandler?.();
}