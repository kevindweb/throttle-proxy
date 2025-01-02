import { DurableObject } from "cloudflare:workers";

export class QueryValues extends DurableObject {
  async get(): Promise<number[]> {
    return (await this.ctx.storage.get("values")) || [];
  }

  async set(values: number[]) {
    await this.ctx.storage.put("values", values);
  }
}
