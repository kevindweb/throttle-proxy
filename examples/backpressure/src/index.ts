import { BackpressureQuery, BackpressureCache, Env } from "./env";
import { getQueryHandler } from "./queries";

const cacheKey = "backpressure_cache";
const emptyCache: BackpressureCache = { lastUpdated: 0, updates: {} };
const headers: HeadersInit = { "Content-Type": "application/json" };
const errResponse: ResponseInit = { status: 500, headers: headers };
const successResponse: ResponseInit = { status: 200, headers: headers };

function calculateThrottlePercent(
  currentValue: number,
  warningThreshold: number,
  emergencyThreshold: number,
  curve: number = 4,
): number {
  if (currentValue <= warningThreshold) return 0;
  if (currentValue >= emergencyThreshold) return 1;

  const loadFactor =
    (currentValue - warningThreshold) / (emergencyThreshold - warningThreshold);
  return 1 - Math.exp(-curve * loadFactor);
}

async function fetchOrInitializeCache(env: Env): Promise<BackpressureCache> {
  return (
    (await env.BACKPRESSURE_KV.get<BackpressureCache>(cacheKey, "json")) ||
    emptyCache
  );
}

async function updateCache(
  env: Env,
  queries: BackpressureQuery[],
  throttlePercents: number[],
  timestamp: number,
  cache: BackpressureCache,
  bpPercent: number,
): Promise<void> {
  const tsValues = queries.reduce(
    (acc, query, index) => {
      acc[query.name] = throttlePercents[index];
      return acc;
    },
    {} as Record<string, number>,
  );
  cache.updates = Object.fromEntries(
    Object.entries(cache.updates)
      .map(([timestamp, data]) => [
        timestamp,
        Object.fromEntries(
          Object.entries(data).filter(
            ([key]) => key in tsValues || key === "backpressure_percent",
          ),
        ),
      ])
      .filter(([_, data]) => Object.keys(data).length > 0),
  );
  tsValues.backpressure_percent = bpPercent;
  const updatedCache: BackpressureCache = {
    lastUpdated: timestamp,
    updates: {
      ...cache.updates,
      [timestamp]: tsValues,
    },
  };

  await env.BACKPRESSURE_KV.put(cacheKey, JSON.stringify(updatedCache));
}

async function fetchAndUpdate(env: Env): Promise<Response> {
  const nowTimestamp = Date.now();
  const cache = await fetchOrInitializeCache(env);
  const shouldRefresh = nowTimestamp - cache.lastUpdated >= 60000;
  const latestUpdate =
    cache.updates[Math.max(...Object.keys(cache.updates).map(Number))] || {};

  const queryResults = await Promise.all(
    env.Queries.map(async (query) => {
      try {
        if (!shouldRefresh && cache.lastUpdated && query.name in latestUpdate) {
          return latestUpdate[query.name];
        }

        return await getQueryHandler(query.type).refresh(env, query);
      } catch (error) {
        console.error(`Error processing query ${query.name}:`, error);
        return latestUpdate[query.name] ?? 0;
      }
    }),
  );

  const queryPercents = await Promise.all(
    queryResults.map(async (queryResult, i) => {
      const query = env.Queries[i];
      return calculateThrottlePercent(
        queryResult,
        query.warn,
        query.emergency,
        query.curve,
      );
    }),
  );

  const bpPercent = 1 - Math.max(...queryPercents);
  if (shouldRefresh) {
    await updateCache(
      env,
      env.Queries,
      queryResults,
      nowTimestamp,
      cache,
      bpPercent,
    );
  }

  return new Response(
    JSON.stringify({ backpressure: bpPercent }),
    successResponse,
  );
}

export default {
  async fetch(req: Request, env: Env, _: ExecutionContext): Promise<Response> {
    try {
      const url = new URL(req.url);
      switch (url.pathname) {
        case "/cache":
          const cache = await fetchOrInitializeCache(env);
          return new Response(JSON.stringify(cache), successResponse);
        case "/queries":
          return new Response(JSON.stringify(env.Queries), successResponse);
        default:
          return fetchAndUpdate(env);
      }
    } catch (error) {
      return new Response(
        JSON.stringify({
          error: error instanceof Error ? error.message : "Unknown error",
        }),
        errResponse,
      );
    }
  },
};
