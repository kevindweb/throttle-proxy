import { Env } from "env";
import { Middleware } from "middleware";

const ActiveCounterName = "ACTIVE_BP_REQUESTS";

export const backpressure: Middleware = async (req, env, next) => {
  const count = await getActiveCounter(env);
  console.log(
    `Applying backpressure queries ${env.BACKPRESSURE.QUERIES.length} requests ${count}`
  );

  await incrementActiveCounter(env);
  const res = await next(req, env);
  await decrementActiveCounter(env);
  return res;
};

async function incrementActiveCounter(env: Env) {
  const id = env.COUNTERS.idFromName(ActiveCounterName);
  const counter = env.COUNTERS.get(id);
  await counter.increment();
}

async function decrementActiveCounter(env: Env) {
  const id = env.COUNTERS.idFromName(ActiveCounterName);
  const counter = env.COUNTERS.get(id);
  await counter.decrement();
}

async function getActiveCounter(env: Env) {
  const id = env.COUNTERS.idFromName(ActiveCounterName);
  const counter = env.COUNTERS.get(id);
  return counter.getCounterValue();
}
