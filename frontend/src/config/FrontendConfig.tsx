import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { api } from "../api/client";
import type { FrontendConfig } from "../api/types";

interface FrontendConfigContextValue {
  config: FrontendConfig;
  loading: boolean;
  refresh: () => Promise<void>;
  update: (
    next: Partial<Pick<FrontendConfig, "rankingAnonymized">>,
  ) => Promise<FrontendConfig>;
}

const DEFAULTS: FrontendConfig = { rankingAnonymized: false };

const FrontendConfigContext = createContext<FrontendConfigContextValue | null>(null);

export function FrontendConfigProvider({ children }: { children: ReactNode }) {
  const [config, setConfig] = useState<FrontendConfig>(DEFAULTS);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      setConfig(await api.frontendConfig());
    } catch {
      // Keep safe defaults when the API is unavailable.
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
    const onFocus = () => {
      void refresh();
    };
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [refresh]);

  const update = useCallback(
    async (next: Partial<Pick<FrontendConfig, "rankingAnonymized">>) => {
      const updated = await api.admin.updateFrontendConfig(next);
      setConfig(updated);
      return updated;
    },
    [],
  );

  const value = useMemo<FrontendConfigContextValue>(
    () => ({ config, loading, refresh, update }),
    [config, loading, refresh, update],
  );

  return (
    <FrontendConfigContext.Provider value={value}>
      {children}
    </FrontendConfigContext.Provider>
  );
}

export function useFrontendConfig(): FrontendConfigContextValue {
  const context = useContext(FrontendConfigContext);
  if (!context) throw new Error("useFrontendConfig must be used within FrontendConfigProvider");
  return context;
}

export function useRankingAnonymization(): boolean {
  return useFrontendConfig().config.rankingAnonymized;
}
