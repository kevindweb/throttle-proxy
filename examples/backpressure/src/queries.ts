import { BackpressureQuery, BackpressureKVQueries, Env } from "./env";

export interface QueryHandler {
  refresh(env: Env, query: BackpressureQuery): Promise<number>;
}

class HTTPEndpointQueryHandler implements QueryHandler {
  async refresh(_: Env, query: BackpressureQuery): Promise<number> {
    try {
      const url = query.config;
      const response = await fetch(url);
      const data = await response.json();
      return parseNumberFromResponse(data);
    } catch (error) {
      console.error(`Query ${query.name} failed:`, error);
      return 0;
    }
  }
}

function parseNumberFromResponse(response: unknown): number {
  if (typeof response === "number") {
    return response;
  }

  if (
    Array.isArray(response) &&
    response.length > 0 &&
    typeof response[0] === "number"
  ) {
    return response[0];
  }

  throw new Error(`Unable to parse number from response: ${response}`);
}

const kvQueriesKey = "backpressure_kv_queries";
class WorkersKVQueryHandler implements QueryHandler {
  async refresh(env: Env, query: BackpressureQuery): Promise<number> {
    try {
      return (
        (await env.BACKPRESSURE_KV.get<number>(
          kvQueriesKey + "/" + query.name,
          "json",
        )) || 0
      );
    } catch (error) {
      console.error(`Query ${query.name} failed:`, error);
      return 0;
    }
  }
}

const QUERY_HANDLERS: Record<string, new () => QueryHandler> = {
  "http-endpoint": HTTPEndpointQueryHandler,
  "workers-kv": WorkersKVQueryHandler,
};

export function getQueryHandler(type: string): QueryHandler {
  const Handler = QUERY_HANDLERS[type];
  if (!Handler) throw new Error(`Unsupported query type: ${type}`);
  return new Handler();
}
