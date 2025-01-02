import { DurableObject } from "cloudflare:workers";

export class Counter extends DurableObject {
  async get(): Promise<number> {
    return (await this.ctx.storage.get("value")) || 0;
  }

  async increment(amount: number = 1): Promise<number> {
    let value: number = (await this.ctx.storage.get("value")) || 0;
    value += amount;
    await this.ctx.storage.put("value", value);
    return value;
  }

  async decrement(amount: number = 1): Promise<number> {
    let value: number = (await this.ctx.storage.get("value")) || 0;
    value -= amount;
    await this.ctx.storage.put("value", value);
    return value;
  }

  async set(value: number): Promise<number> {
    await this.ctx.storage.put("value", value);
    return value;
  }
}
