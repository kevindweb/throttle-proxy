# Backpressure Worker

A flexible Cloudflare Worker for dynamically calculating system backpressure based on configurable queries.

## Overview

The Backpressure Worker allows you to:

- Define multiple query types for monitoring system load
- Calculate backpressure based on custom thresholds
- Gracefully handle partial query failures
- Cache and track query results using Workers KV
- Visualize metrics through an interactive dashboard

## Prerequisites

- Cloudflare Workers account
- Wrangler CLI

## Installation

1. Create KV Namespace

```bash
wrangler kv:namespace create BACKPRESSURE_KV
```

2. Update `wrangler.json`

```json
{
  "kv_namespaces": [
    {
      "binding": "BACKPRESSURE_KV",
      "id": "your-production-namespace-id",
      "preview_id": "your-preview-namespace-id"
    }
  ]
}
```

3. Configure Queries in your environment variables

```json
{
  "vars": {
    "Queries": [
      {
        "name": "example-query",
        "type": "your-query-type",
        "warn": 500,
        "emergency": 900,
        "curve": 4
      }
    ]
  }
}
```

## API Endpoints

The worker exposes three endpoints:

- `/` - Calculates and returns current backpressure value
- `/ui` - Realtime visualization dashboard
- `/cache` - Returns the complete cache history
- `/queries` - Returns the configured queries

All endpoints return JSON responses.

## Visualization Dashboard

The worker includes a built-in monitoring dashboard that provides real-time visualization of all metrics.

The dashboard features:

- Line charts for each metric showing historical trends
- Warning and emergency threshold indicators
- Auto-refresh every 60 seconds
- Time-based filtering via URL parameters (`start` and `end`)
- Responsive design with mobile support

### Dashboard Dependencies

The visualization interface requires the following external libraries:

- Chart.js 3.7.0
- chartjs-plugin-annotation 1.3.0

### URL Parameters

The dashboard supports time-based filtering through URL parameters:

- `start`: Unix timestamp for the start of the time range
- `end`: Unix timestamp for the end of the time range

Example: `/?start=1706313600000&end=1706400000000`

### Customizing the Dashboard

The dashboard uses a clean, modern design with customizable styles. Key styling features include:

- Responsive container with max-width 1200px
- Card-based chart containers with subtle shadows
- System font stack for optimal readability
- Light color scheme with proper contrast
- Error state handling with visual feedback

## Caching Mechanism

- Uses Workers KV to store query results
- Updates cache every 60 seconds (configurable)
- Maintains historical time-series data of query results
- Falls back to cached values on query failures
- Cache format:

```typescript
{
  lastUpdated: number;
  updates: {
    [timestamp: string]: {
      [queryName: string]: number;
      backpressure_percent: number;
    }
  }
}
```

## Query Configuration

### Required Fields

- `name`: Unique identifier for the query
- `type`: Query type handler identifier
- `warn`: Warning threshold value
- `emergency`: Emergency threshold value
- `curve`: Throttling curve exponential factor (default: 4)

## Backpressure Calculation

### Value Ranges

- **Normal Operation**: Value ≤ `warn` (0% throttling)
- **Degraded Performance**: `warn` < value < `emergency` (partial throttling)
- **Emergency State**: Value ≥ `emergency` (100% throttling)

### Calculation Formula

```typescript
throttlePercent = 1 - Math.exp(-curve * loadFactor);
loadFactor =
  (currentValue - warningThreshold) / (emergencyThreshold - warningThreshold);
```

The final backpressure percentage is calculated as: `1 - Math.max(...throttlePercents)`

## Error Handling

- Graceful fallback to cached values on query failures
- Error responses include error messages in JSON format
- HTTP 500 status code for internal errors
- Console error logging for debugging

## Adding Custom Query Types

1. Implement the query handler:

```typescript
interface QueryHandler {
  refresh(query: BackpressureQuery): Promise<number>;
}

class MyCustomQueryHandler implements QueryHandler {
  async refresh(query: BackpressureQuery): Promise<number> {
    // Implement query logic
    return value;
  }
}
```

2. Register the handler in `queries.ts`:

```typescript
export function getQueryHandler(type: string): QueryHandler {
  // Add your handler here
}
```

## Deployment

```bash
wrangler deploy
```
