import { createContext, useContext, type ReactNode } from "react";
import { createClient, type Client } from "./client";

const ApiContext = createContext<Client>(createClient());

export function ApiProvider({ client, children }: { client: Client; children: ReactNode }) {
  return <ApiContext.Provider value={client}>{children}</ApiContext.Provider>;
}

export const useApi = () => useContext(ApiContext);
