export type ThrottlingCurve = number;

export interface BackpressureQueryTimeSeries {
  [timestamp: number]: {
    [queryName: string]: number;
  };
}

export interface BackpressureCache {
  lastUpdated: number;
  updates: {
    [timestamp: number]: {
      [queryName: string]: number;
    };
  };
}

export interface BackpressureKVQueries {
  [query_name: string]: number;
}

export interface BackpressureQuery {
  name: string;
  type: string;
  config: string;
  warn: number;
  emergency: number;
  curve: ThrottlingCurve;
}

export interface Env {
  Queries: BackpressureQuery[];
  BACKPRESSURE_KV: KVNamespace;
}
