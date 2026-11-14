import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { api, ApiError } from "../api/client";
import { getToken, onUnauthorized, setToken } from "../api/session";
import type { User } from "../api/types";

interface Credentials {
  username: string;
  password: string;
  displayName?: string;
}

interface AuthContextValue {
  user: User | null;
  status: "loading" | "ready";
  needsBootstrap: boolean;
  isAdmin: boolean;
  login: (username: string, password: string) => Promise<User>;
  register: (data: Credentials) => Promise<User>;
  bootstrap: (data: Credentials) => Promise<User>;
  logout: () => Promise<void>;
  refreshUser: () => Promise<void>;
  setUser: (user: User | null) => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [status, setStatus] = useState<"loading" | "ready">("loading");
  const [needsBootstrap, setNeedsBootstrap] = useState(false);

  const acceptSession = useCallback((token: string, nextUser: User) => {
    setToken(token);
    setUser(nextUser);
    setNeedsBootstrap(false);
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      let needs = false;
      try {
        needs = (await api.needsBootstrap()).needsBootstrap;
      } catch {
        // Backend may be starting or unreachable; the UI still renders and
        // surfaces errors on action.
      }
      let current: User | null = null;
      if (getToken()) {
        try {
          current = (await api.me()).user;
        } catch (error) {
          if (error instanceof ApiError && error.status === 401) {
            setToken(null);
          }
        }
      }
      if (!cancelled) {
        setNeedsBootstrap(needs);
        setUser(current);
        setStatus("ready");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    onUnauthorized(() => {
      setToken(null);
      setUser(null);
    });
    return () => onUnauthorized(null);
  }, []);

  const login = useCallback(
    async (username: string, password: string) => {
      const response = await api.login({ username, password });
      acceptSession(response.token, response.user);
      return response.user;
    },
    [acceptSession],
  );

  const register = useCallback(
    async (data: Credentials) => {
      const response = await api.register(data);
      acceptSession(response.token, response.user);
      return response.user;
    },
    [acceptSession],
  );

  const bootstrap = useCallback(
    async (data: Credentials) => {
      const response = await api.bootstrap(data);
      acceptSession(response.token, response.user);
      return response.user;
    },
    [acceptSession],
  );

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } catch {
      // Clearing the local session is enough if the server already forgot it.
    }
    setToken(null);
    setUser(null);
  }, []);

  const refreshUser = useCallback(async () => {
    if (!getToken()) {
      setUser(null);
      return;
    }
    const response = await api.me();
    setUser(response.user);
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      status,
      needsBootstrap,
      isAdmin: Boolean(user?.isAdmin),
      login,
      register,
      bootstrap,
      logout,
      refreshUser,
      setUser,
    }),
    [user, status, needsBootstrap, login, register, bootstrap, logout, refreshUser],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) throw new Error("useAuth must be used within AuthProvider");
  return context;
}