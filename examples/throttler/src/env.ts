import { Counter } from "durable_counter";

export interface Env {
  UPSTREAM: string;
  USE_HTTPS: boolean;
  ENABLE_JITTER: boolean;
  JITTER_DELAY: number;
  BACKPRESSURE: Backpressure;
  COUNTERS: DurableObjectNamespace<Counter>;
}

interface Backpressure {
  ENABLE_BACKPRESSURE: boolean;
  CONGESTION_WINDOW_MIN: number;
  CONGESTION_WINDOW_MAX: number;
  QUERIES: BackpressureQuery[];
}

interface BackpressureQuery {
  NAME: string;
  QUERY: string;
  WARN_THRESHOLD: number;
  EMERGENCY_THRESHOLD: number;
}
