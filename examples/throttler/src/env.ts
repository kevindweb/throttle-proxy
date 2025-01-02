import { Counter } from "durable_counter";
import { QueryValues } from "durable_query_values";

export interface Env {
  UPSTREAM: string;
  USE_HTTPS: boolean;
  ENABLE_JITTER: boolean;
  JITTER_DELAY: number;
  BACKPRESSURE: Backpressure;
  COUNTERS: DurableObjectNamespace<Counter>;
  QUERY_VALUES: DurableObjectNamespace<QueryValues>;
}

export interface Backpressure {
  ENABLED: boolean;
  CWDN_MIN: number;
  CWDN_MAX: number;
  QUERIES: BackpressureQuery[];
}

export interface BackpressureQuery {
  NAME: string;
  QUERY: string;
  WARN_THRESHOLD: number;
  EMERGENCY_THRESHOLD: number;
}
