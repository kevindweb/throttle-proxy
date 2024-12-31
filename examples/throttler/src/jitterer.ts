import { Middleware } from "middleware";
import { parseHeaderKey, HeaderCanWait, HeaderCriticality } from "header";
import { CriticalityCriticalPlus } from "criticality";
import { Env } from "env";

const NO_JITTER = 0 as const;

export const jitterer: Middleware = async (req, env, next) => {
  const jitter = random(0, getDelay(req, env));
  console.log(`Jitter for ${jitter} ms`);
  await sleep(jitter);
  return next(req, env);
};

function sleep(ms: number): Promise<null> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function random(min: number, max: number): number {
  return Math.random() * (max - min) + min;
}

function parseDuration(duration: string): number {
  var seconds = 0;
  var days = duration.match(/(\d+)\s*d/);
  var hours = duration.match(/(\d+)\s*h/);
  var minutes = duration.match(/(\d+)\s*m/);
  var strSeconds = duration.match(/(\d+)\s*s/);
  if (days) {
    seconds += parseInt(days[1]) * 86400;
  }
  if (hours) {
    seconds += parseInt(hours[1]) * 3600;
  }
  if (minutes) {
    seconds += parseInt(minutes[1]) * 60;
  }
  if (strSeconds) {
    seconds += parseInt(strSeconds[1]);
  }
  return seconds * 1000;
}

function getDelay(req: Request, env: Env): number {
  if (parseHeaderKey(req, HeaderCriticality) == CriticalityCriticalPlus) {
    return NO_JITTER;
  }

  const canWait = parseHeaderKey(req, HeaderCanWait);
  if (canWait == "") {
    return env.JITTER_DELAY;
  }

  return parseDuration(canWait);
}
