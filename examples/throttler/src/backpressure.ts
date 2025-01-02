import { Env, BackpressureQuery } from "env";
import { Middleware } from "middleware";

const ActiveCounterName = "ACTIVE_BP_REQUESTS";
const WatermarkCounterName = "BP_WATERMARK";
const DurableQueryValuesName = "BP_QUERY_VALUES";

export const backpressure: Middleware = async (req, env, next) => {
  const count = await getActiveCounter(env);
  console.log(
    `Applying backpressure queries ${env.BACKPRESSURE.QUERIES.length} requests ${count}`
  );

  await incrementActiveCounter(env);
  const res = await next(req, env);
  await decrementActiveCounter(env);
  await release(env);
  return res;
};

/*

func (bp *Backpressure) updateThrottle(q BackpressureQuery, curr float64) {
	bp.throttleFlags.Store(q, q.throttlePercent(curr))
	throttlePercent := 0.0
	bp.throttleFlags.Range(func(_ BackpressureQuery, value float64) bool {
		throttlePercent = max(throttlePercent, value)
		return true
	})

	bp.mu.Lock()
	bp.allowance = 1 - throttlePercent
	bp.allowanceGauge.Set(bp.allowance)
	bp.constrainWatermark()
	bp.mu.Unlock()
}
*/

async function allowanceRatio(env: Env): Promise<number> {
  const queryValues = await refreshQueries(env);
  return 4;
}

async function refreshQueries(env: Env): Promise<number[]> {
  const queryValues = await getQueryValues(env);

  for (var i = 0; i < env.BACKPRESSURE.QUERIES.length; i++) {
    queryValues[i] = await fetchQueryValue(env.BACKPRESSURE.QUERIES[i]);
  }

  await setQueryValues(env, queryValues);
  return queryValues;
}

async function fetchQueryValue(query: BackpressureQuery): Promise<number> {
  const data: Response = await fetch(
    "http://www.randomnumberapi.com/api/v1.0/random?min=100&max=1000&count=1"
  );
  const randomValue = (await data.json()) as number[];
  return randomValue[0];
}

async function getQueryValues(env: Env): Promise<number[]> {
  const id = env.QUERY_VALUES.idFromName(DurableQueryValuesName);
  return await env.QUERY_VALUES.get(id).get();
}

async function setQueryValues(env: Env, values: number[]) {
  const id = env.QUERY_VALUES.idFromName(DurableQueryValuesName);
  await env.QUERY_VALUES.get(id).set(values);
}

async function release(env: Env) {
  const allowance = await allowanceRatio(env);
  let watermark = await getWatermark(env);
  watermark = Math.min(watermark + 1, allowance * env.BACKPRESSURE.CWDN_MAX);
  watermark = Math.max(watermark, env.BACKPRESSURE.CWDN_MIN);
  await setWatermark(env, watermark);
}

async function getWatermark(env: Env) {
  const id = env.COUNTERS.idFromName(WatermarkCounterName);
  return env.COUNTERS.get(id).get();
}

async function setWatermark(env: Env, val: number) {
  const id = env.COUNTERS.idFromName(WatermarkCounterName);
  return env.COUNTERS.get(id).set(val);
}

async function incrementActiveCounter(env: Env) {
  const id = env.COUNTERS.idFromName(ActiveCounterName);
  await env.COUNTERS.get(id).increment();
}

async function decrementActiveCounter(env: Env) {
  const id = env.COUNTERS.idFromName(ActiveCounterName);
  await env.COUNTERS.get(id).decrement();
}

async function getActiveCounter(env: Env) {
  const id = env.COUNTERS.idFromName(ActiveCounterName);
  return env.COUNTERS.get(id).get();
}
