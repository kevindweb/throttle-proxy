import { DurableObject } from "cloudflare:workers";

export class Counter extends DurableObject {
  async getCounterValue(): Promise<number> {
    let value: number = (await this.ctx.storage.get("value")) || 0;
    return value;
  }

  async increment(amount: number = 1): Promise<number> {
    let value: number = (await this.ctx.storage.get("value")) || 0;
    value += amount;
    // You do not have to worry about a concurrent request having modified the value in storage.
    // "input gates" will automatically protect against unwanted concurrency.
    // Read-modify-write is safe.
    await this.ctx.storage.put("value", value);
    return value;
  }

  async decrement(amount: number = 1): Promise<number> {
    let value: number = (await this.ctx.storage.get("value")) || 0;
    value -= amount;
    await this.ctx.storage.put("value", value);
    return value;
  }
}
